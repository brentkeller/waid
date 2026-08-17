package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/sessions"
)

// scanKinds are the segments of the filter row, in the order §1.2 draws them. The empty kind is
// `all`, which is what the tab opens on.
var scanKinds = []detect.Kind{"", detect.KindReview, detect.KindPr, detect.KindAhead, detect.KindDirty}

// Column widths for a signal row. The kind, the reference and the age size themselves to the data;
// the meta column is dropped before the subject is squeezed below what a title needs to be
// recognised.
const (
	scanColumnGap  = 2
	scanRefMax     = 16
	scanMetaMax    = 32
	scanMetaMin    = 8
	scanSubjectMin = 24
)

// spinnerFrames turn while a pass is in flight. Detection is sub-second once the worker pool has
// warmed, but sub-second is still a freeze, so the tab bar says what is happening (§1).
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerInterval is how often the frame advances while loading.
const spinnerInterval = 100 * time.Millisecond

// scanLoadedMsg carries a finished detection pass back into the update loop. Every read is a
// tea.Cmd, so Update never waits on git or the network (§6).
type scanLoadedMsg struct {
	result detect.Result
	// at is when the pass ran, which is what the header's age counts from.
	at time.Time
	// err is a pass that did not finish. The last good result stays on screen and the failure moves to
	// the tab bar, since a refresh that failed costs the refresh and nothing else (§7).
	err error
}

// spinnerTickMsg advances the spinner. It is issued alongside a refresh and re-issued until the
// refresh lands, so nothing ticks while the app is idle.
type spinnerTickMsg struct{}

// scanModel is the Scan tab: the last good detection pass, the two filters over it, and the tree's
// state. The tree itself is rebuilt from these on demand, so a filter can never leave the list and
// the cursor describing different things.
type scanModel struct {
	result   detect.Result
	loadedAt time.Time

	// kind is the segment of the filter row that is selected; empty is `all`.
	kind detect.Kind

	cursor   int
	expanded map[string]bool

	// triaged are the keys promoted or dismissed since the pass ran. They are held here rather than
	// removed from the result so the pass stays the thing detection returned, and a fresh pass clears
	// them — detection already excludes a dismissed key from the next one.
	triaged map[string]bool

	// loading is a pass in flight, and spinner is the frame the tab bar is showing for it.
	loading bool
	spinner int

	// failure is why the last pass did not land, standing until one does. The result above it is
	// whatever was last read successfully.
	failure string

	// load is the detection seam. It runs off the update loop and comes back as a scanLoadedMsg.
	load func() tea.Msg
}

// scanLoader is the real read: the folded log and the harvested sessions, handed to detection the
// same way every command hands them to it.
func scanLoader(opts Options) func() tea.Msg {
	return guarded(func() scanLoadedMsg {
		now := time.Now()
		state := events.Load(opts.Cfg.EventsPath)
		result := detect.Signals(opts.Cfg, detect.Deps{
			Sessions: sessions.Load(opts.Cfg).Sessions,
			State:    state,
			Now:      now,
		})
		return scanLoadedMsg{result: result, at: now}
	})
}

// guarded turns a pass that panics into a pass that failed. Detection reaches git and the network, so
// it is the one read that can fail outright, and the CLI already treats that as a note rather than a
// crash; the app keeps its last good data and moves the failure to the tab bar (§7).
func guarded(pass func() scanLoadedMsg) func() tea.Msg {
	return func() (msg tea.Msg) {
		defer func() {
			if recovered := recover(); recovered != nil {
				msg = scanLoadedMsg{err: fmt.Errorf("detection failed (%v)", recovered)}
			}
		}()
		return pass()
	}
}

// refresh starts a pass and sets the tab bar turning. A press while one is already in flight is
// ignored rather than queued.
func (m Model) refreshScan() (Model, tea.Cmd) {
	if m.scan.loading || m.scan.load == nil {
		return m, nil
	}

	m.scan.loading = true
	m.progress = m.spinnerText()
	return m, tea.Batch(m.scan.load, spinnerTick())
}

func spinnerTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

// spinnerText is the tab bar's right-hand slot while a pass runs.
func (m Model) spinnerText() string {
	return spinnerFrames[m.scan.spinner%len(spinnerFrames)] + " scanning"
}

