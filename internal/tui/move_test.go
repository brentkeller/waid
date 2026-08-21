package tui

import (
	"slices"
	"strings"
	"testing"

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
	for i := range m.loops.items {
		if m.loops.items[i].Id == "4h2k" {
			m.loops.items[i].Parent = text("nktt")
		}
	}
	return m, path
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
	m, _ = press(t, m, "j", "m")

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

	m, _ = press(t, m, "j", "m")

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

// Folds open collapsed but for the path down to the item's current parent, which is what keeps a
// tree worth building to roots-plus-one-branch.
func TestMoveOpensCollapsedAlongThePathToTheCurrentParent(t *testing.T) {
	m, _ := filing(t)

	// Two rows down is 4h2k, whose parent nktt hangs under vq2n.
	m, _ = press(t, m, "j", "j", "m")

	view := plain(m.View())
	if !strings.Contains(lineFor(t, view, branchTitle), foldOpen) {
		t.Errorf("the ancestor of the item's parent is closed:\n%s", view)
	}
	if !strings.Contains(view, workersTitle) {
		t.Errorf("the item's parent is not on screen:\n%s", view)
	}
	// The fixture's loose top-level rows were gathered into a bucket, which stays closed.
	if strings.Contains(view, spikeTitle) {
		t.Errorf("a fold off the path to the parent opened:\n%s", view)
	}
}

// The cursor opens on the item's current parent, so an immediate enter is a no-op rather than a
// surprise — and a nudge to a sibling is one keystroke.
func TestMoveOpensOnTheCurrentParentAndCommitsNothing(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "j", "j", "m")
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

	m, _ = press(t, m, "j", "j", "m", "g", "enter")

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
	m, _ = press(t, m, "j", "j", "m", "k")
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

	m, _ = press(t, m, "j", "j", "m", "G")
	if got := focused(t, m); !strings.HasSuffix(got, tree.UnassignedTitle) {
		t.Fatalf("the cursor is on %q, want the top level's bucket", got)
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

// l opens a fold and h closes it again, so a destination behind a closed branch is reached without
// enter — which cannot toggle here, because it commits.
func TestMoveExpandsAndCollapsesWithoutCommitting(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "j", "j", "m", "G", "l")
	if view := plain(m.View()); !strings.Contains(view, spikeTitle) {
		t.Errorf("l did not open the fold under the cursor:\n%s", view)
	}

	m, _ = press(t, m, "h")
	if view := plain(m.View()); strings.Contains(view, spikeTitle) {
		t.Errorf("h did not close the fold under the cursor:\n%s", view)
	}
	assertLog(t, path, nil)
}

// esc abandons the move, leaving neither an event nor a receipt behind.
func TestMoveEscapesWithoutWriting(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "j", "j", "m", "esc")

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

	m, _ = press(t, m, "j", "j", "m", "g", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":null}`,
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":"nktt"}`,
	})

	if got := parentOf(t, m, "4h2k"); got == nil || *got != "nktt" {
		t.Errorf("the undone item sits under %v, want it back under nktt", got)
	}
}
