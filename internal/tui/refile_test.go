package tui

import (
	"slices"
	"strings"
	"testing"
)

// The titles the parent search matches against, named here because the assertions read as prose:
// the item the cursor opens on sits under the first, and the rest are what a fragment can move it to.
const (
	workersTitle = "Background workers speak the requester's language"
	spikeTitle   = "TUI design spike"
	portTitle    = "Read the port design once more"
)

// filing is a Loops tab whose log already holds a parent, since a move is read against where the
// item sits and an item at the top level cannot show that.
func filing(t *testing.T) (Model, string) {
	t.Helper()

	m, path := working(t)
	for i := range m.loops.items {
		if m.loops.items[i].Id == "4h2k" {
			m.loops.items[i].Parent = text("nktt")
		}
	}

	// The tab opens on the heading of the branch, so the cursor is walked down to the item that now
	// sits two levels under it — the one every assertion here is about.
	m, _ = press(t, m, "j", "j")
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

// P opens on the parent the item already sits under, so a move is read against where it is rather
// than typed blind.
func TestParentOpensOnTheParentTheItemSitsUnder(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "P")

	if got := m.prompt.value; got != workersTitle {
		t.Errorf("the prompt opened on %q, want the item's parent %q", got, workersTitle)
	}
	if view := plain(m.View()); !strings.Contains(view, "parent 4h2k "+workersTitle) {
		t.Errorf("the prompt does not name the item and its parent:\n%s", view)
	}
}

// An item at the top level opens on nothing to clear, so the first keystroke searches.
func TestParentOpensEmptyForAnItemAtTheTopLevel(t *testing.T) {
	m, _ := filing(t)

	// G is the last row of the list, which the fixture leaves at the top level.
	m, _ = press(t, m, "G", "P")

	if m.prompt.subject != "p0rt" {
		t.Fatalf("the prompt addresses %q, want the item at the top level", m.prompt.subject)
	}
	if m.prompt.value != "" {
		t.Errorf("the prompt opened on %q, want it empty", m.prompt.value)
	}
}

// A fragment matches any item the log holds, case-insensitively, and the footer lists what it
// matched so enter is never a guess.
func TestParentMatchesAFragmentAgainstItemTitles(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "PORT DESIGN")

	// The list above the footer draws every item, so the offer is read off the footer's own lines
	// rather than the whole view.
	offered := plain(m.parentChoices())
	if !strings.Contains(offered, "› "+portTitle) {
		t.Errorf("the footer does not offer the item the fragment matched:\n%s", offered)
	}
	if strings.Contains(offered, spikeTitle) {
		t.Errorf("the footer offers an item the fragment does not match:\n%s", offered)
	}
}

// tab moves through the matches when a fragment names more than one, wrapping at the end. They are
// offered by title, so the order does not shift as the items underneath are touched.
func TestParentTabMovesThroughTheMatches(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "design")
	if got := m.parentMatches(m.prompt.subject, m.prompt.value); len(got) != 3 {
		t.Fatalf("design matched %v, want the three items whose titles carry it", got)
	}

	m, _ = press(t, m, "tab")
	if offered := plain(m.parentChoices()); !strings.Contains(offered, "› "+portTitle) {
		t.Errorf("tab did not move to the second match:\n%s", offered)
	}

	m, _ = press(t, m, "tab", "tab")
	if offered := plain(m.parentChoices()); !strings.Contains(offered, "› Design template") {
		t.Errorf("tab did not wrap back to the first match:\n%s", offered)
	}
}

// Enter moves the item under the match it is on: the log takes the move and the loaded item carries
// its new parent in the frame the key was pressed in.
func TestParentMovesTheItemUnderTheChosenMatch(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "tui design", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":"9xz1"}`})

	// The receipt names the parent the way every other line of prose does — by its title.
	if view := plain(m.View()); !strings.Contains(view, "filed 4h2k  "+spikeTitle) {
		t.Errorf("the footer carries no receipt for the move:\n%s", view)
	}
	if got := parentOf(t, m, "4h2k"); got == nil || *got != "9xz1" {
		t.Errorf("the item sits under %v, want it moved under 9xz1", got)
	}
}

// A prompt answered with nothing moves the item to the top level, which is the one prompt where an
// empty answer is a write rather than an abandoned edit.
func TestParentClearedMovesTheItemToTheTopLevel(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":null}`})

	if got := parentOf(t, m, "4h2k"); got != nil {
		t.Errorf("the item sits under %q, want it at the top level", *got)
	}
	if view := plain(m.View()); !strings.Contains(view, "filed 4h2k  "+topLevel) {
		t.Errorf("the receipt does not name the top level:\n%s", view)
	}
}

// The inverse of a move is the parent the item held before it, null included — which is what makes
// moving a top-level item reversible.
func TestUndoOfAMoveRestoresTheParentTheItemHeld(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "tui design", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":"9xz1"}`,
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","parent":"nktt"}`,
	})

	if got := parentOf(t, m, "4h2k"); got == nil || *got != "nktt" {
		t.Errorf("the undone item sits under %v, want it back under nktt", got)
	}
}

// Committing the parent the item already sits under is not a write: the log would hold a move that
// moved nothing.
func TestParentCommittedUnchangedWritesNothing(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "enter")

	assertLog(t, path, nil)
	if _, written := m.lastReceipt(); written {
		t.Error("an unchanged parent left a receipt, want the prompt treated as abandoned")
	}
}

// A fragment matching nothing leaves the item where it is, and the footer says so rather than
// writing a parent that does not exist — there is no path to fall back on, since a path is no longer
// a place in the tree.
func TestParentSaysSoWhenAFragmentMatchesNothing(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "nowhere", "enter")

	assertLog(t, path, nil)
	if !strings.Contains(m.hint, "nowhere") {
		t.Errorf("the hint is %q, want it to name the fragment that matched nothing", m.hint)
	}
}

// An item cannot be moved under itself or under anything already hanging beneath it, so neither is
// ever offered as a destination.
func TestParentCandidatesExcludeTheItemAndItsDescendants(t *testing.T) {
	m, _ := filing(t)

	var offered []string
	for _, candidate := range m.parentCandidates("nktt") {
		offered = append(offered, candidate.Id)
	}

	if slices.Contains(offered, "nktt") {
		t.Errorf("the candidates offer the item itself: %v", offered)
	}
	if slices.Contains(offered, "4h2k") {
		t.Errorf("the candidates offer a descendant of the item: %v", offered)
	}
	if !slices.Contains(offered, "9xz1") {
		t.Errorf("the candidates dropped an item that is neither: %v", offered)
	}
}

// The (unassigned) bucket is a rendering artifact rather than an item, so P is inert there and the
// footer says which row it was pressed on (§4).
func TestParentIsInertOnTheBucket(t *testing.T) {
	m, _ := filing(t)

	// The row below the branch's one child is the bucket its siblings were gathered into.
	m, _ = press(t, m, "j", "P")

	if m.prompt.kind != promptNone {
		t.Error("P opened a prompt on the bucket, want it inert")
	}
	if !strings.Contains(m.hint, "this row is a project") {
		t.Errorf("the hint is %q, want it to name the row P was pressed on", m.hint)
	}
}
