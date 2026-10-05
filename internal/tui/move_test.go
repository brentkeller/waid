package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"github.com/brentkeller/waid/internal/tree"
)

// The titles the picker draws, named here because the assertions read as prose: the item the tests
// move sits under the first, and the rest are the destinations on offer beside it.
const (
	branchTitle  = "Localized notifications"
	workersTitle = "Background workers speak the requester's language"
	blockedTitle = "Deploy blocked until the migration is approved"

	spikeTitle = "TUI design spike"
)

// filing is a Loops tab whose log already holds a branch two deep, since a move is read against
// where the item sits and a flat list cannot show that. 4h2k hangs under nktt, which hangs under
// vq2n.
func filing(t *testing.T) (Model, string) {
	t.Helper()

	m, path := working(t)

	loaded, _ := m.Update(loopsNested())
	return loaded.(Model), path
}

// parentOf is the parent the loaded copy of an item holds, which is what says a move landed in the
// list and not only in the log.
func parentOf(t *testing.T, m Model, id string) *string {
	t.Helper()

	item, found := m.loadedItem(id)
	if !found {
		t.Fatalf("no loaded item for %q", id)
	}
	return item.Parent
}

// destinations are the rows the picker offers, by the key each is identified with. They are read off
// the whole tree rather than the visible rows, so a collapsed branch still says what it holds.
func destinations(m Model) []string {
	var keys []string
	for _, row := range m.moveTree(m.viewWidth()).allRows() {
		keys = append(keys, row.Key)
	}
	return keys
}

// focused is the row the cursor is on, by the key it is identified with.
func focused(t *testing.T, m Model) string {
	t.Helper()

	row, ok := m.moveTree(m.viewWidth()).SelectedRow()
	if !ok {
		t.Fatal("the picker has no row under the cursor")
	}
	return row.Key
}

// m lifts the item and everything under it out of the tree, so what is left is the tree as it will
// be after the move.
func TestMoveLiftsTheItemAndItsDescendantsOutOfTheTree(t *testing.T) {
	m, _ := filing(t)

	// j puts the cursor on nktt, the branch's middle row, which carries 4h2k beneath it.
	m, _ = press(t, m, "down", "m")

	view := plain(m.View())
	if strings.Contains(view, blockedTitle) {
		t.Errorf("the picker still draws the descendant coming with the item:\n%s", view)
	}
	if !strings.Contains(view, "moving  nktt  "+workersTitle) {
		t.Errorf("the header does not name the item being moved:\n%s", view)
	}
	if !strings.Contains(view, "+ 1 child") {
		t.Errorf("the header does not count the descendants coming with it:\n%s", view)
	}
}

// The lifted subtree is absent from the destinations too, which is what makes the mode safe rather
// than merely convenient: a move the app would have to reject is not on screen to choose.
func TestMoveOffersNeitherTheItemNorItsDescendants(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "down", "m")

	offered := destinations(m)
	for _, gone := range []string{"nktt", "4h2k"} {
		if slices.Contains(offered, gone) {
			t.Errorf("the picker offers %q as a destination: %v", gone, offered)
		}
	}
	for _, want := range []string{moveTopKey, "vq2n"} {
		if !slices.Contains(offered, want) {
			t.Errorf("the picker dropped %q from its destinations: %v", want, offered)
		}
	}
}

// The picker never applies §3's pass: filing the first item under a fresh project is the reason to
// have one, so an empty heading is a destination whether or not the list is showing it.
func TestMoveOffersAnEmptyHeadingWhicheverWayTheToggleIsSet(t *testing.T) {
	base := looped(t, 140, loopsWithShelf())
	revealed, _ := press(t, base, "h")

	for _, c := range []struct {
		toggle string
		m      Model
	}{{"off", base}, {"on", revealed}} {
		picking, _ := press(t, c.m, "m")

		if offered := destinations(picking); !slices.Contains(offered, "prtw") {
			t.Errorf("with the toggle %s the picker drops the empty heading: %v", c.toggle, offered)
		}
	}
}

// The picker opens on its first two levels: every root is expanded, so the roots and the rows filed
// directly under them are both there to point at, and what hangs deeper stays folded.
func TestMoveOpensOnTheFirstTwoLevels(t *testing.T) {
	m, _ := filing(t)

	// G is the last row of the list, which the fixture leaves at the top level, so the branch beside
	// it is left whole.
	m, _ = press(t, m, "G", "m")

	view := plain(m.View())
	if !strings.Contains(lineFor(t, view, branchTitle), foldOpen) {
		t.Errorf("a root of the tree opened closed:\n%s", view)
	}
	if !strings.Contains(view, workersTitle) {
		t.Errorf("a row filed under a root is not on screen:\n%s", view)
	}
	if strings.Contains(view, blockedTitle) {
		t.Errorf("a row three levels down opened with the picker:\n%s", view)
	}
}

