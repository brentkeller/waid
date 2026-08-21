package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/config"
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
	rangeYesterday
	rangeWeek
	rangeLastWeek
	// rangeDate is the day typed at the prompt. It is a segment of the row only once a day has been
	// picked, so the row carries no empty slot on a tab nobody has typed a date into.
	rangeDate
)

var reviewRangeLabels = [...]string{"today", "yesterday", "week", "last week"}

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
	// The rows above the scrolling transcript: the tab's range row and its rule, and the session
	// heading the pane pins over the turns so a scrolled pane still says what is being read.
	reviewHeadRows  = 2
	previewHeadRows = 4

	// pageShare is how much of the pane a page key moves. It is short of the whole pane so the rows a
	// reader was last on stay on screen across the jump: a whole page replaces everything and has to be
	// re-oriented to, which is the difference between skimming a transcript and reading one.
	pageShare = 0.7
)

// noProject is the group key for a session whose transcript never recorded a cwd.
const noProject = "(no project)"

// reviewLoadedMsg carries a finished read of the session history back into the update loop.
type reviewLoadedMsg struct {
	sessions []sessions.Session
	// paths is where each session's transcript was when the cache was written, keyed by session id.
	// It is what the preview reads; a session missing from it has no transcript on record.
	paths map[string]string
	// at is when the read ran.
	at time.Time
	// note is what the read leaves for the footer: a sync that could not be written, or what an
	// explicit refresh cost. An empty note leaves the footer as it was.
	note string
}

// previewLoadedMsg carries a finished read of one transcript back into the update loop. The id is
// carried with it so a read that lands after the cursor has moved on is dropped rather than shown
// against the wrong session.
type previewLoadedMsg struct {
	id    string
	turns []sessions.Turn
	err   error
}

// resumedMsg is Claude exiting and the app coming back to the alternate screen, with whatever went
// wrong reaching it rather than the terminal the process was handed.
type resumedMsg struct{ err error }

// copiedMsg is a finished copy to the system clipboard, carrying what was copied so the footer can
// name it.
type copiedMsg struct {
	id  string
	err error
}

// reviewModel is the Review tab: the whole harvested history, the window over it, and the tree's
// state. Like Repos, the tree is rebuilt from these on demand so a window and a query can never leave
// the list and the cursor describing different things.
type reviewModel struct {
	sessions []sessions.Session
	paths    map[string]string
	loadedAt time.Time

	// window is the segment of the range row that is selected, and date is the day typed at the prompt.
	// The date outlives a move away from its segment, so the row can be cycled back to it.
	window reviewRange
	date   string

	cursor   int
	expanded map[string]bool

	// preview is whether the transcript pane is open. It is closed until space opens it, and its
	// geometry is the width rule rather than a setting of its own (§5).
	preview bool

	// previewTop is the first line of the transcript the pane is showing. It survives the pane being
	// closed and reopened on the same session, so space is a glance away from a place in a long read
	// rather than a return to the top of it.
	previewTop int

	// previewId is the session the pane is showing, with the turns read for it and the reason the read
	// failed. reading is a read in flight, which is what separates a transcript with no turns from one
	// that has not been read yet.
	previewId  string
	turns      []sessions.Turn
	previewErr string
	reading    bool

	// load is the history seam. It runs off the update loop and comes back as a reviewLoadedMsg. A full
	// read re-parses every transcript on disk; an incremental one re-reads only the transcripts whose
	// stats moved since the last sync.
	load func(full bool) tea.Msg

	// syncing is a read in flight. It is what keeps the tab bar turning, since a full read re-parses
	// every transcript on disk, which is the longest the app ever waits on a read.
	syncing bool

	// readTurns is the transcript seam. A transcript is a file rather than a network call, but it is
	// the largest read the app makes, so it runs off the update loop like every other one (§6).
	readTurns func(path string) ([]sessions.Turn, error)
}

