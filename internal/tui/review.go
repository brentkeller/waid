package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/sessions"
)

// reviewRange is a segment of the range row (§1.3): the calendar window the tab reports on. The
// whole history is loaded once and every window is measured over it, so moving the row is instant
// and issues no work.
type reviewRange int

const (
	rangeToday reviewRange = iota
	rangeWeek
	rangeLastWeek
)

var reviewRangeLabels = [...]string{"today", "week", "last week"}

// Column widths for a session row. The start time and the prompt count are fixed by their data; the
// title takes what is left and is the first thing squeezed, since the fold above it already says
// which project the row belongs to.
const (
	reviewColumnGap = 2
	reviewTitleMin  = 20
)

// The preview's turns: the marker a role is announced with, the column its text hangs from, and the
// gutter the pane keeps in front of it when it is drawn as an overlay.
const (
	turnMark      = "▸"
	turnIndent    = 2
	previewMargin = 1
)

// noProject is the group key for a session whose transcript never recorded a cwd.
const noProject = "(no project)"

// closure is one item closed, with when and where. The window is applied at count time rather than
// at read time, so all three ranges are answerable from one read of the log.
type closure struct {
	id      string
	project *string
	at      time.Time
}

// reviewLoadedMsg carries a finished read of the session history back into the update loop.
type reviewLoadedMsg struct {
	sessions []sessions.Session
	closures []closure
	// paths is where each session's transcript was when the cache was written, keyed by session id.
	// It is what the preview reads; a session missing from it has no transcript on record.
	paths map[string]string
	// at is when the read ran.
	at time.Time
}

// previewLoadedMsg carries a finished read of one transcript back into the update loop. The id is
// carried with it so a read that lands after the cursor has moved on is dropped rather than shown
// against the wrong session.
type previewLoadedMsg struct {
	id    string
	turns []sessions.Turn
	err   error
}

// reviewModel is the Review tab: the whole harvested history, the window over it, and the tree's
// state. Like Scan, the tree is rebuilt from these on demand so a window and a query can never leave
// the list and the cursor describing different things.
type reviewModel struct {
	sessions []sessions.Session
	closures []closure
	paths    map[string]string
	loadedAt time.Time

	// window is the segment of the range row that is selected.
	window reviewRange

	cursor   int
	expanded map[string]bool

	// preview is whether the transcript pane is open. It is closed until space opens it, and its
	// geometry is the width rule rather than a setting of its own (§5).
	preview bool

	// previewId is the session the pane is showing, with the turns read for it and the reason the read
	// failed. reading is a read in flight, which is what separates a transcript with no turns from one
	// that has not been read yet.
	previewId  string
	turns      []sessions.Turn
	previewErr string
	reading    bool

	// load is the history seam. It runs off the update loop and comes back as a reviewLoadedMsg.
	load func() tea.Msg

	// readTurns is the transcript seam. A transcript is a file rather than a network call, but it is
	// the largest read the app makes, so it runs off the update loop like every other one (§6).
	readTurns func(path string) ([]sessions.Turn, error)
}

// reviewLoader is the real read: the session cache and the closures the log holds. Neither reaches
// the network, but both are files, so they are read off the update loop like every other read (§6).
func reviewLoader(opts Options) func() tea.Msg {
	return func() tea.Msg {
		cached := sessions.Load(opts.Cfg).Sessions
		return reviewLoadedMsg{
			sessions: harvested(cached),
			closures: closures(opts.Cfg),
			paths:    transcriptPaths(cached),
			at:       time.Now(),
		}
	}
}

// harvested drops the cache's stat fields, which are an implementation detail of syncing rather than
// part of a session's history.
func harvested(cached []sessions.CachedSession) []sessions.Session {
	harvest := make([]sessions.Session, 0, len(cached))
	for _, session := range cached {
		harvest = append(harvest, session.Session)
	}
	return harvest
}

// transcriptPaths is where the preview reads a session from. The path is the one thing the sync's
// stat fields carry that is history rather than bookkeeping, so it is kept beside the sessions
// rather than inside them.
func transcriptPaths(cached []sessions.CachedSession) map[string]string {
	paths := make(map[string]string, len(cached))
	for _, session := range cached {
		paths[session.Id] = session.File.Path
	}
	return paths
}

// readTranscript is the real transcript read, streamed a line at a time the way `waid transcript`
// reads it.
func readTranscript(path string) ([]sessions.Turn, error) {
	return sessions.ReadTurns(path, sessions.ReadLines)
}

