package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// A tree of three projects, expanded the way Scan shows them, with items whose rendered text is
// just the item itself so a row is easy to assert on.
func fixture() Tree[string] {
	return Tree[string]{
		Groups: []Group[string]{
			{Key: "waid", Title: `C:\dev\waid`, Meta: "2 signals", Items: []string{"a1", "a2"}},
			{Key: "dr", Title: "DevResults/DevResults", Meta: "3 signals", Items: []string{"b1", "b2", "b3"}},
			{Key: "bkc", Title: `C:\dev\bkc-my`, Meta: "1 signal", Items: []string{"c1"}},
		},
		ExpandedByDefault: true,
		Render:            func(item string, width int, focused bool) string { return item },
	}
}

// walk collects the item under the cursor at every position, from the top down.
func walk(t *testing.T, tree Tree[string]) []string {
	t.Helper()

	tree.First()
	var seen []string
	for {
		if item, ok := tree.SelectedItem(); ok {
			seen = append(seen, item)
		}
		before := tree.Cursor
		tree.Down()
		if tree.Cursor == before {
			return seen
		}
	}
}

// The cursor stops at the ends rather than running off them: ↑ on the first row and ↓ on the last
// both leave it where it was, however many times they are pressed.
func TestCursorClampsAtBothEnds(t *testing.T) {
	tree := fixture()

	tree.First()
	top := tree.Cursor
	for range 5 {
		tree.Up()
	}
	if tree.Cursor != top {
		t.Errorf("Cursor = %d after k at the top, want %d", tree.Cursor, top)
	}
	if item, _ := tree.SelectedItem(); item != "a1" {
		t.Errorf("first row = %q, want %q", item, "a1")
	}

	tree.Last()
	last := tree.Cursor
	for range 5 {
		tree.Down()
	}
	if tree.Cursor != last {
		t.Errorf("Cursor = %d after j at the bottom, want %d", tree.Cursor, last)
	}
	if item, _ := tree.SelectedItem(); item != "c1" {
		t.Errorf("last row = %q, want %q", item, "c1")
	}
}

// j and k move between child rows: an expanded project heading is chrome, not a stop, so walking
// the whole tree lands on every item and on no heading (§4).
func TestCursorSkipsExpandedHeadings(t *testing.T) {
	tree := fixture()

	want := []string{"a1", "a2", "b1", "b2", "b3", "c1"}
	if got := walk(t, tree); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("walking down visited %v, want %v", got, want)
	}

	tree.First()
	for range len(want) {
		if tree.OnHeading() {
			t.Fatalf("cursor stopped on a heading at position %d", tree.Cursor)
		}
		tree.Down()
	}
}

// g and G jump to the first and last rows without stepping through what is between them.
func TestFirstAndLastJumpToTheEnds(t *testing.T) {
	tree := fixture()

	tree.Last()
	if item, _ := tree.SelectedItem(); item != "c1" {
		t.Errorf("G selected %q, want %q", item, "c1")
	}

	tree.First()
	if item, _ := tree.SelectedItem(); item != "a1" {
		t.Errorf("g selected %q, want %q", item, "a1")
	}
}

// enter folds the group the cursor is in, and the cursor comes to rest on the fold it just closed —
// otherwise collapsing a group would silently move the selection into the next one.
func TestEnterCollapsesTheGroupUnderTheCursor(t *testing.T) {
	tree := fixture()

	tree.First()
	tree.Down() // a2, the second child of the first group
	tree.Toggle()

	if !tree.OnHeading() {
		t.Fatal("after collapsing, the cursor is not on the fold")
	}
	group, ok := tree.SelectedGroup()
	if !ok || group.Key != "waid" {
		t.Fatalf("after collapsing, the cursor is on group %+v, want waid", group)
	}
	if _, ok := tree.SelectedItem(); ok {
		t.Error("a fold reports an item under the cursor")
	}
}