// reviewLoader is the real read: the transcripts on disk, synced into the session cache. The app is
// the one caller dispatch does not sync for, so a read that only loaded the cache would show a
// history as old as the last command run in a shell. The sync does not reach the network, but it is
// files, so it is read off the update loop like every other read (§6).
func reviewLoader(opts Options) func(full bool) tea.Msg {
	return func(full bool) tea.Msg {
		cached, note := syncedSessions(opts.Cfg, full)
		return reviewLoadedMsg{
			sessions: harvested(cached),
			paths:    transcriptPaths(cached),
			at:       time.Now(),
			note:     note,
		}
	}
}

// syncedSessions rebuilds the session cache from the transcripts on disk and returns what it holds. A
// sync that fails is not worth failing the read over: the cached sessions are still a history, so
// they are shown and the reason is carried back with them, the way the CLI notes a failed sync rather
// than refusing the command (§7).
func syncedSessions(cfg config.Config, full bool) ([]sessions.CachedSession, string) {
	result, err := sessions.Sync(cfg, sessions.SyncOptions{Full: full})
	if err != nil {
		return sessions.Load(cfg).Sessions, fmt.Sprintf("session sync failed (%v); showing the cached sessions", err)
	}

	// Only the explicit refresh says what it did. A sync nobody asked for has nothing to report beyond
	// the history it just put on screen.
	if full {
		return result.Sessions, "re-read " + plural(result.Stats.Parsed, "transcript")
	}
	return result.Sessions, ""
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

// refreshReview re-reads the history and sets the tab bar turning. The window and the query are
// answered from what is already loaded, so this is only ever an explicit refresh or a return from a
// suspension. A press while a read is already in flight is ignored rather than queued.
func (m Model) refreshReview(full bool) (Model, tea.Cmd) {
	if m.review.load == nil || m.review.syncing {
		return m, nil
	}

	m.review.syncing = true
	m.progress = m.spinnerText()
	return m, tea.Batch(reviewCmd(m.review.load, full), spinnerTick())
}

// reviewCmd is the history seam as a command, since the seam takes an argument and a tea.Cmd takes
// none.
func reviewCmd(load func(full bool) tea.Msg, full bool) tea.Cmd {
	return func() tea.Msg { return load(full) }
}

// reviewLoaded takes a finished read, pulling the cursor back into range in case the history came
// back shorter than the list was showing. The pane follows it, since a reload can move the row the
// cursor was resting on and can have grown the transcript it is showing.
func (m Model) reviewLoaded(msg reviewLoadedMsg) (Model, tea.Cmd) {
	m.review.sessions, m.review.loadedAt = msg.sessions, msg.at
	m.review.paths = msg.paths
	m.review.syncing = false
	m.progress = m.progressText()
	if msg.note != "" {
		m.hint = msg.note
	}

	// The pane is pointed at nothing before the cursor is resolved, so a refresh re-reads the transcript
	// it is showing rather than standing on the turns a session had when it was first opened. The place
	// in the read is kept across the re-read, since a transcript that grew is one being followed.
	showing, top := m.review.previewId, m.review.previewTop
	m.review.previewId = ""

	tree := m.reviewTree(m.viewWidth())
	m.review.cursor = tree.Cursor
	m, cmd := m.previewSync(tree)
	if m.review.previewId == showing {
		m.review.previewTop = top
	}
	return m, cmd
}

// reviewKey handles the keys the chrome does not own while Agents is the live tab. The actions are
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
	case "pgup", "pgdown", "home", "end":
		return m.previewPage(pressed)
	case "s":
		m.review.window = (m.review.window + 1) % reviewRange(len(m.reviewSegments()))
		m.review.cursor = 0
		return m, nil, true
	case "d":
		m.prompt = prompt{kind: promptDate, label: "date", subject: "(YYYY-MM-DD)"}
		return m, nil, true
	case "r":
		// The refresh is full: the cache skips a transcript whose size and mtime have not moved, and the
		// press is what someone reaches for when the history on screen and the one on disk disagree.
		m, cmd := m.refreshReview(true)
		return m, cmd, true
	case "R":
		return m.resumeSelected(tree)
	case "o":
		return m.openSession(tree)
	case "y":
		return m.copySessionId(tree)
	default:
		return m, nil, false
	}

	m.review.cursor, m.review.expanded = tree.Cursor, tree.Expanded

	m, cmd := m.previewSync(tree)
	return m, cmd, true
}

