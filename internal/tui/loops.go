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

// The detail pane: the gutter its lines hang from, the column a note's text hangs from, and the
// fewest rows it will draw itself in before the list's share of the body decides.
const (
	loopsDetailIndent = 1
	loopsNoteIndent   = 2
	loopsDetailMin    = 4
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

	// collapsed is whether the detail pane has been folded away. The pane is open otherwise: §1.1 draws
	// it under the list, and `p` collapses it when density matters more.
	collapsed bool

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
	case "p":
		m.loops.collapsed = !m.loops.collapsed
		return m, nil, true
	case "s":
		m.loops.status = nextLoopsStatus(m.loops.status)
		m.loops.cursor = 0
		return m, nil, true
	case "r":
		m, cmd := m.refreshLoops()
		return m, cmd, true
	case "x":
		return m.doneSelected(tree)
	case "w":
		return m.waitingSelected(tree)
	case "e":
		return m.editSelected(tree)
	case "n":
		return m.noteSelected(tree)
	case "P":
		return m.projectPrompt(tree)
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
	for _, optional := range []*string{item.Origin, item.WaitingOn} {
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
// rather than history, so what is owed is readable at a glance the way Repos' signals are (§1.2).
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

// loopsBody is the tab between the bar and the footer: the counts, the rule under them, the tree, and
// the detail pane under it while it is open (§1.1). The height is the rows the body was given, and is
// zero until the terminal has said how tall it is.
func (m Model) loopsBody(width, height int) string {
	head := []string{m.loopsHeader(width), m.theme.Divider.Render(strings.Repeat("─", max(width, 0)))}
	list := m.loopsList(width)
	if m.loops.collapsed {
		return strings.Join(slices.Concat(head, list), "\n")
	}

	rows := height - len(head)
	pane := m.detailPane(width, rows)
	// The pane hangs off the bottom of the body rather than following the last row of the list, so the
	// list does not shift under the cursor as the item it is pointed at grows notes.
	if grow := rows - len(list) - len(pane); grow > 0 {
		list = append(list, make([]string, grow)...)
	}
	return strings.Join(slices.Concat(head, list, pane), "\n")
}

// loopsList is the tree's lines, or what the tab says when nothing is owed.
func (m Model) loopsList(width int) []string {
	tree := m.loopsTree(width)
	if tree.Len() == 0 {
		return []string{m.theme.Dim.Render("  " + tabEmpty[tabLoops])}
	}
	return strings.Split(tree.View(width, m.theme), "\n")
}

// detailPane is the strip under the list: a rule, then `show`'s data for the row the cursor is on. It
// takes at most half the rows the body has, so a long title or a run of notes narrows the pane rather
// than squeezing the list out of the tab.
func (m Model) detailPane(width, rows int) []string {
	limit := 0
	if rows > 0 {
		limit = max(rows/2, loopsDetailMin)
	}

	pane := []string{m.theme.Divider.Render(strings.Repeat("─", max(width, 0)))}
	gutter := strings.Repeat(" ", loopsDetailIndent)
	for _, line := range window(m.detailLines(width-loopsDetailIndent), 0, limit, m.theme) {
		pane = append(pane, strings.TrimRight(gutter+line, " "))
	}
	return pane
}

// detailLines is what the pane holds: the line that identifies the item, its title in full, and the
// notes it has collected. A row that is not an item — a collapsed project fold — says so rather than
// leaving the last item the cursor rested on standing (§4).
func (m Model) detailLines(width int) []string {
	if width <= 0 {
		return nil
	}

	item, selected := m.loopsTree(m.viewWidth()).SelectedItem()
	if !selected {
		return []string{m.theme.Dim.Render(truncate("select an item to see its detail", width))}
	}

	lines := []string{m.theme.Meta.Render(truncate(itemIdentity(item, m.now()), width)), ""}
	for _, line := range wrap(item.Title, width) {
		lines = append(lines, m.theme.Row.Render(line))
	}
	return append(lines, m.noteLines(item, width)...)
}

// itemIdentity is the pane's first line: the id, the project, the status with whoever the item waits
// on, and how long it has been open. The status is joined the way `waid show` joins it rather than
// laid out the way the list's meta column is, since this line is prose.
func itemIdentity(item events.Item, now time.Time) string {
	status := string(item.Status)
	if item.WaitingOn != nil {
		status += " ← " + *item.WaitingOn
	}

	parts := []string{item.Id, projectLabel(item.Origin), status, "created " + agoPhrase(item.Created, now)}
	return strings.Join(parts, " · ")
}

// projectLabel names a project the way a line of prose has room for: the directory the checkout sits
// in, since the heading the pane hangs under already carries the whole path. The separator is either
// platform's, because the log holds the path the machine that wrote it used.
func projectLabel(path *string) string {
	if path == nil {
		return noProject
	}
	if at := strings.LastIndexAny(*path, `/\`); at >= 0 && at < len(*path)-1 {
		return (*path)[at+1:]
	}
	return *path
}

// noteLines is the notes block: every note the item carries, each hung off its own age, with the
// block left out entirely when there are none — the way `waid show` leaves it out.
func (m Model) noteLines(item events.Item, width int) []string {
	if len(item.Notes) == 0 {
		return nil
	}

	now := m.now()
	age := 0
	for _, note := range item.Notes {
		age = max(age, lipgloss.Width(render.RelTime(note.Ts, now)))
	}

	lines := []string{"", m.theme.Heading.Render("notes")}
	for _, note := range item.Notes {
		label := strings.Repeat(" ", loopsNoteIndent) + padLeft(render.RelTime(note.Ts, now), age) +
			strings.Repeat(" ", loopsColumnGap)
		hang := strings.Repeat(" ", lipgloss.Width(label))

		for i, line := range wrap(note.Text, width-lipgloss.Width(label)) {
			prefix := label
			if i > 0 {
				prefix = hang
			}
			lines = append(lines, m.theme.Row.Render(prefix+line))
		}
	}
	return lines
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