// enter on a closed fold opens it and puts the cursor on the first child, since the heading of an
// expanded group is no longer a place the cursor can be.
func TestEnterExpandsTheFoldUnderTheCursor(t *testing.T) {
	tree := fixture()
	tree.Expanded = map[string]bool{"waid": false}

	tree.First()
	if !tree.OnHeading() {
		t.Fatal("a collapsed group's fold is not selectable")
	}

	tree.Toggle()
	if item, _ := tree.SelectedItem(); item != "a1" {
		t.Errorf("after expanding, the cursor is on %q, want %q", item, "a1")
	}
}

// A closed fold hides its children from the cursor as well as from the view: j must not walk into
// rows that are not on screen.
func TestCollapsedGroupChildrenAreUnreachable(t *testing.T) {
	tree := fixture()
	tree.Expanded = map[string]bool{"dr": false}

	want := []string{"a1", "a2", "c1"}
	if got := walk(t, tree); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("walking down visited %v, want %v", got, want)
	}
	if got, want := tree.Len(), 4; got != want {
		t.Errorf("Len() = %d, want %d — two items, one fold, one item", got, want)
	}
}

// Review collapses every project by default, which is the whole reason it reads better than the
// command: a busy day is one line per project until asked for more (§1.3).
func TestGroupsCollapseByDefaultWhenTheTreeSaysSo(t *testing.T) {
	tree := fixture()
	tree.ExpandedByDefault = false

	if got, want := tree.Len(), len(tree.Groups); got != want {
		t.Errorf("Len() = %d, want %d — one fold per project", got, want)
	}
	if walked := walk(t, tree); len(walked) != 0 {
		t.Errorf("walking a fully collapsed tree reached items %v", walked)
	}
}

// The count is what a fold is for, so it stays on the heading closed as well as open.
func TestCountsStayOnTheFoldEitherWay(t *testing.T) {
	tree := fixture()

	open := tree.Lines(80)[0]
	if !open.Heading || !strings.Contains(open.Text, "2 signals") {
		t.Errorf("expanded fold = %q, want the count on it", open.Text)
	}
	if !strings.Contains(open.Text, foldOpen) {
		t.Errorf("expanded fold = %q, want the open marker %q", open.Text, foldOpen)
	}

	tree.Expanded = map[string]bool{"waid": false}
	closed := tree.Lines(80)[0]
	if !closed.Heading || !strings.Contains(closed.Text, "2 signals") {
		t.Errorf("collapsed fold = %q, want the count on it", closed.Text)
	}
	if !strings.Contains(closed.Text, foldClosed) {
		t.Errorf("collapsed fold = %q, want the closed marker %q", closed.Text, foldClosed)
	}
}

// Rows are what the tabs draw: one per heading and one per visible child, with the collapsed
// group's children absent entirely.
func TestRowsCoverTheVisibleTreeOnly(t *testing.T) {
	tree := fixture()
	tree.Expanded = map[string]bool{"dr": false}

	rows := tree.Lines(80)
	if got, want := len(rows), 3+1+2; got != want {
		t.Fatalf("Rows() returned %d rows, want %d", got, want)
	}
	for _, row := range rows {
		if strings.Contains(row.Text, "b1") || strings.Contains(row.Text, "b3") {
			t.Errorf("a collapsed group's child is still drawn: %q", row.Text)
		}
	}
}

// Exactly one row carries the cursor, and it is the one the selection reports.
func TestRowsMarkOnlyTheRowUnderTheCursor(t *testing.T) {
	tree := fixture()
	tree.First()
	tree.Down()
	tree.Down() // b1, the first child of the second group

	var focused []string
	for _, row := range tree.Lines(80) {
		if row.Focused {
			focused = append(focused, row.Text)
		}
	}
	if len(focused) != 1 {
		t.Fatalf("%d rows are focused, want 1: %v", len(focused), focused)
	}
	if !strings.Contains(focused[0], "b1") {
		t.Errorf("focused row = %q, want the row for b1", focused[0])
	}
}

// The width a row is rendered with is the width it actually has, indent and all, so a tab laying
// out right-aligned columns cannot overflow the terminal by the indent.
func TestRenderReceivesTheWidthLeftAfterTheIndent(t *testing.T) {
	tree := fixture()

	var got int
	tree.Render = func(item string, width int, focused bool) string {
		got = width
		return item
	}
	tree.Lines(80)

	if want := 80 - rowIndent; got != want {
		t.Errorf("Render got width %d, want %d", got, want)
	}
}