// scanLoaded takes a finished pass. The tab's count and the age both move with it, and the cursor is
// pulled back into range in case the pass returned fewer signals than the list was showing.
//
// A pass that failed changes nothing but the tab bar: the list, the counts and the age stay as the
// last pass that landed left them (§7).
func (m Model) scanLoaded(msg scanLoadedMsg) Model {
	m.scan.loading = false
	m.progress = ""
	if msg.err != nil {
		m.scan.failure = msg.err.Error()
		return m
	}

	m.scan.result, m.scan.loadedAt, m.scan.failure = msg.result, msg.at, ""
	m.scan.triaged = nil
	m.counts[tabScan] = m.scanCount()

	tree := m.scanTree(m.viewWidth())
	m.scan.cursor = tree.Cursor
	return m
}

// scanCount is the badge on the tab: everything the pass returned, less what has been triaged away
// since it ran. It is the whole pass rather than the filtered list, since a filter narrows the view
// and not the work.
func (m Model) scanCount() int {
	count := 0
	for _, signal := range m.scan.result.Signals {
		if !m.scan.triaged[signal.Key] {
			count++
		}
	}
	return count
}

// scanKey handles the keys the chrome does not own while Scan is the live tab. It reports whether
// the press meant anything here, which includes a key that was inert for a reason worth printing.
func (m Model) scanKey(pressed string) (Model, tea.Cmd, bool) {
	tree := m.scanTree(m.viewWidth())

	switch pressed {
	case "j", "down":
		tree.Down()
	case "k", "up":
		tree.Up()
	case "g":
		tree.First()
	case "G":
		tree.Last()
	case "enter":
		tree.Toggle()
	case "s":
		m.scan.kind = nextScanKind(m.scan.kind)
		m.scan.cursor = 0
		return m, nil, true
	case "r":
		m, cmd := m.refreshScan()
		return m, cmd, true
	case "o":
		return m.openSelected(tree)
	case "p":
		return m.promoteSelected(tree)
	case "d":
		return m.dismissSelected(tree)
	case "e":
		return m.renameSelected()
	default:
		return m, nil, false
	}

	m.scan.cursor, m.scan.expanded = tree.Cursor, tree.Expanded
	return m, nil, true
}

// openSelected hands the row under the cursor to the browser. A project heading and a signal with no
// page — everything detected locally — are inert, and the footer says which (§4).
func (m Model) openSelected(tree Tree[detect.Signal]) (Model, tea.Cmd, bool) {
	signal, ok := tree.SelectedItem()
	if !ok {
		m.hint = "o opens a signal — this row is a project"
		return m, nil, true
	}

	url := signalUrl(signal)
	if url == "" {
		m.hint = fmt.Sprintf("%s signals have no page to open", signal.Kind)
		return m, nil, true
	}

	return m, func() tea.Msg {
		openUrl(url)
		return nil
	}, true
}

// nextScanKind advances the segmented filter, wrapping back to `all`.
func nextScanKind(current detect.Kind) detect.Kind {
	for i, kind := range scanKinds {
		if kind == current {
			return scanKinds[(i+1)%len(scanKinds)]
		}
	}
	return ""
}

// scanVisible are the signals the list is showing: the kind row and the typed query applied over the
// loaded pass. Both filter client-side, so neither issues any work (§1.2).
func (m Model) scanVisible() []detect.Signal {
	query := strings.ToLower(m.filter)

	var kept []detect.Signal
	for _, signal := range m.scan.result.Signals {
		if m.scan.triaged[signal.Key] {
			continue
		}
		if m.scan.kind != "" && signal.Kind != m.scan.kind {
			continue
		}
		if query != "" && !matchesSignal(signal, query) {
			continue
		}
		kept = append(kept, signal)
	}
	return kept
}

