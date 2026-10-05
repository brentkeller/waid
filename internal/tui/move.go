package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/tree"
)

// Moving an item is navigation rather than typing: the tree is already on screen, and making someone
// type a title they can see is worse than letting them point at it. `m` lifts the item — or every
// marked item — and its descendants out of the tree, and what is left is the tree as it will be
// after the move, browsed
// with the ordinary keys. `enter` drops, `esc` cancels (§7.1).
//
// Lifting the subtree is what makes this safe rather than merely convenient: the app never has to
// reject a move, because the invalid destinations are not on screen to choose. An item cannot be
// dropped inside itself once its own descendants have left the tree with it. The cycle check in §6
// remains for the command line, where a fragment can still name a descendant.

// moveTopKey identifies the synthetic row that means no parent, and moveTopTitle is what it draws.
// The roots are items, so without the row there is nothing on screen meaning the top level — and a
// dedicated key would be one more thing to know for a destination the eye can already find.
const (
	moveTopKey   = "\x00top-level"
	moveTopTitle = "── top level ──"
)

// moveKeys are the picker's hints, in place of the tab's own while it holds the keyboard.
const moveKeys = "↑ ↓ move · ← → fold · enter drops · esc cancels"

// moveModel is the picker's state: what is being moved, and the cursor and folds over what is left.
// subjects is empty when the mode is closed, which is the zero value. It never holds an item that
// sits beneath another of its own, since the one above already carries it.
type moveModel struct {
	subjects []string
	cursor   int
	expanded map[string]bool

	// top is the first line of the picker on screen once it holds more than its rows.
	top int
}

// active reports whether the picker is open, which is when it holds the keyboard.
func (mm moveModel) active() bool { return len(mm.subjects) > 0 }

// startMove opens the picker on the marked items, or on the item under the cursor when nothing is
// marked. A row that holds no item is inert, as it is for every other write, and the footer says
// which row the key was pressed on (§4).
func (m Model) startMove(t Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, items, ok := m.targets(t, "m moves an item under another")
	if !ok {
		return m, nil, true
	}

	// The subjects are set first, since the folds and the cursor are read off the tree the mode draws,
	// and that tree is the one the subjects have already been lifted out of.
	subjects := m.topmost(items)
	if len(subjects) == 0 {
		// A log whose parents loop leaves every marked item beneath another, and nothing topmost.
		subjects = items
	}
	parent := sharedParent(subjects)
	m.loops.moving = moveModel{subjects: idsOf(subjects)}
	m.loops.moving.expanded = m.moveFolds(parent)
	m.loops.moving.cursor = m.moveCursor(parent)
	return m, nil, true
}

// moveFolds are the folds the picker opens on: every root, so the two levels a destination is
// usually picked from are both on screen, plus the path down to the parent the subjects share where it
// sits deeper than that. Anything below stays folded, which keeps a tree worth building to one
// screen without a search. The parent itself stays closed — it is where the cursor lands, and what
// it already holds is not the move's business.
func (m Model) moveFolds(parent *string) map[string]bool {
	open := map[string]bool{}
	for _, root := range m.moveForest() {
		open[root.Item.Id] = true
	}
	if parent == nil {
		return open
	}
	for _, ancestor := range (events.State{Items: m.loops.items}).Ancestors(*parent) {
		open[ancestor.Id] = true
	}
	return open
}

// moveCursor is the row the picker opens on: the parent the subjects share, so an immediate enter is a
// no-op rather than a surprise, and a nudge to a sibling project is one keystroke. An item at the
// top level opens on the row that means one, which is the first. Subjects sitting under different
// parents open on the top-level row, as an item at the top level does.
func (m Model) moveCursor(parent *string) int {
	if parent == nil {
		return 0
	}

	for i, row := range m.moveTree(m.viewWidth()).rows() {
		if row.Key == *parent {
			return i
		}
	}
	return 0
}

// lifted is everything that leaves the tree while the picker is open: the subjects and whatever
// hangs beneath each.
func (m Model) lifted() []string {
	var lifted []string
	for _, id := range m.loops.moving.subjects {
		lifted = append(append(lifted, id), m.descendants(id)...)
	}
	return lifted
}

// moveCandidates are the items left once the subjects and everything under them have been lifted out.
func (m Model) moveCandidates() []events.Item {
	lifted := m.lifted()

	kept := make([]events.Item, 0, len(m.loops.items))
	for _, item := range m.loops.items {
		if !slices.Contains(lifted, item.Id) {
			kept = append(kept, item)
		}
	}
	return kept
}

// moveForest is the tree the destinations are read off: the list's forest over what is left, except
// that the roots are never gathered into the (unassigned) bucket. A root with nothing under it is a
// destination in its own right — it is where an item goes to sit at the top level beside the other
// roots — and a bucket takes no drop, so filing them into one would put them out of reach (§7.1).
func (m Model) moveForest() []tree.Node {
	return tree.BucketBelow(m.shapedForest(m.moveCandidates()))
}