// A reload can hand the tree fewer groups than the cursor was resting past; the next keypress has
// to land somewhere real rather than reading off the end of the list.
func TestCursorClampsWhenTheGroupsShrink(t *testing.T) {
	tree := fixture()
	tree.Last()
	tree.Groups = tree.Groups[:1]

	tree.Down()
	if rows := tree.rows(); tree.Cursor >= len(rows) {
		t.Fatalf("Cursor = %d with %d rows on screen", tree.Cursor, len(rows))
	}
	if item, ok := tree.SelectedItem(); !ok || item != "a2" {
		t.Errorf("selection after the shrink = %q (%v), want a2", item, ok)
	}
}

// An empty tree has nothing to select and nothing to draw, and every key still has to be safe on it.
func TestEmptyTreeIsInert(t *testing.T) {
	var tree Tree[string]

	tree.Down()
	tree.Up()
	tree.First()
	tree.Last()
	tree.Toggle()

	if _, ok := tree.SelectedItem(); ok {
		t.Error("an empty tree reports an item under the cursor")
	}
	if _, ok := tree.SelectedGroup(); ok {
		t.Error("an empty tree reports a group under the cursor")
	}
	if rows := tree.Lines(80); len(rows) != 0 {
		t.Errorf("an empty tree drew %d rows", len(rows))
	}
}

// An expanded group with nothing in it keeps its fold selectable — otherwise a project whose items
// were all triaged away would drop out of reach while still on screen.
func TestEmptyGroupKeepsASelectableFold(t *testing.T) {
	tree := fixture()
	tree.Groups[1].Items = nil

	tree.First()
	tree.Down()
	tree.Down()

	if !tree.OnHeading() {
		t.Fatal("the empty group's fold is not selectable")
	}
	group, ok := tree.SelectedGroup()
	if !ok || group.Key != "dr" {
		t.Errorf("cursor is on %+v, want the empty group", group)
	}
}

// Focus puts the cursor on the row holding a named item, wherever the list has since sorted it, and
// leaves the cursor alone when the item is no longer in the list at all.
func TestFocusFindsAnItemWhereverItSits(t *testing.T) {
	tree := fixture()
	tree.First()

	if !tree.Focus(func(item string) bool { return item == "b2" }) {
		t.Fatal("Focus did not find an item the tree holds")
	}
	if item, _ := tree.SelectedItem(); item != "b2" {
		t.Errorf("Focus left the cursor on %q, want b2", item)
	}

	at := tree.Cursor
	if tree.Focus(func(item string) bool { return item == "gone" }) {
		t.Error("Focus reported finding an item the tree does not hold")
	}
	if tree.Cursor != at {
		t.Errorf("a failed Focus moved the cursor to %d, want it left at %d", tree.Cursor, at)
	}
}

// A collapsed group hides its children from the cursor, so Focus cannot land on one.
func TestFocusSkipsTheChildrenOfAClosedGroup(t *testing.T) {
	tree := fixture()
	tree.SetExpanded("dr", false)

	if tree.Focus(func(item string) bool { return item == "b2" }) {
		t.Error("Focus landed on a row the cursor cannot reach")
	}
}

// The rows are the tree flattened: one entry per visible line, each carrying how deep it sits,
// whether it has children and whether they are on screen. Everything the cursor does is an index
// into this list (§7).
func TestRowsCarryDepthAndFoldState(t *testing.T) {
	tree := fixture()
	tree.Expanded = map[string]bool{"dr": false}

	rows := tree.rows()
	if got, want := len(rows), 3+2+1; got != want {
		t.Fatalf("rows() returned %d rows, want %d — three headings, two children, one child", got, want)
	}

	head := rows[0]
	if head.Depth != 0 || !head.HasKids || !head.Expanded || head.Key != "waid" {
		t.Errorf("first row = %+v, want an expanded depth-0 heading keyed waid", head)
	}
	if rows[1].Node != "a1" || rows[1].Depth != 1 || rows[1].HasKids {
		t.Errorf("second row = %+v, want the depth-1 child a1", rows[1])
	}

	closed := rows[3]
	if closed.Depth != 0 || !closed.HasKids || closed.Expanded {
		t.Errorf("collapsed heading = %+v, want a depth-0 heading with children off screen", closed)
	}
}