// closures reads every close the log holds, resolved against the folded state so each one carries
// the project its item belongs to. An item closed, reopened and closed again contributes one closure
// per close; the count for a window folds the repeats out.
func closures(cfg config.Config) []closure {
	state := events.Load(cfg.EventsPath)

	var closed []closure
	for _, record := range events.ReadRecords(cfg.EventsPath) {
		id, hasId := record.Value["id"].(string)
		ts, hasTs := record.Value["ts"].(string)
		if record.Value["ev"] != "close" || !hasId || !hasTs {
			continue
		}

		at, parsed := render.ParseTime(ts)
		item, found := state.Find(id)
		if !parsed || !found {
			continue
		}
		closed = append(closed, closure{id: item.Id, project: item.Project, at: at})
	}
	return closed
}

// refreshReview re-reads the history. The window and the query are answered from what is already
// loaded, so this is only ever the explicit refresh.
func (m Model) refreshReview() (Model, tea.Cmd) {
	if m.review.load == nil {
		return m, nil
	}
	return m, m.review.load
}

// reviewLoaded takes a finished read, pulling the cursor back into range in case the history came
// back shorter than the list was showing. The pane follows it, since a reload can move the row the
// cursor was resting on.
func (m Model) reviewLoaded(msg reviewLoadedMsg) (Model, tea.Cmd) {
	m.review.sessions, m.review.closures, m.review.loadedAt = msg.sessions, msg.closures, msg.at
	m.review.paths = msg.paths

	tree := m.reviewTree(m.viewWidth())
	m.review.cursor = tree.Cursor
	return m.previewSync(tree)
}

// reviewKey handles the keys the chrome does not own while Review is the live tab. The actions are
// not wired yet; they are bound in the key table, so they are silent rather than reported as unbound.
func (m Model) reviewKey(pressed string) (Model, tea.Cmd, bool) {
	tree := m.reviewTree(m.viewWidth())

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
	case " ":
		m.review.preview = !m.review.preview
		m, cmd := m.previewSync(tree)
		return m, cmd, true
	case "s":
		m.review.window = (m.review.window + 1) % reviewRange(len(reviewRangeLabels))
		m.review.cursor = 0
		return m, nil, true
	case "r":
		m, cmd := m.refreshReview()
		return m, cmd, true
	default:
		return m, nil, false
	}

	m.review.cursor, m.review.expanded = tree.Cursor, tree.Expanded

	m, cmd := m.previewSync(tree)
	return m, cmd, true
}

// previewSync keeps the open pane pointed at the row under the cursor: a session it has not read yet
// is read off the update loop, and a project heading empties it. A closed pane reads nothing at all.
func (m Model) previewSync(tree Tree[sessions.Session]) (Model, tea.Cmd) {
	if !m.review.preview {
		return m, nil
	}

	session, selected := tree.SelectedItem()
	if !selected {
		m.review.previewId, m.review.turns, m.review.previewErr, m.review.reading = "", nil, "", false
		return m, nil
	}
	if session.Id == m.review.previewId {
		return m, nil
	}

	m.review.previewId, m.review.turns, m.review.previewErr = session.Id, nil, ""
	m.review.reading = true
	return m, m.previewCmd(session.Id)
}

// previewCmd is the transcript read for one session. A session the cache holds no path for is not a
// failure worth a bar slot — the cache records where a transcript was, not where it still is — so it
// comes back as the pane's own note (§7).
func (m Model) previewCmd(id string) tea.Cmd {
	path, recorded := m.review.paths[id]
	read := m.review.readTurns

	return func() tea.Msg {
		if !recorded || path == "" {
			return previewLoadedMsg{id: id, err: errors.New("no transcript on record for this session")}
		}
		if read == nil {
			read = readTranscript
		}

		turns, err := read(path)
		if err != nil {
			return previewLoadedMsg{id: id, err: fmt.Errorf("cannot read the transcript: %s", path)}
		}
		return previewLoadedMsg{id: id, turns: turns}
	}
}

// previewLoaded takes a finished transcript read, dropping one the cursor has already moved past.
func (m Model) previewLoaded(msg previewLoadedMsg) Model {
	if msg.id != m.review.previewId {
		return m
	}

	m.review.reading = false
	m.review.turns, m.review.previewErr = msg.turns, ""
	if msg.err != nil {
		m.review.previewErr = msg.err.Error()
	}
	return m
}