// A parent deeper than those two levels is still on screen: the path down to it opens with the mode,
// which is what leaves the cursor on the item's current parent wherever it sits.
func TestMoveOpensAlongThePathToADeepCurrentParent(t *testing.T) {
	// A chain four deep, so the parent of the deepest row sits a level below what opens by default.
	m := looped(t, 140, loopsChain(4))

	m, _ = press(t, m, "G", "m")

	if view := plain(m.View()); !strings.Contains(view, "level 2") {
		t.Errorf("the item's parent is not on screen:\n%s", view)
	}
	if got := focused(t, m); got != "n002" {
		t.Errorf("the cursor opened on %q, want the item's current parent", got)
	}
}

// A root with nothing under it is a destination in its own right, so the roots are never gathered
// into the bucket: an item is filed beside them, and a bucket takes no drop.
func TestMoveOffersTheLooseRootsAsRowsOfTheirOwn(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "down", "down", "m")

	view := plain(m.View())
	if !strings.Contains(view, spikeTitle) {
		t.Errorf("a loose root is not offered as a row of its own:\n%s", view)
	}
	for _, row := range m.moveTree(m.viewWidth()).allRows() {
		if row.Depth == 0 && strings.HasSuffix(row.Key, tree.UnassignedTitle) {
			t.Errorf("the picker gathered its roots into a bucket: %v", destinations(m))
		}
	}
}

// The cursor opens on the item's current parent, so an immediate enter is a no-op rather than a
// surprise — and a nudge to a sibling is one keystroke.
func TestMoveOpensOnTheCurrentParentAndCommitsNothing(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "down", "down", "m")
	if got := focused(t, m); got != "nktt" {
		t.Errorf("the cursor opened on %q, want the item's current parent", got)
	}

	m, _ = press(t, m, "enter")
	assertLog(t, path, nil)
	if _, written := m.lastReceipt(); written {
		t.Error("dropping on the current parent left a receipt, want the move treated as a no-op")
	}
	if m.loops.moving.active() {
		t.Error("the picker is still open after enter")
	}
}

// An item at the top level opens on the synthetic row that means one, since the roots are items and
// nothing else on screen says no parent.
func TestMoveOpensOnTheTopLevelRowForAnItemWithNoParent(t *testing.T) {
	m, _ := filing(t)

	// G is the last row of the list, which the fixture leaves at the top level.
	m, _ = press(t, m, "G", "m")

	if got := focused(t, m); got != moveTopKey {
		t.Errorf("the cursor opened on %q, want the top-level row", got)
	}
	if view := plain(m.View()); !strings.Contains(view, moveTopTitle) {
		t.Errorf("the picker draws no top-level row:\n%s", view)
	}
}

// The top-level row is a destination like any other, and writes a null parent.
func TestMoveDropsOnTheTopLevelRow(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "down", "down", "m", "g", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":null}`})

	if got := parentOf(t, m, "4h2k"); got != nil {
		t.Errorf("the item sits under %q, want it at the top level", *got)
	}
	if view := plain(m.View()); !strings.Contains(view, "filed 4h2k  "+topLevel) {
		t.Errorf("the receipt does not name the top level:\n%s", view)
	}
}

