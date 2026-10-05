package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Repos and Agents are the same shape — a project heading with children folded under it — so they
// render the same component (§2). Two hand-written trees would drift within a month and each would
// look correct in isolation, so only the child row and the action set differ between the tabs.
const (
	foldOpen   = "▾"
	foldClosed = "▸"
	cursorMark = "▸"

	// markGlyph is drawn in the first column of a row that carries a mark. indent always leaves that
	// column blank, so the glyph meets neither the cursor mark nor the fold marker.
	markGlyph = "●"

	// headingIndent leaves a column before a top-level row's marker, and indentStep is what each
	// level below it moves in by. rowIndent is where a depth-1 child's text starts: one step past
	// the heading, plus the marker column and the space after it.
	headingIndent = 1
	indentStep    = 2
	rowIndent     = headingIndent + indentStep + 2

	// maxIndentDepth is the deepest generation that still earns an indent of its own. Past it the
	// nesting is already plain from the rows above, and the title column is worth more than the
	// depth-for-depth fidelity (§7).
	maxIndentDepth = 4
)

// Group is one project and the children folded under it. Key identifies the group across reloads,
// so an expanded fold stays expanded when the data behind it is refreshed. Meta is the count shown
// on the fold — the nouns differ between the tabs, so the tab supplies the wording.
type Group[T any] struct {
	Key   string
	Title string
	Meta  string
	Items []T
}

// Row is one line of the tree: the node it draws, how deep the line sits, whether anything is
// folded under it and whether that is on screen. The cursor is an index into these rows, so a tree
// of any depth is navigated the same way as a two-level one (§7).
type Row[T any] struct {
	Node     T
	Depth    int
	HasKids  bool
	Expanded bool

	// Key is what Expanded is keyed by, supplied by whatever built the row: a path for Repos and
	// Agents, an item id for Loops.
	Key string

	// Title and Meta are what a row with no item behind it draws. Repos and Agents head each fold
	// with a project, which is not an item and has only a name and a count.
	Title string
	Meta  string

	// parent is the index of the row this one is folded under, or -1 at the top level. at is where
	// the row sits in the whole tree, folds ignored. node reports whether Node holds anything: a
	// heading built from a Group has no item behind it.
	parent int
	at     int
	node   bool

	// plain draws a row as a heading line with the marker column left blank, for a row that heads a
	// level without holding anything visible: the move picker's top-level row is a destination
	// rather than a branch, and an empty heading is a landmark over work rather than work of its
	// own. A closed marker beside either would offer a fold that is not there.
	plain bool
}

// Tree is the collapsible list the tabs draw. It takes the tree as a flat list of rows and shows
// the ones no closed fold is hiding.
type Tree[T any] struct {
	// Rows is the whole tree in reading order, folded or not: the tab supplies Node, Depth, Key and
	// — for a row that is not an item — Title and Meta, and the tree derives the rest from where
	// each row sits. A tree supplying no rows is built from Groups instead.
	Rows []Row[T]

	Groups   []Group[T]
	Cursor   int
	Expanded map[string]bool

	// ExpandedByDefault decides a fold the Expanded map says nothing about. Repos opens its groups
	// so the signals are readable at a glance; Agents closes them, which is the whole reason it
	// reads better than the command (§1.3).
	ExpandedByDefault bool

	// Render draws one child row into the width left after the indent. Focus is passed so a row can
	// vary its own content with it; the cursor marker and the row style are the tree's business.
	Render func(item T, width int, focused bool) string

	// Selectable decides which rows the cursor may rest on, above the rule that a row with nothing
	// visible under it is always reachable. Repos and Agents pass Depth > 0, which calls a heading
	// whose children are on screen chrome; Loops selects everything, since its headings are items
	// (§7).
	Selectable func(row Row[T]) bool

	// Marked reports whether a row carries a mark. A tree supplying none draws none.
	Marked func(row Row[T]) bool
}

// Line is one printed line, already laid out, with what the caller needs to style it. Keeping the
// lines as data is what lets the tabs be tested without a terminal (§8).
type Line struct {
	Text    string
	Heading bool
	Focused bool
}

// IsExpanded reports whether the named fold is open.
func (t Tree[T]) IsExpanded(key string) bool {
	if open, ok := t.Expanded[key]; ok {
		return open
	}
	return t.ExpandedByDefault
}

// SetExpanded opens or closes a fold, allocating the map on first use.
func (t *Tree[T]) SetExpanded(key string, open bool) {
	if t.Expanded == nil {
		t.Expanded = map[string]bool{}
	}
	t.Expanded[key] = open
}

