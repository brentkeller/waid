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

// Tree is the collapsible project → children list both tabs draw. The cursor addresses the visible
// rows a selection can land on: the children of expanded groups, and the fold of any group that is
// closed or empty. An expanded heading is chrome and the cursor steps over it (§4).
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
}

// Row is one printed line: a fold or a child, already laid out, with what the caller needs to style
// it. Keeping the rows as data is what lets the tabs be tested without a terminal (§8).
type Row struct {
	Text    string
	Heading bool
	Focused bool
}

// position is a place the cursor can rest: a child of an expanded group, or the fold of a group
// with no visible children, marked by an item index of -1.
type position struct {
	group int
	item  int
}

const onFold = -1

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

func (t Tree[T]) positions() []position {
	var out []position
	for gi, group := range t.Groups {
		if !t.IsExpanded(group.Key) || len(group.Items) == 0 {
			out = append(out, position{group: gi, item: onFold})
			continue
		}
		for ii := range group.Items {
			out = append(out, position{group: gi, item: ii})
		}
	}
	return out
}

// Len is the number of rows the cursor can reach, which is not the number of rows drawn: an
// expanded heading is drawn but never selected.
func (t Tree[T]) Len() int { return len(t.positions()) }

func (t Tree[T]) at(cursor int) (position, bool) {
	positions := t.positions()
	if cursor < 0 || cursor >= len(positions) {
		return position{}, false
	}
	return positions[cursor], true
}

// clamp pulls the cursor back into range, which a reload can put it out of by handing the tree
// fewer groups than it had.
func (t *Tree[T]) clamp() {
	if last := t.Len() - 1; t.Cursor > last {
		t.Cursor = last
	}
	if t.Cursor < 0 {
		t.Cursor = 0
	}
}

// Down and Up move a row at a time and stop at the ends rather than wrapping.
func (t *Tree[T]) Down() {
	t.clamp()
	if t.Cursor < t.Len()-1 {
		t.Cursor++
	}
}

func (t *Tree[T]) Up() {
	t.clamp()
	if t.Cursor > 0 {
		t.Cursor--
	}
}

// First and Last are g and G.
func (t *Tree[T]) First() { t.Cursor = 0 }

func (t *Tree[T]) Last() {
	t.Cursor = t.Len() - 1
	t.clamp()
}

// Toggle opens or closes the fold the cursor is in and leaves the cursor on that group: closing
// puts it on the fold, opening puts it on the first child, since an open heading cannot be
// selected. Without the re-anchor a fold would carry the selection off into the next project.
func (t *Tree[T]) Toggle() {
	t.clamp()
	pos, ok := t.at(t.Cursor)
	if !ok {
		return
	}

	key := t.Groups[pos.group].Key
	t.SetExpanded(key, !t.IsExpanded(key))

	for i, p := range t.positions() {
		if p.group == pos.group {
			t.Cursor = i
			break
		}
	}
	t.clamp()
}

// Focus puts the cursor on the first child the predicate accepts, reporting whether it found one. A
// write re-anchors with it: the row it acted on carries a fresh timestamp and sorts elsewhere, and a
// cursor left on an index would be pointing at whatever moved into the place.
func (t *Tree[T]) Focus(match func(item T) bool) bool {
	for i, pos := range t.positions() {
		if pos.item == onFold {
			continue
		}
		if match(t.Groups[pos.group].Items[pos.item]) {
			t.Cursor = i
			return true
		}
	}
	return false
}

// OnHeading reports whether the cursor is resting on a fold rather than a child. The tabs use it to
// leave a key inert with a reason in the footer when it has no meaning for a project (§4).
func (t Tree[T]) OnHeading() bool {
	pos, ok := t.at(t.Cursor)
	return ok && pos.item == onFold
}

// SelectedGroup is the group the cursor is in, whether it is on the fold or on a child.
func (t Tree[T]) SelectedGroup() (Group[T], bool) {
	pos, ok := t.at(t.Cursor)
	if !ok {
		return Group[T]{}, false
	}
	return t.Groups[pos.group], true
}

// SelectedItem is the child under the cursor. A fold has no item, so the actions that need one are
// inert there.
func (t Tree[T]) SelectedItem() (T, bool) {
	var zero T
	pos, ok := t.at(t.Cursor)
	if !ok || pos.item == onFold {
		return zero, false
	}
	return t.Groups[pos.group].Items[pos.item], true
}

// Rows lays the visible tree out at the given width: every group's fold, and the children of the
// ones that are open.
func (t Tree[T]) Rows(width int) []Row {
	cursor, hasCursor := t.at(t.Cursor)

	var rows []Row
	for gi, group := range t.Groups {
		open := t.IsExpanded(group.Key) && len(group.Items) > 0
		focused := hasCursor && cursor.group == gi && cursor.item == onFold

		rows = append(rows, Row{Text: t.heading(group, open, width), Heading: true, Focused: focused})
		if !open {
			continue
		}
		for ii, item := range group.Items {
			focused := hasCursor && cursor.group == gi && cursor.item == ii
			rows = append(rows, Row{Text: t.row(item, width, focused), Focused: focused})
		}
	}

	return rows
}

// View renders the tree through the palette, which is the only place a row's colour is decided.
func (t Tree[T]) View(width int, theme Theme) string {
	var b strings.Builder
	for i, row := range t.Rows(width) {
		if i > 0 {
			b.WriteString("\n")
		}
		switch {
		case row.Focused:
			b.WriteString(theme.RowFocused.Render(row.Text))
		case row.Heading:
			b.WriteString(theme.Heading.Render(row.Text))
		default:
			b.WriteString(theme.Row.Render(row.Text))
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