// matchesSignal is the query test: every column the row can show, plus the key the signal is
// addressed by, so a key pasted from the CLI finds its row.
func matchesSignal(signal detect.Signal, query string) bool {
	fields := []string{signal.Key, string(signal.Kind), signal.Title, signal.Subject, signal.Detail}
	if signal.Project != nil {
		fields = append(fields, *signal.Project)
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

// scanTree builds the tree from the current state. It is rebuilt rather than stored because the two
// filters change what the cursor is addressing, and a stored tree would go stale between them.
func (m Model) scanTree(width int) Tree[detect.Signal] {
	visible := m.scanVisible()
	columns := scanColumnsFor(visible)

	tree := Tree[detect.Signal]{
		Groups:            scanGroups(visible),
		Cursor:            m.scan.cursor,
		Expanded:          m.scan.expanded,
		ExpandedByDefault: true,
		Render: func(signal detect.Signal, width int, _ bool) string {
			return scanRow(signal, columns, width)
		},
	}
	tree.clamp()
	return tree
}

// scanGroups files the signals under their projects, keeping the rank order detection returned both
// within a group and between them.
func scanGroups(signals []detect.Signal) []Group[detect.Signal] {
	var groups []Group[detect.Signal]
	index := map[string]int{}

	for _, signal := range signals {
		key := scanGroupKey(signal)
		at, known := index[key]
		if !known {
			at = len(groups)
			index[key] = at
			groups = append(groups, Group[detect.Signal]{Key: key, Title: key})
		}
		groups[at].Items = append(groups[at].Items, signal)
	}

	for i := range groups {
		groups[i].Meta = plural(len(groups[i].Items), "signal")
	}
	return groups
}

// scanGroupKey is the project a signal is filed under: its local checkout when detection matched
// one, and otherwise the GitHub repository its key names, so a pull request against a repo with no
// clone still groups with its own repo rather than into a bucket of everything unmatched.
func scanGroupKey(signal detect.Signal) string {
	if signal.Project != nil {
		return *signal.Project
	}
	if repository, _ := signalRepository(signal); repository != "" {
		return repository
	}
	return "no project"
}

// signalRepository splits a GitHub key into the `owner/repo` and the number it names. A local key —
// `ahead:<repo>:<branch>`, `dirty:<repo>` — carries no number and comes back empty.
func signalRepository(signal detect.Signal) (repository, number string) {
	_, rest, found := strings.Cut(signal.Key, ":")
	if !found {
		return "", ""
	}
	repository, number, found = strings.Cut(rest, "#")
	if !found {
		return "", ""
	}
	return repository, number
}

// signalUrl is the page a signal opens to: the pull request its key names. Locally detected signals
// have no page, and `o` is inert on them.
func signalUrl(signal detect.Signal) string {
	if signal.Kind != detect.KindReview && signal.Kind != detect.KindPr {
		return ""
	}

	repository, number := signalRepository(signal)
	if repository == "" || number == "" {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/pull/%s", repository, number)
}

// openUrl hands a URL to whatever the desktop opens links with. It is a variable so a test can watch
// it rather than open windows.
var openUrl = func(url string) error {
	switch runtime.GOOS {
	case "windows":
		// url.dll is the handler the shell itself uses, and it needs no console window to run in.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// signalRef is the middle identifier a row is addressed by: the pull request number for a GitHub
// signal, the branch for a local one.
func signalRef(signal detect.Signal) string {
	if _, number := signalRepository(signal); number != "" {
		return "#" + number
	}
	if signal.Branch != nil {
		return *signal.Branch
	}
	return ""
}

// signalMeta is the right-hand column without the age, which the row prints in its own column. The
// age is always the last part detection joined into the detail, so it comes off the end.
func signalMeta(signal detect.Signal) string {
	meta := signal.Detail
	if signal.Age == "" {
		return meta
	}
	return strings.TrimSuffix(strings.TrimSuffix(meta, signal.Age), " · ")
}

// scanColumns are the widths a row's fixed columns share, measured across every visible signal so
// the list reads down a column rather than across a row.
type scanColumns struct{ kind, ref, meta, age int }

func scanColumnsFor(signals []detect.Signal) scanColumns {
	var columns scanColumns
	for _, signal := range signals {
		columns.kind = max(columns.kind, lipgloss.Width(string(signal.Kind)))
		columns.ref = max(columns.ref, lipgloss.Width(signalRef(signal)))
		columns.meta = max(columns.meta, lipgloss.Width(signalMeta(signal)))
		columns.age = max(columns.age, lipgloss.Width(signal.Age))
	}

	columns.ref = min(columns.ref, scanRefMax)
	columns.meta = min(columns.meta, scanMetaMax)
	return columns
}

// scanRow lays one signal out across the width the tree left it: kind, reference, subject, meta, and
// the age against the right edge.
func scanRow(signal detect.Signal, columns scanColumns, width int) string {
	if width <= 0 {
		return ""
	}

	// The meta column gives its width up to the subject before the subject is squeezed below what a
	// title needs to be recognised, and is dropped outright once what is left of it says nothing.
	meta := columns.meta
	fixed := columns.kind + columns.ref + columns.age
	subject := width - fixed - meta - scanColumnGap*4
	if subject < scanSubjectMin {
		meta -= scanSubjectMin - subject
		subject = scanSubjectMin
	}
	if meta < scanMetaMin {
		meta, subject = 0, width-fixed-scanColumnGap*3
	}

	cells := []string{pad(string(signal.Kind), columns.kind), pad(signalRef(signal), columns.ref), pad(signal.Subject, subject)}
	if meta > 0 {
		cells = append(cells, pad(signalMeta(signal), meta))
	}
	cells = append(cells, padLeft(signal.Age, columns.age))

	gap := strings.Repeat(" ", scanColumnGap)
	return strings.TrimRight(strings.Join(cells, gap), " ")
}

// scanBody is the tab between the bar and the footer: the filter row and its counts, the rule under
// them, and the tree.
func (m Model) scanBody(width int) string {
	body := []string{m.scanHeader(width), m.theme.Divider.Render(strings.Repeat("─", max(width, 0)))}

	tree := m.scanTree(width)
	if tree.Len() == 0 {
		body = append(body, m.theme.Dim.Render("  "+tabEmpty[tabScan]))
	} else {
		body = append(body, tree.View(width, m.theme))
	}

	if strip := m.noteStrip(m.scan.result.Notes); strip != "" {
		body = append(body, strip)
	}
	return strings.Join(body, "\n")
}

// scanHeader is the segmented kind row with the counts opposite it (§1.2).
func (m Model) scanHeader(width int) string {
	row, rowWidth := m.scanFilterRow()
	counts := m.scanCounts()

	gap := width - rowWidth - lipgloss.Width(counts) - 1
	if gap < 1 {
		gap = 1
	}
	return row + strings.Repeat(" ", gap) + m.theme.Count.Render(counts)
}

// scanFilterRow draws the segments and returns the printed width alongside them, since the styling
// makes the string longer than the columns it occupies.
func (m Model) scanFilterRow() (string, int) {
	var row strings.Builder
	row.WriteString(" ")
	width := 1

	for i, kind := range scanKinds {
		if i > 0 {
			row.WriteString("  ")
			width += 2
		}

		label := scanKindLabel(kind)
		style := m.theme.FilterInactive
		if kind == m.scan.kind {
			label, style = "‹"+label+"›", m.theme.FilterActive
		}

		row.WriteString(style.Render(label))
		width += lipgloss.Width(label)
	}
	return row.String(), width
}

func scanKindLabel(kind detect.Kind) string {
	if kind == "" {
		return "all"
	}
	return string(kind)
}

// scanCounts is what the header says on the right: what is shown, what was suppressed, and how old
// the pass is.
func (m Model) scanCounts() string {
	parts := []string{plural(len(m.scanVisible()), "signal")}
	// The keys triaged since the pass ran carry a dismiss each, so they count where a refresh would
	// count them.
	if dismissed := m.scan.result.DismissedCount + len(m.scan.triaged); dismissed > 0 {
		parts = append(parts, fmt.Sprintf("%d dismissed", dismissed))
	}
	if age := m.scanAge(); age != "" {
		parts = append(parts, age)
	}
	return strings.Join(parts, " · ")
}

// scanAge is how long ago the shown data was read, which is the only thing that says a list has gone
// stale while the app sat idle.
func (m Model) scanAge() string {
	if m.scan.loadedAt.IsZero() {
		return ""
	}

	age := render.RelTime(events.FormatTs(m.scan.loadedAt), m.now())
	if age == "just now" {
		return "⏱ just now"
	}
	return "⏱ " + age + " ago"
}

// plural counts a noun for a column: `1 signal`, `2 signals`.
func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// pad fits text to a column, measuring the printed width rather than counting runes.
func pad(text string, width int) string {
	if width <= 0 {
		return ""
	}

	text = truncate(text, width)
	return text + strings.Repeat(" ", width-lipgloss.Width(text))
}

// padLeft is pad against the right edge, for the columns that read as numbers.
func padLeft(text string, width int) string {
	if width <= 0 {
		return ""
	}

	text = truncate(text, width)
	return strings.Repeat(" ", width-lipgloss.Width(text)) + text
}