// resumeSelected hands the session under the cursor back to Claude. The app suspends rather than
// embedding it: ExecProcess releases the terminal, runs Claude on the app's own stdio, and re-enters
// the alternate screen when it exits, so ctrl-c goes where the user expects (§5).
func (m Model) resumeSelected(tree Tree[sessions.Session]) (Model, tea.Cmd, bool) {
	session, ok := tree.SelectedItem()
	if !ok {
		m.hint = inertOn(tree, noSessions, "R resumes a session — this row is a project")
		return m, nil, true
	}
	return m, resumeProcess(resumeCommand(session)), true
}

// resumeCommand is the process a resume runs. It runs in the directory the session ran in, since
// Claude files its transcripts per project and a resume started elsewhere would not find the session.
func resumeCommand(session sessions.Session) *exec.Cmd {
	cmd := exec.Command("claude", "--resume", session.Id)
	if session.Project != nil {
		cmd.Dir = *session.Project
	}
	return cmd
}

// resumeProcess suspends the app for the length of the process. It is a variable so a test can watch
// what would be run rather than launch Claude from the test binary.
var resumeProcess = func(cmd *exec.Cmd) tea.Cmd {
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return resumedMsg{err: err} })
}

// resumed is the app back from a suspension. The session that was just worked in has grown prompts
// and turns the loaded read knows nothing about, so the history is re-read rather than left stale.
func (m Model) resumed(msg resumedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.hint = fmt.Sprintf("resume failed: %v", msg.err)
		return m, nil
	}
	return m.refreshReview(false)
}

// openSession opens the checkout the session under the cursor ran in, which is what a page-less tab
// has to open instead of a URL. A session whose transcript never recorded a cwd has nothing to open
// and says so (§4).
func (m Model) openSession(tree Tree[sessions.Session]) (Model, tea.Cmd, bool) {
	session, ok := tree.SelectedItem()
	if !ok {
		m.hint = inertOn(tree, noSessions, "o opens the repo a session ran in — this row is a project")
		return m, nil, true
	}
	if session.Project == nil {
		m.hint = "this session recorded no project to open"
		return m, nil, true
	}
	return m, opener(*session.Project), true
}

// copySessionId puts the id on the system clipboard, which is what resuming from another terminal or
// pasting into `waid transcript` needs. The copy runs off the update loop, since the clipboard is
// reached through a process of its own (§6).
func (m Model) copySessionId(tree Tree[sessions.Session]) (Model, tea.Cmd, bool) {
	session, ok := tree.SelectedItem()
	if !ok {
		m.hint = inertOn(tree, noSessions, "y copies a session id — this row is a project")
		return m, nil, true
	}

	id := session.Id
	return m, func() tea.Msg { return copiedMsg{id: id, err: copyText(id)} }, true
}

// copied reports what the copy did. It is a hint rather than a receipt: nothing reached the event
// log, so there is nothing to undo and nothing to replay on quit (§3.1).
func (m Model) copied(msg copiedMsg) Model {
	if msg.err != nil {
		m.hint = fmt.Sprintf("copy failed: %v", msg.err)
		return m
	}

	m.hint = "copied " + msg.id
	return m
}

