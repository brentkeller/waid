package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/tree"
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

	// headings is whether a heading with nothing visible under it is drawn. It is an axis of its own
	// rather than a segment of the status row, so it composes with every status; the zero value hides
	// them, and `S` flips it for the length of the session (§3).
	headings bool

	// collapsed is whether the detail pane has been folded away. The pane is open otherwise: §1.1 draws
	// it under the list, and `p` collapses it when density matters more.
	collapsed bool

	cursor   int
	expanded map[string]bool

	// moving is the destination picker, open over the list while an item is being moved (§7.1). Its
	// zero value is the mode closed, which is every frame but those.
	moving moveModel

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
	case "h", "left":
		tree.Collapse()
	case "l", "right":
		tree.Expand()
	case "p":
		m.loops.collapsed = !m.loops.collapsed
		return m, nil, true
	case "s":
		m.loops.status = nextLoopsStatus(m.loops.status)
		m.loops.cursor = 0
		return m, nil, true
	case "S":
		m.loops.headings = !m.loops.headings
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
	case "m":
		return m.startMove(tree)
	case "P":
		return m.headingSelected(tree)
	default:
		return m, nil, false
	}

	m.loops.cursor, m.loops.expanded = tree.Cursor, tree.Expanded
	return m, nil, true
}

// owed is everything the log still holds against the user, which is what `waid loops` means by an
// open loop and what the tab's badge carries. The status row and the headings toggle narrow the list
// without touching it: a filter narrows the view and not the work.
//
// It is counted off a tree rather than off the flat list so a heading with no open descendants is
// left out — the badge is what is owed, and an empty shelf is owed nothing (§4). The gate keeps the
// closed ancestors that lead down to open work, so what survived is filtered a second time to leave
// them out of the count.
func (m Model) owed() []events.Item {
	roots, _ := tree.Build(events.State{Items: m.loops.items})
	roots = tree.Prune(roots, func(item events.Item) bool { return item.Status != events.StatusDone })

	var kept []events.Item
	for _, item := range itemsIn(tree.DropEmptyHeadings(roots)) {
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

// loopsForest is the tree the list draws: the loaded log nested, gated by the status row, narrowed
// by the typed query, and bucketed last so (unassigned) is computed against what survived (§4).
//
// The two filters are applied separately because they mean different things: the status row gates
// each row in its own right, so a closed row under an open parent still goes, while the typed query
// is a question — a heading that answers it is asked for along with everything under it, and one
// that does not is kept only as the path down to a descendant that did. Both filter client-side, so
// neither issues any work (§1.1).
func (m Model) loopsForest() []tree.Node {
	return m.forestOf(m.loops.items)
}

// forestOf is that shaping over whatever set of items it is handed, so the move picker draws the
// tree the list draws — the same gates, the same ordering — over what is left once the subtree being
// moved has been lifted out of it (§7.1).
// The empty headings are dropped here rather than in the shaping the picker shares, since the picker
// offers every heading whatever the toggle says (§3). The pass runs before the bucketing, so a level
// whose only head was a dropped heading is a level of nothing but leaves and renders them directly.
func (m Model) forestOf(items []events.Item) []tree.Node {
	roots := m.shapedForest(items)
	if !m.loops.headings {
		roots = tree.DropEmptyHeadings(roots)
	}
	return tree.Bucket(roots)
}

// shapedForest is that shaping short of the bucketing, which is the one thing the two trees do
// differently: the picker buckets a level lower, since its top level holds destinations rather than
// headings (§7.1).
func (m Model) shapedForest(items []events.Item) []tree.Node {
	roots, _ := tree.Build(events.State{Items: items})
	roots = tree.Prune(roots, m.matchesStatus)

	if query := strings.ToLower(m.filter); query != "" {
		roots = tree.Filter(roots, func(item events.Item) bool { return matchesItem(item, query) })
	}
	return roots
}

// loopsVisible are the items the list is showing, read off the tree rather than filtered a second
// time so the count in the header and the rows under it can never disagree.
func (m Model) loopsVisible() []events.Item {
	return itemsIn(m.loopsForest())
}

// itemsIn flattens a forest to the logged items in it, in the order the rows are drawn. The
// synthetic bucket is left out: it is a rendering artifact rather than an item (§4).
func itemsIn(nodes []tree.Node) []events.Item {
	var items []events.Item
	for _, node := range nodes {
		if !node.Synthetic {
			items = append(items, node.Item)
		}
		items = append(items, itemsIn(node.Children)...)
	}
	return items
}

// leavesIn are the nodes of a forest drawn as item rows rather than as folds, which is the set the
// row columns are measured across: a fold draws its title and its count and none of the columns, and
// so does a heading holding nothing.
func leavesIn(nodes []tree.Node) []events.Item {
	var leaves []events.Item
	for _, node := range nodes {
		if len(node.Children) > 0 {
			leaves = append(leaves, leavesIn(node.Children)...)
			continue
		}
		if !node.Synthetic && !node.Item.Heading {
			leaves = append(leaves, node.Item)
		}
	}
	return leaves
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

// loopsTree builds the tree from the current state. Folds open by default: Loops is a working list
// rather than history, so what is owed is readable at a glance the way Repos' signals are (§1.2).
func (m Model) loopsTree(width int) Tree[events.Item] {
	now := m.now()
	roots := m.loopsForest()
	columns := loopsColumnsFor(leavesIn(roots), now)

	built := Tree[events.Item]{
		Rows:              loopsRows(roots, "", 0),
		Cursor:            m.loops.cursor,
		Expanded:          m.loops.expanded,
		ExpandedByDefault: true,
		// Loops selects every row, headings included: its headings are items, and everything the keys
		// ask of a row — close it, note against it, retitle it — is something an item answers (§7).
		Selectable: func(Row[events.Item]) bool { return true },
		Render: func(item events.Item, width int, _ bool) string {
			return loopsRow(item, columns, now, width)
		},
	}
	built.clamp()
	return built
}

// loopsRows flattens the forest into the tree's rows in reading order. Folds are keyed by item id,
// so one stays open across a reload however the level it sits on has been re-sorted; the bucket
// holds no id, so it is keyed by the fold it hangs under.
func loopsRows(nodes []tree.Node, parent string, depth int) []Row[events.Item] {
	var rows []Row[events.Item]
	for _, node := range nodes {
		key := node.Item.Id
		if node.Synthetic {
			key = parent + "/" + tree.UnassignedTitle
		}

		rows = append(rows, Row[events.Item]{
			Node:  node.Item,
			Depth: depth,
			Key:   key,
			Title: node.Item.Title,
			Meta:  foldMeta(node),
			node:  !node.Synthetic,
			// A heading holding nothing is drawn as a heading line all the same, and a marker beside
			// it would offer a fold that is not there (§4).
			plain: tree.EmptyHeading(node),
		})
		rows = append(rows, loopsRows(node.Children, key, depth+1)...)
	}
	return rows
}

// foldMeta is the count a fold carries: how many open loops sit beneath it. An ordinary row with
// nothing under it is drawn as an item rather than as a fold, so it carries none; a heading is drawn
// as a heading line either way, and says `0 open` when it holds nothing (§4).
func foldMeta(node tree.Node) string {
	if len(node.Children) == 0 && !node.Item.Heading {
		return ""
	}
	return fmt.Sprintf("%d open", openUnder(node))
}

// openUnder is the work a fold holds: the still-owed rows beneath it that are leaves. A heading is a
// landmark over work rather than work of its own — it cannot be closed while anything under it is
// open (§5) — so counting the headings as well would count the same loop again at every level it
// hangs under, and an empty one would be counted as work nobody owes.
func openUnder(node tree.Node) int {
	if len(node.Children) == 0 {
		if node.Synthetic || node.Item.Heading || node.Item.Status == events.StatusDone {
			return 0
		}
		return 1
	}

	count := 0
	for _, child := range node.Children {
		count += openUnder(child)
	}
	return count
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
	if m.loops.moving.active() {
		return m.moveBody(width)
	}

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

	parts := []string{item.Id, projectLabel(item.Origin), status}
	// A heading is a landmark over work rather than a state the item is in, so it is named beside the
	// status rather than folded into it — the shape `waid show` prints (§4).
	if item.Heading {
		parts = append(parts, "heading")
	}
	return strings.Join(append(parts, "created "+agoPhrase(item.Created, now)), " · ")
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
	chip, chipWidth := m.loopsHeadingsChip()
	return m.headerLine(row+chip, rowWidth+chipWidth, m.loopsCounts(), width)
}

// loopsHeadingsChip draws the toggle that reveals the headings holding nothing (§6.1). It is a
// switch of its own rather than another status, so it sits behind the rule the tab bar divides its
// zones with and is spelled the way a segment is. The printed width comes back alongside it, since
// the styling makes the string longer than the columns it occupies.
func (m Model) loopsHeadingsChip() (string, int) {
	label, style := "headings", m.theme.FilterInactive
	if m.loops.headings {
		label, style = "‹headings›", m.theme.FilterActive
	}

	// The gap either side of the rule, and the rule itself, are the columns the label is drawn past.
	const chrome = 5
	return "  " + m.theme.TabRule.Render("│") + "  " + style.Render(label), chrome + lipgloss.Width(label)
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

// loopsCounts is what the header says on the right: how many rows the list is showing, headings
// counted with the rest since a heading is an item like any other (§1).
func (m Model) loopsCounts() string {
	return plural(len(m.loopsVisible()), "item")
}