// reviewWindow is the half-open span the selected segment covers, measured against the app's present
// so a window opened before midnight moves when it passes.
func (m Model) reviewWindow() (start, end time.Time) {
	now := m.now()
	switch m.review.window {
	case rangeWeek:
		return render.WeekBounds(now, 0)
	case rangeLastWeek:
		return render.WeekBounds(now, -1)
	default:
		return render.DayBounds(render.LocalYmd(now))
	}
}

// reviewVisible are the sessions the list is showing: the window and the typed query applied over
// the loaded history, oldest first. A session that recorded no prompts is history of nothing and is
// left out, as it is in `waid today`.
func (m Model) reviewVisible() []sessions.Session {
	start, end := m.reviewWindow()
	query := strings.ToLower(m.filter)

	var kept []sessions.Session
	for _, session := range m.review.sessions {
		if session.Prompts == 0 || !session.Overlaps(start, end) {
			continue
		}
		if query != "" && !matchesSession(session, query) {
			continue
		}
		kept = append(kept, session)
	}

	slices.SortStableFunc(kept, sessions.ByStart)
	return kept
}

// matchesSession is the query test: the columns the row can show, plus the branch and the session id,
// so an id pasted from a transcript finds its row.
func matchesSession(session sessions.Session, query string) bool {
	fields := []string{session.Id, session.Title}
	for _, optional := range []*string{session.Project, session.Branch} {
		if optional != nil {
			fields = append(fields, *optional)
		}
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

// reviewClosed counts the items closed inside the window. An item closed, reopened and closed again
// in one window is still one closure, which is what `waid week` already counts.
func (m Model) reviewClosed() int {
	start, end := m.reviewWindow()

	counted := map[string]bool{}
	for _, closed := range m.review.closures {
		if closed.at.Before(start) || !closed.at.Before(end) || counted[closed.id] {
			continue
		}
		counted[closed.id] = true
	}
	return len(counted)
}

// reviewTree builds the tree from the current state. Groups are closed by default, which is the whole
// reason Review reads better than the command it replaces (§1.3).
func (m Model) reviewTree(width int) Tree[sessions.Session] {
	visible := m.reviewVisible()
	columns := reviewColumnsFor(visible)

	tree := Tree[sessions.Session]{
		Groups:   reviewGroups(visible),
		Cursor:   m.review.cursor,
		Expanded: m.review.expanded,
		Render: func(session sessions.Session, width int, _ bool) string {
			return reviewRow(session, columns, width)
		},
	}
	tree.clamp()
	return tree
}

// reviewGroups files the sessions under their projects, busiest first so the window reads as where
// the time went; ties fall back to name, matching how `waid today` orders the same rollup.
func reviewGroups(visible []sessions.Session) []Group[sessions.Session] {
	var groups []Group[sessions.Session]
	index := map[string]int{}

	for _, session := range visible {
		key := reviewGroupKey(session)
		at, known := index[key]
		if !known {
			at = len(groups)
			index[key] = at
			groups = append(groups, Group[sessions.Session]{Key: key, Title: key})
		}
		groups[at].Items = append(groups[at].Items, session)
	}

	for i := range groups {
		groups[i].Meta = reviewGroupMeta(groups[i].Items)
	}

	slices.SortStableFunc(groups, func(a, b Group[sessions.Session]) int {
		if left, right := promptsIn(a.Items), promptsIn(b.Items); left != right {
			return right - left
		}
		return project.CompareNames(a.Items[0].Project, b.Items[0].Project)
	})
	return groups
}

func reviewGroupKey(session sessions.Session) string {
	if session.Project == nil {
		return noProject
	}
	return *session.Project
}

// reviewGroupMeta is the count on the fold: what a project cost in sessions and in prompts.
func reviewGroupMeta(items []sessions.Session) string {
	return plural(len(items), "session") + " · " + plural(promptsIn(items), "prompt")
}

func promptsIn(items []sessions.Session) int {
	total := 0
	for _, session := range items {
		total += session.Prompts
	}
	return total
}

// reviewColumns are the widths a row's fixed columns share, measured across every visible session so
// the list reads down a column rather than across a row.
type reviewColumns struct{ start, prompts int }

func reviewColumnsFor(visible []sessions.Session) reviewColumns {
	var columns reviewColumns
	for _, session := range visible {
		columns.start = max(columns.start, lipgloss.Width(sessionStart(session)))
		columns.prompts = max(columns.prompts, lipgloss.Width(plural(session.Prompts, "prompt")))
	}
	return columns
}

// sessionStart is the clock time a session began, in the terminal's own timezone. A session whose
// timestamps are unreadable shows none rather than a placeholder.
func sessionStart(session sessions.Session) string {
	started, ok := session.StartedAt()
	if !ok {
		return ""
	}
	return started.Local().Format("15:04")
}

// reviewRow lays one session out across the width the tree left it: the start time, the title, and
// the prompt count against the right edge.
func reviewRow(session sessions.Session, columns reviewColumns, width int) string {
	if width <= 0 {
		return ""
	}

	// The prompt count gives its column up to the title before the title is squeezed below what a
	// title needs to be recognised.
	prompts := columns.prompts
	title := width - columns.start - prompts - reviewColumnGap*2
	if title < reviewTitleMin {
		prompts, title = 0, width-columns.start-reviewColumnGap
	}

	cells := []string{pad(sessionStart(session), columns.start), pad(session.Title, title)}
	if prompts > 0 {
		cells = append(cells, padLeft(plural(session.Prompts, "prompt"), prompts))
	}

	gap := strings.Repeat(" ", reviewColumnGap)
	return strings.TrimRight(strings.Join(cells, gap), " ")
}

// reviewBody is the tab between the bar and the footer: the range row and its totals, the rule under
// them, and the list — beside the preview, or replaced by it, when the pane is open (§5). The height
// is the rows the body has been given, and is zero until the terminal says how tall it is.
func (m Model) reviewBody(width, height int) string {
	head := []string{m.reviewHeader(width), m.theme.Divider.Render(strings.Repeat("─", max(width, 0)))}
	rows := height - len(head)

	panes := Split(width)
	// Below the threshold a side-by-side split leaves the transcript a gutter too narrow to read prose
	// in, so the preview takes the whole width and the list gives way to it. The margin stands in for
	// the divider the split would have put in front of it, so the overlay hangs off the same column
	// every other row of the app does.
	if m.review.preview && !panes.SideBySide {
		overlay := m.previewLines(width-previewMargin, rows)
		for i, line := range overlay {
			overlay[i] = strings.TrimRight(strings.Repeat(" ", previewMargin)+line, " ")
		}
		return strings.Join(append(head, overlay...), "\n")
	}

	listWidth := width
	if m.review.preview {
		listWidth = panes.List
	}

	list := m.reviewList(listWidth)
	if m.review.preview {
		list = m.splitRows(list, m.previewLines(panes.Detail, rows), panes.List)
	}
	return strings.Join(append(head, list...), "\n")
}

// reviewList is the tree's lines, or what the tab says when the window holds nothing.
func (m Model) reviewList(width int) []string {
	tree := m.reviewTree(width)
	if tree.Len() == 0 {
		return []string{m.theme.Dim.Render("  " + tabEmpty[tabReview])}
	}
	return strings.Split(tree.View(width, m.theme), "\n")
}

// splitRows lays the list and the preview side by side, with the rule between them running down the
// taller of the two so the split reads as one shape rather than two ragged blocks.
func (m Model) splitRows(list, preview []string, listWidth int) []string {
	rule := m.theme.Divider.Render("│")

	rows := max(len(list), len(preview))
	out := make([]string, 0, rows)
	for i := range rows {
		line := fill(lineAt(list, i), listWidth) + " " + rule + " " + lineAt(preview, i)
		out = append(out, strings.TrimRight(line, " "))
	}
	return out
}

func lineAt(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return ""
	}
	return lines[i]
}

// fill grows a rendered line to a column count, measuring the printed width so the styling is not
// counted. It never cuts: what the panes hold was laid out to fit them already.
func fill(line string, width int) string {
	if gap := width - lipgloss.Width(line); gap > 0 {
		return line + strings.Repeat(" ", gap)
	}
	return line
}

// previewLines is the pane's contents: what identifies the session, then its turns by role. It is
// cut to the rows the pane has, since a transcript is longer than any terminal and the tab is not a
// session viewer — reading one properly is what `R` hands off to Claude for.
func (m Model) previewLines(width, height int) []string {
	if width <= 0 {
		return nil
	}

	session, known := m.previewSession()
	if !known {
		return m.previewNote("select a session to preview", width)
	}

	lines := []string{
		m.theme.Heading.Render(truncate(session.Title, width)),
		m.theme.Meta.Render(truncate(previewSpan(session), width)),
		m.theme.Meta.Render(truncate(session.Id, width)),
		"",
	}

	switch {
	case m.review.previewErr != "":
		lines = append(lines, m.previewNote(m.review.previewErr, width)...)
	case m.review.reading:
		lines = append(lines, m.previewNote("reading the transcript…", width)...)
	case len(m.review.turns) == 0:
		lines = append(lines, m.previewNote("(no turns)", width)...)
	default:
		lines = append(lines, m.turnLines(width)...)
	}
	return cut(lines, height, m.theme)
}

// previewNote is the pane with nothing to show in it: no row selected, no transcript on record, or
// a read still running.
func (m Model) previewNote(note string, width int) []string {
	return []string{m.theme.Dim.Render(truncate(note, width))}
}

// previewSession is the session the pane is pointed at, which a reload can leave the history without.
func (m Model) previewSession() (sessions.Session, bool) {
	for _, session := range m.review.sessions {
		if session.Id == m.review.previewId {
			return session, true
		}
	}
	return sessions.Session{}, false
}

// previewSpan is the line under the title: when the session ran, what it cost, and the branch it ran
// on. Each part is left out when the session never recorded it.
func previewSpan(session sessions.Session) string {
	var parts []string
	if start := sessionStart(session); start != "" {
		span := start
		if ended, ok := session.EndedAt(); ok {
			span += "–" + ended.Local().Format("15:04")
		}
		parts = append(parts, span)
	}

	parts = append(parts, plural(session.Prompts, "prompt"))
	if session.Branch != nil {
		parts = append(parts, *session.Branch)
	}
	return strings.Join(parts, " · ")
}

// turnLines prints the conversation: a role marker per turn with the text wrapped under it, which is
// enough to recognise a session without being a transcript viewer.
func (m Model) turnLines(width int) []string {
	var lines []string
	for i, turn := range m.review.turns {
		if i > 0 {
			lines = append(lines, "")
		}

		lines = append(lines, m.theme.Heading.Render(turnMark+" "+turn.RoleLabel()))
		for _, line := range wrap(turn.Text, width-turnIndent) {
			lines = append(lines, m.theme.Row.Render(strings.Repeat(" ", turnIndent)+line))
		}
	}
	return lines
}

// wrap breaks text onto lines that fit the width, keeping the paragraph breaks the transcript
// carried but not the runs of blank lines between them. A word wider than the column is cut rather
// than pushing the pane wider than the terminal.
func wrap(text string, width int) []string {
	if width <= 0 {
		return nil
	}

	var lines []string
	blank := func() bool { return len(lines) == 0 || lines[len(lines)-1] == "" }

	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			if !blank() {
				lines = append(lines, "")
			}
			continue
		}

		line := ""
		for _, word := range words {
			switch {
			case line == "":
				line = word
			case lipgloss.Width(line)+1+lipgloss.Width(word) <= width:
				line += " " + word
			default:
				lines = append(lines, truncate(line, width))
				line = word
			}
		}
		lines = append(lines, truncate(line, width))
	}
	return lines
}

