package tui

import (
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
	"github.com/brentkeller/waid/internal/render"
)

// loopsStatus is a segment of the status row: either one status an item can hold, or one of the two
// spans over several. The zero value is what the tab opens on.
type loopsStatus string

const (
	// loopsOwed is everything the log still holds against the user, open and waiting alike. It is what
	// `waid loops` means by an open loop and what the tab's badge counts, so the segment is drawn `open`.
	loopsOwed    loopsStatus = ""
	loopsWaiting loopsStatus = loopsStatus(events.StatusWaiting)
	loopsDone    loopsStatus = loopsStatus(events.StatusDone)
	loopsAll     loopsStatus = "all"
)

// loopsStatuses are the segments of the status row, in the order §1.1 draws them.
var loopsStatuses = []loopsStatus{loopsOwed, loopsWaiting, loopsDone, loopsAll}

// Column widths for an item row. The id, the status and the age size themselves to the data; the meta
// column carrying the tags and who the item waits on is dropped before the title is squeezed below
// what a title needs to be recognised.
const (
	loopsColumnGap = 2
	loopsMetaMax   = 24
	loopsMetaMin   = 6
	loopsTitleMin  = 24
)

// loopsLoadedMsg carries a finished read of the event log back into the update loop. Every read is a
// tea.Cmd, so Update never waits on the filesystem (§6).
type loopsLoadedMsg struct {
	items []events.Item
	// at is when the read ran.
	at time.Time
}

// loopsModel is the Loops tab: the folded log and the tree's state over it. The tree is rebuilt from
// these on demand, so a filter can never leave the list and the cursor describing different things.
type loopsModel struct {
	items    []events.Item
	loadedAt time.Time

	// status is the segment of the status row that is selected; the zero value is what is owed.
	status loopsStatus

	cursor   int
	expanded map[string]bool

	// load is the log seam. It runs off the update loop and comes back as a loopsLoadedMsg.
	load func() tea.Msg
}

// loopsLoader is the real read: the whole log folded into the items it declares. The log is a local
// file rather than a network call, but it is read off the update loop like every other read (§6).
func loopsLoader(opts Options) func() tea.Msg {
	return func() tea.Msg {
		return loopsLoadedMsg{items: events.Load(opts.Cfg.EventsPath).Items, at: time.Now()}
	}
}

// refreshLoops re-reads the log, which is the only thing that brings in an item another session
// declared. The status row and the query are answered from what is already loaded.
func (m Model) refreshLoops() (Model, tea.Cmd) {
	if m.loops.load == nil {
		return m, nil
	}
	return m, m.loops.load
}

// loopsLoaded takes a finished read, moving the tab's badge with it and pulling the cursor back into
// range in case the log came back holding fewer items than the list was showing.
func (m Model) loopsLoaded(msg loopsLoadedMsg) Model {
	m.loops.items, m.loops.loadedAt = msg.items, msg.at
	m.counts[tabLoops] = len(m.owed())

	m.loops.cursor = m.loopsTree(m.viewWidth()).Cursor
	return m
}

// loopsKey handles the keys the chrome does not own while Loops is the live tab. It reports whether
// the press meant anything here.
func (m Model) loopsKey(pressed string) (Model, tea.Cmd, bool) {
	tree := m.loopsTree(m.viewWidth())

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
		m.loops.status = nextLoopsStatus(m.loops.status)
		m.loops.cursor = 0
		return m, nil, true
	case "r":
		m, cmd := m.refreshLoops()
		return m, cmd, true
	default:
		return m, nil, false
	}

	m.loops.cursor, m.loops.expanded = tree.Cursor, tree.Expanded
	return m, nil, true
}

// owed is everything the log still holds against the user, which is what `waid loops` means by an
// open loop and what the tab's badge carries. The status row narrows the list without touching it: a
// filter narrows the view and not the work.
func (m Model) owed() []events.Item {
	var kept []events.Item
	for _, item := range m.loops.items {
		if item.Status != events.StatusDone {
			kept = append(kept, item)
		}
	}
	return kept
}

// nextLoopsStatus advances the segmented row, wrapping back to what is owed.
func nextLoopsStatus(current loopsStatus) loopsStatus {
	for i, status := range loopsStatuses {
		if status == current {
			return loopsStatuses[(i+1)%len(loopsStatuses)]
		}
	}
	return loopsOwed
}

// matchesStatus is the status row's test over one item.
func (m Model) matchesStatus(item events.Item) bool {
	switch m.loops.status {
	case loopsAll:
		return true
	case loopsOwed:
		return item.Status != events.StatusDone
	default:
		return string(item.Status) == string(m.loops.status)
	}
}

// loopsVisible are the items the list is showing: the status row and the typed query applied over the
// loaded log, oldest touched first, which is the order `waid loops` prints them in. Both filter
// client-side, so neither issues any work (§1.1).
func (m Model) loopsVisible() []events.Item {
	query := strings.ToLower(m.filter)

	var kept []events.Item
	for _, item := range m.loops.items {
		if !m.matchesStatus(item) {
			continue
		}
		if query != "" && !matchesItem(item, query) {
			continue
		}
		kept = append(kept, item)
	}

	slices.SortStableFunc(kept, func(a, b events.Item) int { return strings.Compare(a.Updated, b.Updated) })
	return kept
}