// The cursor addresses visible rows, so it counts the headings it steps over. What it lands on is
// what matters; the index is only how the tree gets there.
func TestCursorIndexesVisibleRows(t *testing.T) {
	tree := fixture()

	tree.First()
	if tree.Cursor != 1 {
		t.Errorf("g left the cursor at %d, want 1 — row 0 is the heading it steps over", tree.Cursor)
	}
	if item, _ := tree.SelectedItem(); item != "a1" {
		t.Errorf("g selected %q, want a1", item)
	}
}

// Selectable is what decides whether a heading is chrome. Loops passes always-true, because its
// headings are items that get noted, tagged and closed (§7).
func TestSelectableOpensHeadingsToTheCursor(t *testing.T) {
	tree := fixture()
	tree.Selectable = func(Row[string]) bool { return true }

	if got, want := tree.Len(), 3+6; got != want {
		t.Errorf("Len() = %d, want %d — every heading and every child", got, want)
	}

	tree.First()
	if !tree.OnHeading() {
		t.Fatal("g did not land on the first heading")
	}

	group, ok := tree.SelectedGroup()
	if !ok || group.Key != "waid" {
		t.Errorf("cursor is on %+v, want the first group", group)
	}
	if _, ok := tree.SelectedItem(); ok {
		t.Error("a heading reports an item under the cursor")
	}
}

// Repos and Agents pass Depth > 0, which is the step-over rule they have today: the walk reaches
// every child and no expanded heading, and a collapsed fold stays reachable so it can be opened.
func TestSelectableDepthMatchesTodaysRule(t *testing.T) {
	tree := fixture()
	tree.Selectable = func(row Row[string]) bool { return row.Depth > 0 }

	want := []string{"a1", "a2", "b1", "b2", "b3", "c1"}
	if got := walk(t, tree); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("walking down visited %v, want %v", got, want)
	}

	tree.ExpandedByDefault = false
	tree.First()
	if !tree.OnHeading() || tree.Len() != len(tree.Groups) {
		t.Fatalf("a collapsed tree offers %d rows, want one reachable fold per project", tree.Len())
	}

	tree.Toggle()
	if item, _ := tree.SelectedItem(); item != "a1" {
		t.Errorf("after expanding, the cursor is on %q, want a1", item)
	}
}

// A tab whose headings are selectable keeps the cursor on the heading through a fold, rather than
// diving to the first child the way a tab that steps over headings has to.
func TestToggleStaysOnASelectableHeading(t *testing.T) {
	tree := fixture()
	tree.Selectable = func(Row[string]) bool { return true }

	tree.First()
	tree.Toggle()
	if !tree.OnHeading() {
		t.Fatal("collapsing moved the cursor off the heading")
	}

	tree.Toggle()
	if !tree.OnHeading() {
		t.Fatal("expanding moved the cursor off the heading")
	}
	if group, _ := tree.SelectedGroup(); group.Key != "waid" {
		t.Errorf("the cursor left the group it toggled, landing on %+v", group)
	}
}

// deep is a three-level tree built from rows rather than groups, which is the shape Loops draws and
// the one the fold keys are worth asserting against.
func deep() Tree[string] {
	return Tree[string]{
		Rows: []Row[string]{
			{Node: "root", Depth: 0, Key: "root", node: true},
			{Node: "mid", Depth: 1, Key: "mid", node: true},
			{Node: "leaf", Depth: 2, Key: "leaf", node: true},
			{Node: "other", Depth: 0, Key: "other", node: true},
		},
		ExpandedByDefault: true,
		Selectable:        func(Row[string]) bool { return true },
		Render:            func(item string, width int, focused bool) string { return item },
	}
}

