package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"github.com/brentkeller/waid/internal/events"
)

// markedIds are the ids carrying a mark, sorted so an assertion does not depend on map order.
func markedIds(m Model) []string {
	ids := []string{}
	for id := range m.loops.marked {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// x marks the row under the cursor and steps down, so a run of rows is marked by repeating the key.
func TestXMarksTheRowAndStepsDown(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "x")

	if got, want := markedIds(m), []string{"nktt", "sga9"}; !slices.Equal(got, want) {
		t.Errorf("marked %v, want %v", got, want)
	}
	if log := written(t, path); len(log) != 0 {
		t.Errorf("marking wrote %v, want nothing", log)
	}

	view := plain(m.View())
	for _, id := range []string{"sga9", "nktt"} {
		if row := rowFor(t, view, id); !strings.HasPrefix(row, markGlyph) {
			t.Errorf("the row for %s is %q, want the mark in its gutter", id, row)
		}
	}
	if row := rowFor(t, view, "9xz1"); strings.HasPrefix(row, markGlyph) {
		t.Errorf("an unmarked row drew the mark: %q", row)
	}
}

// A second press on a marked row takes the mark off, and steps down all the same.
func TestXTakesTheMarkOffAMarkedRow(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "x", "up", "x")

	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v, want nothing", got)
	}
	if item, _ := m.loopsTree(m.viewWidth()).SelectedItem(); item.Id != "nktt" {
		t.Errorf("the cursor is on %q, want it stepped down to nktt", item.Id)
	}
}

// The last row has nowhere to step to, so the cursor stays on it.
func TestXStaysOnTheLastRow(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "G", "x")

	if got, want := markedIds(m), []string{"4h2k"}; !slices.Equal(got, want) {
		t.Errorf("marked %v, want %v", got, want)
	}
	if item, _ := m.loopsTree(m.viewWidth()).SelectedItem(); item.Id != "4h2k" {
		t.Errorf("the cursor is on %q, want it left on the last row", item.Id)
	}
}

// A mark covers the item it is on: marking a fold marks nothing filed under it.
func TestXOnAFoldMarksTheFoldAlone(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "x")

	if got, want := markedIds(m), []string{"vq2n"}; !slices.Equal(got, want) {
		t.Errorf("marked %v, want %v", got, want)
	}
	if line := lineFor(t, plain(m.View()), "Localized notifications"); !strings.HasPrefix(line, markGlyph) {
		t.Errorf("the fold is drawn %q, want the mark in its gutter", line)
	}
}

// The bucket is not an item, so it takes no mark and the footer says why.
func TestXIsInertOnTheBucket(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "down", "down", "x")

	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v, want nothing", got)
	}
	if !strings.Contains(m.hint, "project") {
		t.Errorf("hint = %q, want it to name the row", m.hint)
	}
}

// X clears every mark at once.
func TestShiftXClearsTheMarks(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "x", "x", "X")

	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v after X, want nothing", got)
	}
}

// Marks are keyed by id, so they hold while the list is filtered, cleared, re-gated and re-read —
// and esc, which clears the query, leaves them alone.
func TestMarksHoldAcrossFiltersAndReloads(t *testing.T) {
	m, _ := working(t)
	m, _ = press(t, m, "down", "x")

	m, _ = press(t, m, "/", "s", "p", "i", "k", "e", "enter")
	if view := plain(m.View()); !strings.Contains(view, "1 marked · 1 item") {
		t.Errorf("the header does not count the mark the query hides:\n%s", view)
	}

	m, _ = press(t, m, "esc", "s", "h", "left")
	next, _ := m.Update(m.loops.load())
	m = next.(Model)

	if got, want := markedIds(m), []string{"sga9"}; !slices.Equal(got, want) {
		t.Errorf("marked %v, want %v", got, want)
	}
}

// A read that no longer holds a marked item drops its mark, so the count stays honest.
func TestAReloadDropsMarksForItemsTheLogNoLongerHolds(t *testing.T) {
	m, _ := working(t)
	m, _ = press(t, m, "down", "x")

	msg := loopsFixture()
	msg.items = slices.DeleteFunc(msg.items, func(item events.Item) bool { return item.Id == "sga9" })
	next, _ := m.Update(msg)

	if got := markedIds(next.(Model)); len(got) != 0 {
		t.Errorf("marked %v, want the mark dropped with the item", got)
	}
}

// While anything is marked the header counts it and the footer offers the keys that take the set.
func TestMarksChangeTheHeaderAndTheFooter(t *testing.T) {
	m, _ := working(t)

	bare := plain(m.View())
	if strings.Contains(bare, "marked") || strings.Contains(bare, "X clear") {
		t.Errorf("an unmarked list mentions marks:\n%s", bare)
	}

	m, _ = press(t, m, "down", "x", "x")
	view := plain(m.View())
	if !strings.Contains(view, "2 marked · 6 items") {
		t.Errorf("the header does not count the marks:\n%s", view)
	}
	if !strings.Contains(view, markKeys) {
		t.Errorf("the footer does not offer the batch keys:\n%s", view)
	}
}

// x and X are Loops keys: pressed elsewhere they are inert and the footer names the tab.
func TestMarkKeysBelongToLoops(t *testing.T) {
	m, _ := working(t)

	for _, pressed := range []string{"x", "X"} {
		next, _ := press(t, m, "2", pressed)
		if !strings.Contains(next.hint, "Loops key") {
			t.Errorf("%s on Repos left the hint %q, want it to name Loops", pressed, next.hint)
		}
	}
}

// A marked list at the two widths the goldens pin: the gutter glyph on a fold and on a leaf, and the
// count beside the item count.
func TestGoldenLoopsMarkedAt80Columns(t *testing.T) { goldenMarked(t, 80) }

func TestGoldenLoopsMarkedAt140Columns(t *testing.T) { goldenMarked(t, 140) }

func goldenMarked(t *testing.T, width int) {
	t.Helper()

	m, _ := press(t, looped(t, width, loopsFixture()), "x", "x")
	teatest.RequireEqualOutput(t, []byte(plain(m.View())))
}

// The keys that address one item keep addressing the row under the cursor while rows are marked.
func TestSingleItemKeysIgnoreTheMarks(t *testing.T) {
	m, _ := working(t)

	// Marking sga9 steps the cursor onto nktt.
	m, _ = press(t, m, "down", "x", "e")

	if m.prompt.kind != promptRename || m.prompt.subject != "nktt" {
		t.Errorf("e opened prompt %d on %q, want the title of the cursor row nktt", m.prompt.kind, m.prompt.subject)
	}
}