// matchesItem is the query test: the columns the row can show, plus the id, so an id pasted from the
// CLI finds its row.
func matchesItem(item events.Item, query string) bool {
	fields := append([]string{item.Id, item.Title, string(item.Status)}, item.Tags...)
	for _, optional := range []*string{item.Project, item.WaitingOn} {
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

// loopsTree builds the tree from the current state. Groups open by default: Loops is a working list
// rather than history, so what is owed is readable at a glance the way Scan's signals are (§1.2).
func (m Model) loopsTree(width int) Tree[events.Item] {
	now := m.now()
	visible := m.loopsVisible()
	columns := loopsColumnsFor(visible, now)

	tree := Tree[events.Item]{
		Groups:            loopsGroups(visible),
		Cursor:            m.loops.cursor,
		Expanded:          m.loops.expanded,
		ExpandedByDefault: true,
		Render: func(item events.Item, width int, _ bool) string {
			return loopsRow(item, columns, now, width)
		},
	}
	tree.clamp()
	return tree
}

// loopsGroups files the items under their projects through the same grouping the list and loops
// commands print, so the app and the CLI never disagree about which project an item belongs to.
func loopsGroups(visible []events.Item) []Group[events.Item] {
	grouped := project.GroupByProject(visible)

	groups := make([]Group[events.Item], 0, len(grouped))
	for _, group := range grouped {
		key := noProject
		if group.Project != nil {
			key = *group.Project
		}
		groups = append(groups, Group[events.Item]{
			Key:   key,
			Title: key,
			Meta:  plural(len(group.Items), "item"),
			Items: group.Items,
		})
	}
	return groups
}

// loopsColumns are the widths a row's fixed columns share, measured across every visible item so the
// list reads down a column rather than across a row.
type loopsColumns struct{ id, status, meta, age int }

func loopsColumnsFor(visible []events.Item, now time.Time) loopsColumns {
	var columns loopsColumns
	for _, item := range visible {
		columns.id = max(columns.id, lipgloss.Width(item.Id))
		columns.status = max(columns.status, lipgloss.Width(string(item.Status)))
		columns.meta = max(columns.meta, lipgloss.Width(itemMeta(item)))
		columns.age = max(columns.age, lipgloss.Width(itemAge(item, now)))
	}

	columns.meta = min(columns.meta, loopsMetaMax)
	return columns
}

// itemMeta is the column between the title and the age: the tags the item carries, then who it is
// waiting on. Both are what an item was marked with rather than what it says, so they share a column.
func itemMeta(item events.Item) string {
	var parts []string
	if len(item.Tags) > 0 {
		parts = append(parts, "["+strings.Join(item.Tags, ",")+"]")
	}
	if item.WaitingOn != nil {
		parts = append(parts, "@"+*item.WaitingOn)
	}
	return strings.Join(parts, " ")
}

// itemAge is how long ago the item was last touched, which is what the CLI's own row prints.
func itemAge(item events.Item, now time.Time) string {
	return render.RelTime(item.Updated, now)
}

// loopsRow lays one item out across the width the tree left it: id, status, title, meta, and the age
// against the right edge.
func loopsRow(item events.Item, columns loopsColumns, now time.Time, width int) string {
	if width <= 0 {
		return ""
	}

	// The meta column gives its width up to the title before the title is squeezed below what a title
	// needs to be recognised, and is dropped outright once what is left of it says nothing.
	meta := columns.meta
	fixed := columns.id + columns.status + columns.age
	title := width - fixed - meta - loopsColumnGap*4
	if title < loopsTitleMin {
		meta -= loopsTitleMin - title
		title = loopsTitleMin
	}
	if meta < loopsMetaMin {
		meta, title = 0, width-fixed-loopsColumnGap*3
	}

	cells := []string{pad(item.Id, columns.id), pad(string(item.Status), columns.status), pad(item.Title, title)}
	if meta > 0 {
		cells = append(cells, pad(itemMeta(item), meta))
	}
	cells = append(cells, padLeft(itemAge(item, now), columns.age))

	gap := strings.Repeat(" ", loopsColumnGap)
	return strings.TrimRight(strings.Join(cells, gap), " ")
}

// loopsBody is the tab between the bar and the footer: the counts, the rule under them, and the tree.
func (m Model) loopsBody(width int) string {
	body := []string{m.loopsHeader(width), m.theme.Divider.Render(strings.Repeat("─", max(width, 0)))}

	tree := m.loopsTree(width)
	if tree.Len() == 0 {
		return strings.Join(append(body, m.theme.Dim.Render("  "+tabEmpty[tabLoops])), "\n")
	}
	return strings.Join(append(body, tree.View(width, m.theme)), "\n")
}

// loopsHeader is the segmented status row with the counts opposite it (§1.1).
func (m Model) loopsHeader(width int) string {
	row, rowWidth := m.segmentRow(loopsStatusLabels(), m.loopsSegment())
	return m.headerLine(row, rowWidth, m.loopsCounts(), width)
}

// loopsStatusLabels name the segments. What is owed is drawn `open`, since that is the word the CLI
// and the tab's badge already use for it.
func loopsStatusLabels() []string {
	labels := make([]string, 0, len(loopsStatuses))
	for _, status := range loopsStatuses {
		if status == loopsOwed {
			status = loopsStatus(events.StatusOpen)
		}
		labels = append(labels, string(status))
	}
	return labels
}

// loopsSegment is which of the segments is selected.
func (m Model) loopsSegment() int {
	return slices.Index(loopsStatuses, m.loops.status)
}

// loopsCounts is what the header says on the right: what is owed, and how many projects it is spread
// across.
func (m Model) loopsCounts() string {
	visible := m.loopsVisible()
	return plural(len(visible), "item") + " · " + plural(len(loopsGroups(visible)), "project")
}