// Collapse closes the fold the cursor is in and lands on it, so repeated presses walk out of a
// branch a level at a time and one at the top level with nothing open is inert (§7).
func TestCollapseWalksOutOfABranch(t *testing.T) {
	tree := deep()

	tree.Cursor = 2
	if item, _ := tree.SelectedItem(); item != "leaf" {
		t.Fatalf("the cursor is on %q, want the bottom of the branch", item)
	}

	for _, want := range []string{"mid", "root"} {
		tree.Collapse()
		if item, _ := tree.SelectedItem(); item != want {
			t.Fatalf("collapsing landed on %q, want %q", item, want)
		}
		if tree.IsExpanded(want) {
			t.Errorf("the fold at %q is still open", want)
		}
	}

	tree.Collapse()
	if item, _ := tree.SelectedItem(); item != "root" {
		t.Errorf("collapsing at the top level moved the cursor to %q", item)
	}
}

// Expand opens the fold under the cursor and no more: one press reveals one level, and the cursor
// stays on the row that was pressed (§7).
func TestExpandOpensOneLevel(t *testing.T) {
	tree := deep()
	tree.ExpandedByDefault = false

	tree.First()
	tree.Expand()
	if item, _ := tree.SelectedItem(); item != "root" {
		t.Errorf("expanding moved the cursor to %q, want the fold it opened", item)
	}
	if !tree.IsExpanded("root") {
		t.Fatal("expanding did not open the fold under the cursor")
	}
	if tree.IsExpanded("mid") {
		t.Error("expanding opened the branch below the fold as well")
	}

	// A fold already open is left alone rather than walked into.
	tree.Expand()
	if item, _ := tree.SelectedItem(); item != "root" {
		t.Errorf("expanding an open fold moved the cursor to %q", item)
	}

	// A leaf has nothing to open, so the press leaves both the cursor and the folds where they are.
	// The branch below root is still closed, so the row after it is the tree's other root.
	tree.Cursor = 2
	tree.Expand()
	if item, _ := tree.SelectedItem(); item != "other" {
		t.Errorf("expanding a leaf moved the cursor to %q", item)
	}
	if tree.IsExpanded("mid") {
		t.Error("expanding a leaf opened a fold elsewhere")
	}
}

// marking is a tree two levels deep whose fold and first leaf carry a mark.
func marking(marked func(Row[string]) bool) Tree[string] {
	return Tree[string]{
		Rows: []Row[string]{
			{Node: "root", Depth: 0, Key: "root", Title: "root", node: true},
			{Node: "leaf", Depth: 1, Key: "leaf", node: true},
			{Node: "other", Depth: 1, Key: "other", node: true},
		},
		ExpandedByDefault: true,
		Selectable:        func(Row[string]) bool { return true },
		Marked:            marked,
	}
}

// A marked row draws the glyph in the gutter at any depth, on a fold and on an item alike, and the
// row is no wider for it.
func TestMarkedRowsDrawTheGlyphInTheGutter(t *testing.T) {
	marked := marking(func(row Row[string]) bool { return row.Key != "other" }).Lines(40)
	bare := marking(nil).Lines(40)

	for i, want := range []bool{true, true, false} {
		if got := strings.HasPrefix(marked[i].Text, markGlyph); got != want {
			t.Errorf("line %d is %q, marked = %v, want %v", i, marked[i].Text, got, want)
		}
		if got, want := lipgloss.Width(marked[i].Text), lipgloss.Width(bare[i].Text); got != want {
			t.Errorf("line %d is %d columns marked and %d bare", i, got, want)
		}
	}
}

// A tree that marks nothing renders what a tree with no notion of marks renders.
func TestATreeMarkingNothingRendersUnchanged(t *testing.T) {
	none := marking(func(Row[string]) bool { return false }).Lines(40)
	bare := marking(nil).Lines(40)

	for i := range bare {
		if none[i].Text != bare[i].Text {
			t.Errorf("line %d is %q, want %q", i, none[i].Text, bare[i].Text)
		}
	}
}
