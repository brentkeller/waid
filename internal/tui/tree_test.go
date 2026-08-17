package tui

import (
	"strings"
	"testing"
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

// The cursor stops at the ends rather than running off them: k on the first row and j on the last
// both leave it where it was, however many times they are pressed.
func TestCursorClampsAtBothEnds(t *testing.T) {
	tree := fixture()

	tree.First()
	for range 5 {
		tree.Up()
	}
	if tree.Cursor != 0 {
		t.Errorf("Cursor = %d after k at the top, want 0", tree.Cursor)
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

	open := tree.Rows(80)[0]
	if !open.Heading || !strings.Contains(open.Text, "2 signals") {
		t.Errorf("expanded fold = %q, want the count on it", open.Text)
	}
	if !strings.Contains(open.Text, foldOpen) {
		t.Errorf("expanded fold = %q, want the open marker %q", open.Text, foldOpen)
	}

	tree.Expanded = map[string]bool{"waid": false}
	closed := tree.Rows(80)[0]
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

	rows := tree.Rows(80)
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
	for _, row := range tree.Rows(80) {
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
	tree.Rows(80)

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
	if tree.Cursor >= tree.Len() {
		t.Fatalf("Cursor = %d with %d rows", tree.Cursor, tree.Len())
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
	if rows := tree.Rows(80); len(rows) != 0 {
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