// copyText hands text to the system clipboard through whatever the desktop copies with, trying each
// tool in turn since a Linux session may be running either display protocol. It is a variable so a
// test can watch it rather than write to the real clipboard.
var copyText = func(text string) error {
	tools := clipboardCommands()
	for _, argv := range tools {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	names := make([]string, 0, len(tools))
	for _, argv := range tools {
		names = append(names, argv[0])
	}
	return fmt.Errorf("no clipboard tool available (tried %s)", strings.Join(names, ", "))
}

// clipboardCommands are the copy commands to try, in the order a desktop is likely to answer them.
func clipboardCommands() [][]string {
	switch runtime.GOOS {
	case "windows":
		return [][]string{{"clip"}}
	case "darwin":
		return [][]string{{"pbcopy"}}
	default:
		return [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
}

// previewPage moves the pane's window over the transcript. The extent is measured from the lines the
// pane is drawing at the size it is drawn in, so a page is a page of what is on screen and the window
// cannot be moved past the end of a transcript however long the terminal is left held down.
func (m Model) previewPage(pressed string) (Model, tea.Cmd, bool) {
	if !m.review.preview {
		m.hint = keyLabel(pressed) + " scrolls the transcript — press space to open the preview"
		return m, nil, true
	}

	rows := max(m.previewRows(), 1)
	page := max(int(float64(rows)*pageShare), 1)
	last := max(len(m.turnLines(m.previewWidth()))-rows, 0)

	top := m.review.previewTop
	switch pressed {
	case "pgup":
		top -= page
	case "pgdown":
		top += page
	case "home":
		top = 0
	case "end":
		top = last
	}

	m.review.previewTop = min(max(top, 0), last)
	return m, nil, true
}

// previewWidth is the columns the pane is drawn in: the split's detail column when the terminal is
// wide enough for one, and the whole body less its gutter when it is not (§5).
func (m Model) previewWidth() int {
	if panes := Split(m.viewWidth()); panes.SideBySide {
		return panes.Detail
	}
	return m.viewWidth() - previewMargin
}

// previewRows is the rows the transcript itself is drawn in, once the tab's own header and the
// session heading pinned above the turns have taken theirs.
func (m Model) previewRows() int {
	return m.bodyRows() - reviewHeadRows - previewHeadRows
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
	m.review.reading, m.review.previewTop = true, 0
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
	case rangeYesterday:
		return render.DayBounds(render.LocalYmd(now.AddDate(0, 0, -1)))
	case rangeWeek:
		return render.WeekBounds(now, 0)
	case rangeLastWeek:
		return render.WeekBounds(now, -1)
	case rangeDate:
		if m.review.date != "" {
			return render.DayBounds(m.review.date)
		}
	}
	return render.DayBounds(render.LocalYmd(now))
}

// reviewSegments are the range row's labels: the fixed windows, and the picked day after them once
// one has been typed. The row's length is what s cycles over, so a tab with no date picked cycles
// the four fixed windows alone.
func (m Model) reviewSegments() []string {
	segments := slices.Clone(reviewRangeLabels[:])
	if m.review.date != "" {
		segments = append(segments, m.review.date)
	}
	return segments
}

// pickDate moves the window onto a typed calendar day. A day the layout does not accept leaves the
// window where it was: an empty window would read as a day with no work in it rather than as a
// mistyped date (§4).
func (m Model) pickDate(typed string) Model {
	parsed, ok := render.ParseYmd(typed)
	if !ok {
		m.hint = fmt.Sprintf("%q is not a date — use YYYY-MM-DD", typed)
		return m
	}

	m.review.date = render.LocalYmd(parsed)
	m.review.window = rangeDate
	m.review.cursor = 0
	return m
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

// reviewTree builds the tree from the current state. Groups are closed by default, which is the whole
// reason Agents reads better than the command it replaces (§1.3).
func (m Model) reviewTree(width int) Tree[sessions.Session] {
	visible := m.reviewVisible()
	columns := reviewColumnsFor(visible)

	tree := Tree[sessions.Session]{
		Rows:     reviewRows(visible),
		Cursor:   m.review.cursor,
		Expanded: m.review.expanded,
		Render: func(session sessions.Session, width int, _ bool) string {
			return reviewRow(session, columns, width)
		},
		// A project heading is chrome here rather than a row to act on, so the cursor steps over the
		// ones it can see under (§7).
		Selectable: func(row Row[sessions.Session]) bool { return row.Depth > 0 },
	}
	tree.clamp()
	return tree
}

// reviewRows files the sessions under their projects and flattens the result into the tree's rows,
// busiest project first so the window reads as where the time went; ties fall back to name, matching
// how `waid today` orders the same rollup.
func reviewRows(visible []sessions.Session) []Row[sessions.Session] {
	var keys []string
	filed := map[string][]sessions.Session{}

	for _, session := range visible {
		key := reviewGroupKey(session)
		if _, known := filed[key]; !known {
			keys = append(keys, key)
		}
		filed[key] = append(filed[key], session)
	}

	slices.SortStableFunc(keys, func(a, b string) int {
		if left, right := promptsIn(filed[a]), promptsIn(filed[b]); left != right {
			return right - left
		}
		return project.CompareNames(filed[a][0].Project, filed[b][0].Project)
	})

	var rows []Row[sessions.Session]
	for _, key := range keys {
		under := filed[key]
		rows = append(rows, Row[sessions.Session]{
			Depth: 0,
			Key:   key,
			Title: key,
			Meta:  reviewGroupMeta(under),
		})
		for _, session := range under {
			rows = append(rows, Row[sessions.Session]{Node: session, Depth: 1, node: true})
		}
	}
	return rows
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

	head := []string{
		m.theme.Heading.Render(truncate(session.Title, width)),
		m.theme.Meta.Render(truncate(previewSpan(session), width)),
		m.theme.Meta.Render(truncate(session.Id, width)),
		"",
	}

	var body []string
	switch {
	case m.review.previewErr != "":
		body = m.previewNote(m.review.previewErr, width)
	case m.review.reading:
		body = m.previewNote("reading the transcript…", width)
	case len(m.review.turns) == 0:
		body = m.previewNote("(no turns)", width)
	default:
		body = m.turnLines(width)
	}

	// The heading is pinned so a pane scrolled into the middle of a long transcript still says which
	// session it is showing. A pane too short to hold the heading at all cuts that instead.
	rows := height - len(head)
	if height > 0 && rows <= 0 {
		return window(head, 0, height, m.theme)
	}
	return append(head, window(body, m.review.previewTop, rows, m.theme)...)
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

// window is the slice of a block the pane is showing, marked at whichever end it was cut so a block
// scrolled into the middle of does not read as a short one. The marks ride on the block's own rows
// rather than in chrome beside it, so a scrolled pane is the same shape as one that fits. A height of
// zero is a terminal that has not said how tall it is, and cuts nothing.
func window(lines []string, top, height int, theme Theme) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}

	// The offset is clamped here as well as where it is moved, since a resize can shorten the block
	// under an offset that was in range when the key was pressed.
	top = min(max(top, 0), len(lines)-height)
	shown := slices.Clone(lines[top : top+height])

	// A mark replaces a row of the block, so it is dropped rather than crowding the last of the content
	// out of a pane with only a row or two to give. The mark below outlives the one above, since a pane
	// that has not been scrolled yet is the commoner one and the end of the block is the news.
	mark := theme.Dim.Render("…")
	if top+height < len(lines) && height >= 2 {
		shown[len(shown)-1] = mark
	}
	if top > 0 && height >= 3 {
		shown[0] = mark
	}
	return shown
}

// reviewHeader is the segmented range row with the window's totals opposite it (§1.3).
func (m Model) reviewHeader(width int) string {
	row, rowWidth := m.segmentRow(m.reviewSegments(), int(m.review.window))
	return m.headerLine(row, rowWidth, m.reviewTotals(), width)
}

// reviewTotals is what the header says on the right: the window's sessions and the prompts they
// took. The tab lists sessions and nothing else, so a total counting anything else has nothing under
// it to look at.
func (m Model) reviewTotals() string {
	visible := m.reviewVisible()

	return plural(len(visible), "session") + " · " + plural(promptsIn(visible), "prompt")
}