// cut trims a block to the rows it has, marking the cut so a truncated transcript does not read as a
// short one. A height of zero is a terminal that has not said how tall it is, and cuts nothing.
func cut(lines []string, height int, theme Theme) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	return append(lines[:height-1:height-1], theme.Dim.Render("…"))
}

// reviewHeader is the segmented range row with the window's totals opposite it (§1.3).
func (m Model) reviewHeader(width int) string {
	row, rowWidth := m.reviewRangeRow()
	totals := m.reviewTotals()

	gap := width - rowWidth - lipgloss.Width(totals) - 1
	if gap < 1 {
		gap = 1
	}
	return row + strings.Repeat(" ", gap) + m.theme.Count.Render(totals)
}

// reviewRangeRow draws the segments and returns the printed width alongside them, since the styling
// makes the string longer than the columns it occupies.
func (m Model) reviewRangeRow() (string, int) {
	var row strings.Builder
	row.WriteString(" ")
	width := 1

	for i, label := range reviewRangeLabels {
		if i > 0 {
			row.WriteString("  ")
			width += 2
		}

		style := m.theme.FilterInactive
		if reviewRange(i) == m.review.window {
			label, style = "‹"+label+"›", m.theme.FilterActive
		}

		row.WriteString(style.Render(label))
		width += lipgloss.Width(label)
	}
	return row.String(), width
}

// reviewTotals is what the header says on the right: the window's sessions, the prompts they took,
// and the items closed inside it. A window that closed nothing says nothing, the way the Scan header
// leaves out a dismissal count of zero.
func (m Model) reviewTotals() string {
	visible := m.reviewVisible()

	parts := []string{plural(len(visible), "session"), plural(promptsIn(visible), "prompt")}
	if closed := m.reviewClosed(); closed > 0 {
		parts = append(parts, fmt.Sprintf("%s closed", plural(closed, "item")))
	}
	return strings.Join(parts, " · ")
}
