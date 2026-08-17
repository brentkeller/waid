package tui

import (
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
	// at is when the read ran.
	at time.Time
}

// reviewModel is the Review tab: the whole harvested history, the window over it, and the tree's
// state. Like Scan, the tree is rebuilt from these on demand so a window and a query can never leave
// the list and the cursor describing different things.
type reviewModel struct {
	sessions []sessions.Session
	closures []closure
	loadedAt time.Time

	// window is the segment of the range row that is selected.
	window reviewRange

	cursor   int
	expanded map[string]bool

	// load is the history seam. It runs off the update loop and comes back as a reviewLoadedMsg.
	load func() tea.Msg
}

// reviewLoader is the real read: the session cache and the closures the log holds. Neither reaches
// the network, but both are files, so they are read off the update loop like every other read (§6).
func reviewLoader(opts Options) func() tea.Msg {
	return func() tea.Msg {
		return reviewLoadedMsg{
			sessions: harvested(opts.Cfg),
			closures: closures(opts.Cfg),
			at:       time.Now(),
		}
	}
}

// harvested drops the cache's stat fields, which are an implementation detail of syncing rather than
// part of a session's history.
func harvested(cfg config.Config) []sessions.Session {
	cached := sessions.Load(cfg).Sessions

	harvest := make([]sessions.Session, 0, len(cached))
	for _, session := range cached {
		harvest = append(harvest, session.Session)
	}
	return harvest
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
// back shorter than the list was showing.
func (m Model) reviewLoaded(msg reviewLoadedMsg) Model {
	m.review.sessions, m.review.closures, m.review.loadedAt = msg.sessions, msg.closures, msg.at

	tree := m.reviewTree(m.viewWidth())
	m.review.cursor = tree.Cursor
	return m
}

// reviewKey handles the keys the chrome does not own while Review is the live tab. The preview and
// the actions are not wired yet; they are bound in the key table, so they are silent rather than
// reported as unbound.
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
	return m, nil, true
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
// them, and the tree.
func (m Model) reviewBody(width int) string {
	body := []string{m.reviewHeader(width), m.theme.Divider.Render(strings.Repeat("─", max(width, 0)))}

	tree := m.reviewTree(width)
	if tree.Len() == 0 {
		body = append(body, m.theme.Dim.Render("  "+tabEmpty[tabReview]))
	} else {
		body = append(body, tree.View(width, m.theme))
	}
	return strings.Join(body, "\n")
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