// allRows is the whole tree with the folds ignored: the rows the tab supplied, or its groups
// flattened into the same shape — a heading at depth 0 and its children at depth 1.
func (t Tree[T]) allRows() []Row[T] {
	if t.Rows != nil {
		return t.Rows
	}

	var out []Row[T]
	for _, group := range t.Groups {
		out = append(out, Row[T]{Depth: 0, Key: group.Key, Title: group.Title, Meta: group.Meta})
		for _, item := range group.Items {
			out = append(out, Row[T]{Node: item, Depth: 1, node: true})
		}
	}
	return out
}

// rows is what is on screen: every row no closed fold is hiding, carrying the fold state and the
// parent link derived from where it sits in the flat list.
func (t Tree[T]) rows() []Row[T] {
	all := t.allRows()

	var out []Row[T]
	// folded is the depth a closed fold was found at; everything deeper is off screen until a row
	// at that depth or above ends the run. -1 is nothing hidden.
	folded := -1
	// ancestors[d] is the index in out of the last row drawn at depth d, which is what the next row
	// one level deeper hangs off.
	var ancestors []int

	for i, row := range all {
		if folded >= 0 && row.Depth > folded {
			continue
		}
		folded = -1

		row.at = i
		row.HasKids = i+1 < len(all) && all[i+1].Depth > row.Depth
		row.Expanded = row.HasKids && t.IsExpanded(row.Key)
		if row.HasKids && !row.Expanded {
			folded = row.Depth
		}

		row.parent = -1
		if row.Depth > 0 && row.Depth <= len(ancestors) {
			row.parent = ancestors[row.Depth-1]
		}

		ancestors = append(ancestors[:min(row.Depth, len(ancestors))], len(out))
		out = append(out, row)
	}
	return out
}

// selectable reports whether the cursor can come to rest on a row. A row with nothing visible under
// it always can: a closed fold has to be reachable or it could never be opened again. Selectable
// decides the rest, and a tree that supplies none steps over any heading it has expanded.
func (t Tree[T]) selectable(row Row[T]) bool {
	if !row.HasKids || !row.Expanded {
		return true
	}
	if t.Selectable == nil {
		return false
	}
	return t.Selectable(row)
}

// snap is the row the cursor is really on: the index pulled into range, and then forwards to the
// nearest row a selection can rest on. It is -1 when there is nothing to select at all.
func (t Tree[T]) snap(rows []Row[T]) int {
	if len(rows) == 0 {
		return -1
	}

	at := min(max(t.Cursor, 0), len(rows)-1)
	for i := at; i < len(rows); i++ {
		if t.selectable(rows[i]) {
			return i
		}
	}
	for i := at - 1; i >= 0; i-- {
		if t.selectable(rows[i]) {
			return i
		}
	}
	return -1
}

// Len is the number of rows the cursor can reach, which is not the number of rows drawn: a heading
// the tab calls chrome is drawn but never selected.
func (t Tree[T]) Len() int {
	var n int
	for _, row := range t.rows() {
		if t.selectable(row) {
			n++
		}
	}
	return n
}

// selected is the row under the cursor.
func (t Tree[T]) selected() (Row[T], bool) {
	rows := t.rows()
	at := t.snap(rows)
	if at < 0 {
		return Row[T]{}, false
	}
	return rows[at], true
}

// clamp pulls the cursor back onto a row it can rest on, which a reload can put it off by handing
// the tree fewer rows than it had.
func (t *Tree[T]) clamp() {
	t.Cursor = max(t.snap(t.rows()), 0)
}

// CursorLine is the printed line the cursor is on, which is what a list scrolled to fit its rows
// keeps in sight.
func (t Tree[T]) CursorLine() int {
	return max(t.snap(t.rows()), 0)
}

// LineCount is the lines the tree draws: every row no closed fold is hiding.
func (t Tree[T]) LineCount() int {
	return len(t.rows())
}

// Down and Up move a row at a time, skipping what cannot be selected, and stop at the ends rather
// than wrapping.
func (t *Tree[T]) Down() {
	rows := t.rows()
	at := t.snap(rows)
	if at < 0 {
		t.Cursor = 0
		return
	}

	t.Cursor = at
	for i := at + 1; i < len(rows); i++ {
		if t.selectable(rows[i]) {
			t.Cursor = i
			return
		}
	}
}

func (t *Tree[T]) Up() {
	rows := t.rows()
	at := t.snap(rows)
	if at < 0 {
		t.Cursor = 0
		return
	}

	t.Cursor = at
	for i := at - 1; i >= 0; i-- {
		if t.selectable(rows[i]) {
			t.Cursor = i
			return
		}
	}
}