// Every real row is a destination, leaves included: dropping onto a leaf makes it a parent.
func TestMoveDropsOntoALeafAndMakesItAParent(t *testing.T) {
	m, path := filing(t)

	// The picker opens on nktt, a leaf itself once 4h2k has left it; the row above is the branch's
	// other leaf.
	m, _ = press(t, m, "down", "down", "m", "up")
	if got := focused(t, m); got != "sga9" {
		t.Fatalf("the cursor is on %q, want the leaf beside the item's parent", got)
	}

	m, _ = press(t, m, "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":"sga9"}`})

	if got := parentOf(t, m, "4h2k"); got == nil || *got != "sga9" {
		t.Errorf("the item sits under %v, want it moved under sga9", got)
	}
	// The receipt names the destination by its title, which the status line clips to the width it has.
	if view := plain(m.View()); !strings.Contains(view, "filed 4h2k  Design template") {
		t.Errorf("the footer carries no receipt for the move:\n%s", view)
	}
}

// The (unassigned) bucket is a rendering artifact rather than an item, so it takes no drop: dropping
// into it is dropping onto the fold it hangs under, which is a row of its own.
func TestMoveRefusesTheUnassignedBucket(t *testing.T) {
	m, path := filing(t)

	// Moving the fixture's last row leaves the branch whole, so the bucket under it is the one level
	// still holding parents and leaves side by side: three rows down from the top-level row.
	m, _ = press(t, m, "G", "m", "down", "down", "down")
	if got := focused(t, m); !strings.HasSuffix(got, tree.UnassignedTitle) {
		t.Fatalf("the cursor is on %q, want the branch's bucket", got)
	}

	m, _ = press(t, m, "enter")

	assertLog(t, path, nil)
	if !m.loops.moving.active() {
		t.Error("the picker closed on a row that takes no drop")
	}
	if !strings.Contains(m.hint, "not a destination") {
		t.Errorf("the hint is %q, want it to say the bucket takes no drop", m.hint)
	}
}

// → opens a fold and ← closes it again, so a destination behind a closed branch is reached without
// enter — which cannot toggle here, because it commits.
func TestMoveExpandsAndCollapsesWithoutCommitting(t *testing.T) {
	m, path := filing(t)

	// Two rows down from the top-level row is nktt, which the picker opened closed over 4h2k.
	m, _ = press(t, m, "G", "m", "down", "down", "right")
	if view := plain(m.View()); !strings.Contains(view, blockedTitle) {
		t.Errorf("→ did not open the fold under the cursor:\n%s", view)
	}

	m, _ = press(t, m, "left")
	if view := plain(m.View()); strings.Contains(view, blockedTitle) {
		t.Errorf("← did not close the fold under the cursor:\n%s", view)
	}
	assertLog(t, path, nil)
}

// esc abandons the move, leaving neither an event nor a receipt behind.
func TestMoveEscapesWithoutWriting(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "down", "down", "m", "esc")

	assertLog(t, path, nil)
	if m.loops.moving.active() {
		t.Error("esc left the picker open")
	}
	if _, written := m.lastReceipt(); written {
		t.Error("esc left a receipt, want the move abandoned")
	}
}

// The inverse of a move is the parent the item held before it, which is what makes one reversible.
func TestUndoOfAMoveRestoresTheParentTheItemHeld(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "down", "down", "m", "g", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":null}`,
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":"nktt"}`,
	})

	if got := parentOf(t, m, "4h2k"); got == nil || *got != "nktt" {
		t.Errorf("the undone item sits under %v, want it back under nktt", got)
	}
}

// A golden of the picker mid-move at the two widths §8 names: the header naming what is being moved
// and what comes with it, the synthetic top-level row, the folds opened along the path to the
// current parent, and the lifted subtree absent from what is left (§9).
func TestGoldenMoveAt80Columns(t *testing.T) { goldenMove(t, 80) }

func TestGoldenMoveAt140Columns(t *testing.T) { goldenMove(t, 140) }

func goldenMove(t *testing.T, width int) {
	t.Helper()

	// Two rows down is 4h2k, the deepest row of the three-deep fixture.
	m, _ := press(t, looped(t, width, loopsNested()), "down", "down", "m")
	teatest.RequireEqualOutput(t, []byte(plain(m.View())))
}

// The other direction of the same pair: an item moved off the top level is put back on it, which the
// inverse event carries as an explicit null. An update that omitted the key would leave the item
// under its new parent, so the top level is the half worth pinning separately.
func TestUndoOfAMoveOffTheTopLevelPutsItBack(t *testing.T) {
	m, path := filing(t)

	// G is the last row of the list, which the fixture leaves at the top level; j off the synthetic
	// row the picker opens on is the branch's root.
	m, _ = press(t, m, "G", "m")
	if got := focused(t, m); got != moveTopKey {
		t.Fatalf("the picker opened on %q, want the top-level row", got)
	}

	m, _ = press(t, m, "down", "enter")
	if got := parentOf(t, m, "p0rt"); got == nil || *got != "vq2n" {
		t.Fatalf("the item sits under %v, want it moved under vq2n", got)
	}

	m, _ = press(t, m, "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"p0rt","parent":"vq2n"}`,
		`{"ts":"` + ts + `","ev":"update","id":"p0rt","parent":null}`,
	})

	if got := parentOf(t, m, "p0rt"); got != nil {
		t.Errorf("the undone item sits under %q, want it back at the top level", *got)
	}
	if view := plain(m.View()); !strings.Contains(view, "filed p0rt  "+topLevel) {
		t.Errorf("the undo receipt does not name the top level:\n%s", view)
	}
}