// moveTree is the destinations on offer: the tree with the lifted subtree gone, and the synthetic
// top-level row pinned above it. Folds are closed by default rather than open, which is the other
// way it differs from the list it stands in for.
func (m Model) moveTree(width int) Tree[events.Item] {
	now := m.now()
	roots := m.moveForest()
	columns := loopsColumnsFor(leavesIn(roots), now)

	top := Row[events.Item]{Depth: 0, Key: moveTopKey, Title: moveTopTitle, plain: true}

	built := Tree[events.Item]{
		Rows:     append([]Row[events.Item]{top}, loopsRows(roots, "", 0)...),
		Cursor:   m.loops.moving.cursor,
		Expanded: m.loops.moving.expanded,
		// Every row the cursor can reach is one the eye can land on, headings included: a closed fold
		// has to be selectable to be opened at all, and enter is what decides whether the row it
		// reached can take the item.
		Selectable: func(Row[events.Item]) bool { return true },
		Render: func(item events.Item, width int, _ bool) string {
			return loopsRow(item, columns, now, width)
		},
	}
	built.clamp()
	return built
}

// moveKey handles the keys while the picker holds the keyboard. Every other key is inert: the mode
// asks one question, and a stray `a` in the middle of it would be a write nobody asked for. ctrl-c
// still quits, as it does from a prompt.
func (m Model) moveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := m.moveTree(m.viewWidth())
	m.hint = ""

	switch msg.String() {
	case "esc":
		m.loops.moving = moveModel{}
		return m, nil
	case "enter":
		return m.commitMove(t)
	case "down":
		t.Down()
	case "up":
		t.Up()
	case "g":
		t.First()
	case "G":
		t.Last()
	case "left":
		t.Collapse()
	case "right":
		t.Expand()
	default:
		return m, nil
	}

	m.loops.moving.cursor, m.loops.moving.expanded = t.Cursor, t.Expanded
	return m, nil
}

// commitMove drops what is being moved on the row the cursor is on. The top-level row writes a null
// parent, and every row holding an item writes that item's id — leaves included, since dropping onto
// a leaf makes it a parent and there is no reason to forbid it.
//
// The `(unassigned)` bucket is not an item and so is not a destination, as everywhere else: dropping
// into it is dropping onto the fold it hangs under, which is already a row of its own. The picker
// stays open, since the press was a miss rather than an answer.
func (m Model) commitMove(t Tree[events.Item]) (Model, tea.Cmd) {
	row, ok := t.SelectedRow()
	if !ok {
		m.hint = noItems
		return m, nil
	}

	var parent *string
	switch {
	case row.Key == moveTopKey:
	case row.node:
		parent = &row.Node.Id
	default:
		m.hint = row.Title + " is not a destination — l opens it, or drop on the row above it"
		return m, nil
	}

	subjects := m.loops.moving.subjects
	m.loops.moving = moveModel{}
	return m.refile(subjects, parent)
}

// moveBody is the picker in place of the list: the header naming what is being moved, the rule under
// it, and the destinations. The detail pane is left out — the question the mode asks is where a row
// sits, not what it says.
func (m Model) moveBody(width, height int) string {
	head := []string{m.moveHeader(width), m.theme.Divider.Render(strings.Repeat("─", max(width, 0)))}
	tree := m.moveTree(width)
	list := strings.Split(tree.View(width, m.theme), "\n")
	if height > 0 {
		rows := m.loopsListRows(height)
		list = scrollLines(list, scrollTop(m.loops.moving.top, tree.CursorLine(), len(list), rows), rows)
	}
	return strings.Join(append(head, list...), "\n")
}

// moveHeader names what is being moved, how much comes with it, and the two keys that end the mode:
// one item by its id and title, several by their count. The title gives way first, since the id and
// the keys are the parts a narrow terminal cannot infer.
func (m Model) moveHeader(width int) string {
	subjects := m.loops.moving.subjects

	head := " moving"
	lead, title := "  "+plural(len(subjects), "item"), ""
	if len(subjects) == 1 {
		item, loaded := m.loadedItem(subjects[0])
		if !loaded {
			return ""
		}
		lead, title = "  "+item.Id+"  ", item.Title
	}

	carried := ""
	if n := len(m.lifted()) - len(subjects); n > 0 {
		carried = "  + " + kin(n)
	}

	fixed := lipgloss.Width(head+lead+carried) + lipgloss.Width(moveEnders) + 2
	rest := lead + truncate(title, max(width-fixed, 0)) + carried

	row := m.theme.Heading.Render(head) + m.theme.Row.Render(rest)
	return m.headerLine(row, lipgloss.Width(head+rest), moveEnders, width)
}

// moveEnders are the two keys the header hangs against its right edge: the mode has one way out in
// each direction, and both belong beside what is being moved.
const moveEnders = "esc cancels · enter drops"

// kin counts the descendants a lifted subtree carries. plural cannot inflect this one.
func kin(count int) string {
	if count == 1 {
		return "1 child"
	}
	return fmt.Sprintf("%d children", count)
}