// First and Last are g and G.
func (t *Tree[T]) First() {
	t.Cursor = 0
	t.clamp()
}

func (t *Tree[T]) Last() {
	rows := t.rows()
	t.Cursor = len(rows) - 1
	t.clamp()
}

// Toggle opens or closes the fold the cursor is in — its own if the row has children, and otherwise
// the one it hangs under — and leaves the cursor within that fold: on the fold itself where that is
// selectable, and on its first child where it is not. Without the re-anchor a fold would carry the
// selection off into the next project.
func (t *Tree[T]) Toggle() {
	rows := t.rows()
	at := t.snap(rows)
	if at < 0 {
		return
	}

	fold := at
	if !rows[fold].HasKids && rows[fold].parent >= 0 {
		fold = rows[fold].parent
	}
	key := rows[fold].Key
	t.SetExpanded(key, !t.IsExpanded(key))

	// Only the fold's own descendants come and go, so the rows above it — and the fold itself — sit
	// at the same indexes they did before the toggle.
	depth := rows[fold].Depth
	rows = t.rows()
	for i := fold; i < len(rows); i++ {
		if i > fold && rows[i].Depth <= depth {
			break
		}
		if t.selectable(rows[i]) {
			t.Cursor = i
			return
		}
	}
	t.Cursor = fold
	t.clamp()
}

// Expand opens the fold under the cursor, leaving the cursor on it. A row with nothing folded under
// it, and one whose fold is already open, are left alone: `→` reads down into a branch rather than
// walking into it, so the cursor never moves under a key that only reveals rows.
func (t *Tree[T]) Expand() {
	rows := t.rows()
	at := t.snap(rows)
	if at < 0 {
		return
	}

	t.Cursor = at
	if !rows[at].HasKids || rows[at].Expanded {
		return
	}
	t.SetExpanded(rows[at].Key, true)
}

// Collapse closes the fold the cursor is in — its own if it is open, and otherwise the one it hangs
// under — and lands the cursor on the fold it closed. Repeated presses of `←` therefore walk out of
// a branch a level at a time, and one at the top level with nothing open is inert.
func (t *Tree[T]) Collapse() {
	rows := t.rows()
	at := t.snap(rows)
	if at < 0 {
		return
	}

	fold := at
	if !rows[fold].Expanded {
		fold = rows[fold].parent
	}
	if fold < 0 {
		t.Cursor = at
		return
	}

	t.SetExpanded(rows[fold].Key, false)
	// Only the fold's own descendants leave, so the rows above it — and the fold itself — sit at the
	// indexes they did before the collapse.
	t.Cursor = fold
	t.clamp()
}

// Focus puts the cursor on the first row the predicate accepts, reporting whether it found one. A
// write re-anchors with it: the row it acted on carries a fresh timestamp and sorts elsewhere, and a
// cursor left on an index would be pointing at whatever moved into the place.
func (t *Tree[T]) Focus(match func(item T) bool) bool {
	for i, row := range t.rows() {
		if !row.node || !t.selectable(row) {
			continue
		}
		if match(row.Node) {
			t.Cursor = i
			return true
		}
	}
	return false
}

// OnHeading reports whether the cursor is resting on a fold rather than on an item. The tabs use it
// to leave a key inert with a reason in the footer when it has no meaning for a project (§4).
func (t Tree[T]) OnHeading() bool {
	row, ok := t.selected()
	return ok && !row.node
}

// SelectedGroup is the top-level fold the cursor is under, whether it is on the fold itself or on a
// row within it. It carries the items folded beneath it whether or not they are on screen: a
// project collapsed to one line is still a project.
func (t Tree[T]) SelectedGroup() (Group[T], bool) {
	rows := t.rows()
	at := t.snap(rows)
	for at >= 0 && rows[at].Depth > 0 {
		at = rows[at].parent
	}
	if at < 0 {
		return Group[T]{}, false
	}

	head := rows[at]
	return Group[T]{Key: head.Key, Title: head.Title, Meta: head.Meta, Items: t.itemsUnder(head)}, true
}

// itemsUnder are the items folded beneath a row, read off the whole tree rather than the visible
// rows so a closed fold still reports what it holds.
func (t Tree[T]) itemsUnder(row Row[T]) []T {
	all := t.allRows()

	var items []T
	for i := row.at + 1; i < len(all) && all[i].Depth > row.Depth; i++ {
		if all[i].node {
			items = append(items, all[i].Node)
		}
	}
	return items
}