// m with rows marked lifts all of them into one picker and files each under the row it is dropped on.
func TestMoveFilesEveryMarkedItem(t *testing.T) {
	m, path := working(t)

	// Four rows down is 9xz1, and the row under it is p0rt: both sit at the top level, so the picker
	// opens on the row that means it and one step down is the fold.
	m, _ = press(t, m, "down", "down", "down", "down", "x", "down", "x", "m")
	if view := plain(m.View()); !strings.Contains(view, "moving  2 items") {
		t.Fatalf("the header does not count what is being moved:\n%s", view)
	}
	if got := focused(t, m); got != moveTopKey {
		t.Fatalf("the picker opened on %q, want the parent the items share", got)
	}

	m, _ = press(t, m, "down", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"9xz1","parent":"vq2n"}`,
		`{"ts":"` + ts + `","ev":"update","id":"p0rt","parent":"vq2n"}`,
	})
	if view := plain(m.View()); !strings.Contains(view, "filed 2 items  "+branchTitle) {
		t.Errorf("the footer carries no summary of the move:\n%s", view)
	}
	if len(m.loops.marked) != 0 {
		t.Errorf("the marks are %v after the move, want them cleared", m.loops.marked)
	}

	m, _ = press(t, m, "u")
	for _, id := range []string{"9xz1", "p0rt"} {
		if got := parentOf(t, m, id); got != nil {
			t.Errorf("%s sits under %q after the undo, want it back at the top level", id, *got)
		}
	}
}

// With several subjects lifted, none of them and nothing under any of them is on offer.
func TestMoveOffersNoMarkedItemNorItsDescendants(t *testing.T) {
	m, _ := filing(t)

	m.loops.marked = map[string]bool{"nktt": true, "p0rt": true}
	m, _ = press(t, m, "m")

	offered := destinations(m)
	for _, gone := range []string{"nktt", "4h2k", "p0rt"} {
		if slices.Contains(offered, gone) {
			t.Errorf("the picker offers %q as a destination: %v", gone, offered)
		}
	}
	if view := plain(m.View()); !strings.Contains(view, "moving  2 items  + 1 child") {
		t.Errorf("the header does not count what comes with the marked items:\n%s", view)
	}
}

// A marked item under another marked item travels with it and keeps its place beneath it.
func TestMoveLeavesAMarkedDescendantUnderItsMarkedAncestor(t *testing.T) {
	m, path := filing(t)

	m.loops.marked = map[string]bool{"nktt": true, "4h2k": true}
	m, _ = press(t, m, "m", "g", "enter")

	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"update","id":"nktt","parent":null}`})
	if got := parentOf(t, m, "4h2k"); got == nil || *got != "nktt" {
		t.Errorf("the descendant sits under %v, want it still under nktt", got)
	}
}

// A marked item already under the destination is skipped rather than written a move that moves
// nothing, and the one write that is left is receipted as the single move it is.
func TestMoveSkipsAMarkedItemAlreadyAtTheDestination(t *testing.T) {
	m, path := working(t)

	m.loops.marked = map[string]bool{"sga9": true, "p0rt": true}
	m, _ = press(t, m, "m")
	if got := focused(t, m); got != moveTopKey {
		t.Fatalf("the picker opened on %q, want the top level for items sharing no parent", got)
	}

	m, _ = press(t, m, "down", "enter")

	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"update","id":"p0rt","parent":"vq2n"}`})
	if view := plain(m.View()); !strings.Contains(view, "filed p0rt  "+branchTitle) {
		t.Errorf("the footer does not receipt the one move made:\n%s", view)
	}
}

// Items sharing a parent open the picker on it, so an immediate enter moves nothing — and a drop
// that moves nothing leaves the marks standing.
func TestMoveOntoTheSharedParentWritesNothing(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "down", "x", "m")
	if got := focused(t, m); got != "vq2n" {
		t.Fatalf("the picker opened on %q, want the parent the items share", got)
	}

	m, _ = press(t, m, "enter")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("a drop on the current parent wrote %v", log)
	}
	if m.loops.moving.active() {
		t.Error("the picker is still open after the drop")
	}
	if len(m.loops.marked) != 2 {
		t.Errorf("the marks are %v, want both standing", m.loops.marked)
	}
}

// esc abandons the move and leaves the marks as they were.
func TestEscOutOfTheMoveKeepsTheMarks(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "x", "down", "x", "m", "esc")

	if m.loops.moving.active() {
		t.Error("the picker is still open after esc")
	}
	if len(m.loops.marked) != 2 {
		t.Errorf("the marks are %v, want both standing", m.loops.marked)
	}
}

// The picker's header over several subjects, at the narrow width where it has least room.
func TestGoldenMoveMarkedAt80Columns(t *testing.T) {
	m := looped(t, 80, loopsNested())
	m.loops.marked = map[string]bool{"nktt": true, "p0rt": true}

	m, _ = press(t, m, "m")
	teatest.RequireEqualOutput(t, []byte(plain(m.View())))
}
