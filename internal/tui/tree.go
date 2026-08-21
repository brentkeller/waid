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

	// headingIndent leaves a column before the fold marker; rowIndent puts a child's text two
	// columns past its heading's title, with the cursor marker sitting in the gap.
	headingIndent = 1
	rowIndent     = 5
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

// Row is one visible line of the tree: the node it draws, how deep the line sits, whether anything
// is folded under it and whether that is on screen. The cursor is an index into these rows, so a
// tree of any depth is navigated the same way as a two-level one (§7).
type Row[T any] struct {
	Node     T
	Depth    int
	HasKids  bool
	Expanded bool

	// Key is what Expanded is keyed by, supplied by whatever built the row. A row with nothing
	// folded under it has no fold and carries none.
	Key string

	// parent is the index of the row this one is folded under, or -1 at the top level. group is the
	// Group the row was built from, which a heading draws its title and count out of. node reports
	// whether Node holds anything: a heading built from a Group has no item behind it.
	parent int
	group  int
	node   bool
}

// Tree is the collapsible list the tabs draw. It takes projects and their children and flattens
// them into rows the cursor walks.
type Tree[T any] struct {
	Groups   []Group[T]
	Cursor   int
	Expanded map[string]bool

	// ExpandedByDefault decides a group the Expanded map says nothing about. Repos opens its groups
	// so the signals are readable at a glance; Agents closes them, which is the whole reason it
	// reads better than the command (§1.3).
	ExpandedByDefault bool

	// Render draws one child row into the width left after the indent. Focus is passed so a row can
	// vary its own content with it; the cursor marker and the row style are the tree's business.
	Render func(item T, width int, focused bool) string

	// Selectable decides which rows the cursor may rest on, above the rule that a row with nothing
	// visible under it is always reachable. Repos and Agents call a heading whose children are on
	// screen chrome and step over it; Loops selects everything, since its headings are items (§7).
	Selectable func(row Row[T]) bool
}

// Line is one printed line, already laid out, with what the caller needs to style it. Keeping the
// lines as data is what lets the tabs be tested without a terminal (§8).
type Line struct {
	Text    string
	Heading bool
	Focused bool
}

// IsExpanded reports whether the named group's fold is open.
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

// rows flattens the groups into the lines that are on screen: every heading, and the children of
// the ones that are open.
func (t Tree[T]) rows() []Row[T] {
	var out []Row[T]
	for gi, group := range t.Groups {
		open := t.IsExpanded(group.Key) && len(group.Items) > 0

		head := len(out)
		out = append(out, Row[T]{
			Depth:    0,
			HasKids:  len(group.Items) > 0,
			Expanded: open,
			Key:      group.Key,
			parent:   -1,
			group:    gi,
		})
		if !open {
			continue
		}
		for _, item := range group.Items {
			out = append(out, Row[T]{Node: item, Depth: 1, parent: head, group: gi, node: true})
		}
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
// the tree fewer groups than it had.
func (t *Tree[T]) clamp() {
	t.Cursor = max(t.snap(t.rows()), 0)
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

// SelectedGroup is the group the cursor is in, whether it is on the fold or on a child.
func (t Tree[T]) SelectedGroup() (Group[T], bool) {
	row, ok := t.selected()
	if !ok {
		return Group[T]{}, false
	}
	return t.Groups[row.group], true
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
		if row.node {
			lines = append(lines, Line{Text: t.row(row.Node, width, focused), Focused: focused})
			continue
		}
		lines = append(lines, Line{
			Text:    t.heading(t.Groups[row.group], row.Expanded, width),
			Heading: true,
			Focused: focused,
		})
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

// heading is the fold line: the marker, the project, and the count pushed to the right edge.
func (t Tree[T]) heading(group Group[T], open bool, width int) string {
	marker := foldClosed
	if open {
		marker = foldOpen
	}

	prefix := strings.Repeat(" ", headingIndent) + marker + " "
	if width <= 0 {
		return prefix + group.Title
	}

	room := width - lipgloss.Width(prefix)
	if group.Meta == "" {
		return prefix + truncate(group.Title, room)
	}

	meta := truncate(group.Meta, room)
	title := truncate(group.Title, room-lipgloss.Width(meta)-1)
	gap := room - lipgloss.Width(title) - lipgloss.Width(meta)
	if gap < 1 {
		gap = 1
	}
	return prefix + title + strings.Repeat(" ", gap) + meta
}

// row is a child line: the cursor marker, then whatever the tab drew into the width that is left.
func (t Tree[T]) row(item T, width int, focused bool) string {
	room := width - rowIndent
	if width <= 0 {
		room = 0
	}

	text := fmt.Sprint(item)
	if t.Render != nil {
		text = t.Render(item, room, focused)
	}

	marker := " "
	if focused {
		marker = cursorMark
	}
	prefix := strings.Repeat(" ", rowIndent-2) + marker + " "

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