// SelectedParent is the item the row under the cursor hangs beneath: the nearest row above it that
// holds one, so a fold drawn from something that is not an item — Repos' project headings, Loops'
// `(unassigned)` bucket — is looked through rather than reported. A row at the top level has none.
func (t Tree[T]) SelectedParent() (T, bool) {
	var zero T
	rows := t.rows()
	at := t.snap(rows)
	if at < 0 {
		return zero, false
	}

	for at = rows[at].parent; at >= 0; at = rows[at].parent {
		if rows[at].node {
			return rows[at].Node, true
		}
	}
	return zero, false
}

// SelectedRow is the whole row under the cursor, for a caller that needs more of it than the item
// behind it: the move picker tells its synthetic rows apart by their keys, and one of them is a
// destination while the other is not (§7.1).
func (t Tree[T]) SelectedRow() (Row[T], bool) {
	return t.selected()
}

// SelectedItem is the item under the cursor. A fold has none, so the actions that need one are
// inert there.
func (t Tree[T]) SelectedItem() (T, bool) {
	var zero T
	row, ok := t.selected()
	if !ok || !row.node {
		return zero, false
	}
	return row.Node, true
}

// Lines lays the visible tree out at the given width.
func (t Tree[T]) Lines(width int) []Line {
	rows := t.rows()
	cursor := t.snap(rows)

	var lines []Line
	for i, row := range rows {
		focused := i == cursor
		// A row holding anything below it is drawn as a fold whether or not it is an item: the
		// marker and the count are what say a branch is there, and Loops' folds are items (§4). A
		// plain row is a heading line without the marker, so it is drawn as one though it heads
		// nothing.
		if row.node && !row.HasKids && !row.plain {
			lines = append(lines, Line{Text: t.itemLine(row, width, focused), Focused: focused})
			continue
		}
		lines = append(lines, Line{Text: t.heading(row, width), Heading: true, Focused: focused})
	}
	return lines
}

// View renders the tree through the palette, which is the only place a row's colour is decided.
func (t Tree[T]) View(width int, theme Theme) string {
	var b strings.Builder
	for i, line := range t.Lines(width) {
		if i > 0 {
			b.WriteString("\n")
		}
		switch {
		case line.Focused:
			b.WriteString(theme.RowFocused.Render(line.Text))
		case line.Heading:
			b.WriteString(theme.Heading.Render(line.Text))
		default:
			b.WriteString(theme.Row.Render(line.Text))
		}
	}
	return b.String()
}

// indent is the blank margin a row sits behind, before the column its own marker takes. It is
// capped past a few levels, so a deep branch cannot squeeze the row's own columns to nothing on a
// narrow terminal.
func indent(depth int) string {
	return strings.Repeat(" ", headingIndent+indentStep*min(depth, maxIndentDepth))
}

// gutter is a row's prefix with the mark drawn in its first column when the row carries one.
func (t Tree[T]) gutter(row Row[T], prefix string) string {
	if t.Marked == nil || !t.Marked(row) {
		return prefix
	}
	return markGlyph + strings.TrimPrefix(prefix, " ")
}

// heading is a fold line: the marker, the project, and the count pushed to the right edge.
func (t Tree[T]) heading(row Row[T], width int) string {
	marker := foldClosed
	switch {
	case row.plain:
		marker = " "
	case row.Expanded:
		marker = foldOpen
	}

	prefix := t.gutter(row, indent(row.Depth)+marker+" ")
	if width <= 0 {
		return prefix + row.Title
	}

	room := width - lipgloss.Width(prefix)
	if row.Meta == "" {
		return prefix + truncate(row.Title, room)
	}

	meta := truncate(row.Meta, room)
	title := truncate(row.Title, room-lipgloss.Width(meta)-1)
	gap := room - lipgloss.Width(title) - lipgloss.Width(meta)
	if gap < 1 {
		gap = 1
	}
	return prefix + title + strings.Repeat(" ", gap) + meta
}

// itemLine is a row with an item behind it: the cursor marker, then whatever the tab drew into the
// width that is left.
func (t Tree[T]) itemLine(row Row[T], width int, focused bool) string {
	marker := " "
	if focused {
		marker = cursorMark
	}
	prefix := t.gutter(row, indent(row.Depth)+marker+" ")

	room := width - lipgloss.Width(prefix)
	if width <= 0 {
		room = 0
	}

	text := fmt.Sprint(row.Node)
	if t.Render != nil {
		text = t.Render(row.Node, room, focused)
	}

	if width <= 0 {
		return prefix + text
	}
	return prefix + truncate(text, room)
}

// truncate trims to a printed width, marking the cut so a clipped title does not read as a short
// one. Widths are measured rather than counted because a row can carry wide runes.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}

	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
