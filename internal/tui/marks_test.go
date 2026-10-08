package tui

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

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

// x marks the row under the cursor, so rows are marked one at a time as the cursor reaches them.
func TestXMarksTheRowUnderTheCursor(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "down", "x")

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

// A second press on a marked row takes the mark off, and the cursor has not moved between them.
func TestXTakesTheMarkOffAMarkedRow(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "x", "x")

	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v, want nothing", got)
	}
	if item, _ := m.loopsTree(m.viewWidth()).SelectedItem(); item.Id != "sga9" {
		t.Errorf("the cursor is on %q, want it left on sga9", item.Id)
	}
}

// Marking the last row leaves the cursor on it.
func TestXOnTheLastRowLeavesTheCursorThere(t *testing.T) {
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

	m, _ = press(t, m, "down", "x", "down", "x", "X")

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

	m, _ = press(t, m, "down", "x", "down", "x")
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

	m, _ := press(t, looped(t, width, loopsFixture()), "x", "down", "x")
	teatest.RequireEqualOutput(t, []byte(plain(m.View())))
}

// The keys that address one item keep addressing the row under the cursor while rows are marked, and
// leave the marks as they found them.
func TestSingleItemKeysIgnoreTheMarks(t *testing.T) {
	copied := []string{}
	restore := copyText
	copyText = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	t.Cleanup(func() { copyText = restore })

	// sga9 is marked and the cursor is then moved off it, onto nktt.
	marked := func(t *testing.T) (Model, string) {
		t.Helper()

		m, path := working(t)
		m, _ = press(t, m, "down", "x", "down")
		return m, path
	}

	for _, c := range []struct {
		pressed string
		kind    promptKind
	}{{"e", promptRename}, {"n", promptNote}} {
		m, _ := marked(t)
		m, _ = press(t, m, c.pressed)

		if m.prompt.kind != c.kind || m.prompt.subject != "nktt" {
			t.Errorf("%s opened prompt %d on %q, want prompt %d on the cursor row nktt",
				c.pressed, m.prompt.kind, m.prompt.subject, c.kind)
		}
		if got, want := markedIds(m), []string{"sga9"}; !slices.Equal(got, want) {
			t.Errorf("%s left the marks %v, want %v", c.pressed, got, want)
		}
	}

	// a files beside the cursor row and A beneath it, wherever the marks are.
	for pressed, parent := range map[string]string{"a": "vq2n", "A": "nktt"} {
		m, _ := marked(t)
		m, _ = press(t, m, pressed)

		target := m.prompt.target.parent
		if m.prompt.kind != promptAdd || target == nil || *target != parent {
			t.Errorf("%s opened prompt %d filing under %v, want an add under %s", pressed, m.prompt.kind, target, parent)
		}
		if got, want := markedIds(m), []string{"sga9"}; !slices.Equal(got, want) {
			t.Errorf("%s left the marks %v, want %v", pressed, got, want)
		}
	}

	m, path := marked(t)
	m, _ = press(t, m, "H")
	assertLog(t, path, []string{headingLine("nktt", true)})
	if got, want := markedIds(m), []string{"sga9"}; !slices.Equal(got, want) {
		t.Errorf("H left the marks %v, want %v", got, want)
	}

	m, _ = marked(t)
	m, cmd := press(t, m, "y")
	m = deliver(t, m, cmd)
	if want := []string{"nktt"}; !slices.Equal(copied, want) {
		t.Errorf("y copied %v, want %v", copied, want)
	}
	if got, want := markedIds(m), []string{"sga9"}; !slices.Equal(got, want) {
		t.Errorf("y left the marks %v, want %v", got, want)
	}
}

// A row that is both marked and under the cursor draws the mark in the gutter and keeps the cursor's
// own glyph beside it.
func TestAMarkedRowUnderTheCursorDrawsBothGlyphs(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "x")

	row := rowFor(t, plain(m.View()), "sga9")
	if !strings.HasPrefix(row, markGlyph) || !strings.Contains(row, cursorMark) {
		t.Errorf("the row is %q, want the mark in its gutter and the cursor beside it", row)
	}

	m, _ = press(t, m, "down")
	if row := rowFor(t, plain(m.View()), "sga9"); strings.Contains(row, cursorMark) {
		t.Errorf("the row is %q once the cursor has left it, want the mark alone", row)
	}
}

// d closes every marked item in the order the log holds them, leaves a line per item for the replay
// and a count for the footer, and clears the marks it answered.
func TestDoneClosesEveryMarkedItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "down", "x", "d")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"close","id":"nktt"}`,
		`{"ts":"` + ts + `","ev":"close","id":"sga9"}`,
	})

	view := plain(m.View())
	if listed(view, "sga9") || listed(view, "nktt") {
		t.Errorf("a closed item is still in the list:\n%s", view)
	}
	if !strings.Contains(view, "closed 2 items") {
		t.Errorf("the footer carries no summary of the batch:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v after the batch, want the marks cleared", got)
	}

	replayed := replay(m.receipts)
	if !strings.Contains(replayed, "nktt") || !strings.Contains(replayed, "sga9") || strings.Contains(replayed, "2 items") {
		t.Errorf("the replay is\n%s\nwant a line per item and no summary", replayed)
	}
}

// One u reverses the whole batch, and does not put the marks back.
func TestUndoOfABatchDoneReopensEveryItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "down", "x", "d", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"close","id":"nktt"}`,
		`{"ts":"` + ts + `","ev":"close","id":"sga9"}`,
		`{"ts":"` + ts + `","ev":"reopen","id":"nktt"}`,
		`{"ts":"` + ts + `","ev":"reopen","id":"sga9"}`,
	})

	view := plain(m.View())
	if !listed(view, "sga9") || !listed(view, "nktt") {
		t.Errorf("a reopened item did not come back:\n%s", view)
	}
	if !strings.Contains(view, "restored 2 items") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("the undo put back the marks %v", got)
	}
}

// The close guard judges the set as one: a heading marked with everything open beneath it closes.
func TestDoneClosesAHeadingMarkedWithItsDescendants(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "x", "down", "x", "down", "x", "d")

	if log := written(t, path); len(log) != 3 {
		t.Errorf("the batch wrote %v, want the heading and both items closed", log)
	}
}

// A heading marked with a descendant left out is refused whole: nothing is written, and the marks
// stand so the set can be put right.
func TestDoneRefusesAMarkedHeadingWithADescendantLeftOut(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "x", "down", "x", "d")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("a refused batch wrote %v", log)
	}
	if !strings.Contains(m.hint, "cannot close vq2n") {
		t.Errorf("hint = %q, want the guard's refusal", m.hint)
	}
	if got := markedIds(m); len(got) != 2 {
		t.Errorf("marked %v after a refusal, want both marks standing", got)
	}
}

// An item already closed is skipped, and a set holding nothing else is a hint and no write.
func TestDoneSkipsMarkedItemsAlreadyClosed(t *testing.T) {
	m, path := working(t)

	m.loops.marked = map[string]bool{"shut": true, "p0rt": true}
	mixed, _ := press(t, m, "d")
	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"close","id":"p0rt"}`})
	if view := plain(mixed.View()); !strings.Contains(view, "closed p0rt") {
		t.Errorf("a batch that closed one item did not receipt it as one:\n%s", view)
	}

	m.loops.marked = map[string]bool{"shut": true}
	closed, _ := press(t, m, "d")
	if log := written(t, path); len(log) != 1 {
		t.Errorf("a set of closed items wrote %v", log[1:])
	}
	if !strings.Contains(closed.hint, "already closed") {
		t.Errorf("hint = %q, want it to say the item is closed", closed.hint)
	}
}

// A mark the query is hiding is still part of the set, so the batch closes it.
func TestDoneClosesAMarkedItemTheFilterHides(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "/", "s", "p", "i", "k", "e", "enter", "d")

	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"close","id":"sga9"}`})
}

// A write that cannot land says so, records nothing, and leaves the marks standing for a retry.
func TestAFailedBatchKeepsItsMarks(t *testing.T) {
	m, _ := working(t)
	m, _ = press(t, m, "down", "x", "down", "x")

	// A directory is not a file a line can be appended to.
	m.opts.Cfg.EventsPath = t.TempDir()
	m, _ = press(t, m, "d")

	if !strings.Contains(m.hint, "done failed") {
		t.Errorf("hint = %q, want it to say the write failed", m.hint)
	}
	if len(m.receipts) != 0 {
		t.Errorf("a failed batch left %d receipts", len(m.receipts))
	}
	if got := markedIds(m); len(got) != 2 {
		t.Errorf("marked %v after a failed batch, want both marks standing", got)
	}
}

// A batch that fails part way keeps what landed: the item written is closed, receipted and undone by
// one u, the hint names the failure, and the marks stand so the rest can be tried again.
func TestABatchFailingPartWayKeepsWhatLanded(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only file still takes a write from root")
	}

	m, path := working(t)
	m, _ = press(t, m, "down", "x", "down", "x")

	// The clock is read for every append, so it is where the log can be shut between two of them.
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	m.clock = func() time.Time {
		if len(events.ReadLines(path)) == 1 {
			os.Chmod(path, 0o444)
		}
		return loopsNow
	}
	m, _ = press(t, m, "d")

	ts := loopsStamped()
	closed := `{"ts":"` + ts + `","ev":"close","id":"nktt"}`
	assertLog(t, path, []string{closed})

	if !strings.Contains(m.hint, "done failed") {
		t.Errorf("hint = %q, want it to say the write failed", m.hint)
	}
	if item, _ := m.loadedItem("nktt"); item.Status != events.StatusDone {
		t.Errorf("nktt is %q, want the write that landed applied", item.Status)
	}
	if item, _ := m.loadedItem("sga9"); item.Status != events.StatusOpen {
		t.Errorf("sga9 is %q, want it left open by the write that failed", item.Status)
	}
	if replayed := replay(m.receipts); len(m.receipts) != 1 || !strings.Contains(replayed, "nktt") {
		t.Errorf("the replay is\n%s\nwant the one item that closed", replayed)
	}
	if got, want := markedIds(m), []string{"nktt", "sga9"}; !slices.Equal(got, want) {
		t.Errorf("marked %v after a batch that failed part way, want %v", got, want)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	m.clock = func() time.Time { return loopsNow }
	m, _ = press(t, m, "u")
	assertLog(t, path, []string{closed, `{"ts":"` + ts + `","ev":"reopen","id":"nktt"}`})
	if item, _ := m.loadedItem("nktt"); item.Status != events.StatusOpen {
		t.Errorf("nktt is %q after the undo, want it reopened", item.Status)
	}
}

// w asks once who the marked items are waiting on and writes the answer to each.
func TestWaitingWritesEveryMarkedItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "down", "x", "w")
	if view := plain(m.View()); !strings.Contains(view, "waiting on (2 items)") {
		t.Fatalf("the prompt does not say how many items it is asking about:\n%s", view)
	}

	m, _ = press(t, m, "maria", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"nktt","status":"waiting","waitingOn":"maria"}`,
		`{"ts":"` + ts + `","ev":"update","id":"sga9","status":"waiting","waitingOn":"maria"}`,
	})

	view := plain(m.View())
	for _, id := range []string{"sga9", "nktt"} {
		if row := rowFor(t, view, id); !strings.Contains(row, "@maria") {
			t.Errorf("the row for %s is %q, want it waiting on maria", id, row)
		}
	}
	if !strings.Contains(view, "waiting 2 items  maria") {
		t.Errorf("the footer carries no summary of the batch:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v after the batch, want the marks cleared", got)
	}
}

// One u reverses a batch waiting: an item that was open is reopened, one that was already waiting is
// put back on the name it held, and the marks are not put back.
func TestUndoOfABatchWaitingRestoresEveryItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "G", "x", "w", "sam", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"sga9","status":"waiting","waitingOn":"sam"}`,
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","status":"waiting","waitingOn":"sam"}`,
		`{"ts":"` + ts + `","ev":"reopen","id":"sga9"}`,
		`{"ts":"` + ts + `","ev":"reopen","id":"4h2k"}`,
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","status":"waiting","waitingOn":"maria"}`,
	})

	view := plain(m.View())
	if row := rowFor(t, view, "sga9"); strings.Contains(row, "@") {
		t.Errorf("the undone row is %q, want it waiting on nobody", row)
	}
	if row := rowFor(t, view, "4h2k"); !strings.Contains(row, "@maria") {
		t.Errorf("the undone row is %q, want it back waiting on maria", row)
	}
	if !strings.Contains(view, "restored 2 items") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("the undo put back the marks %v", got)
	}
}

// An abandoned prompt writes nothing and leaves the marks standing.
func TestAnAbandonedBatchWaitingKeepsItsMarks(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "down", "x", "w", "esc")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("an abandoned prompt wrote %v", log)
	}
	if got := markedIds(m); len(got) != 2 {
		t.Errorf("marked %v, want both marks standing", got)
	}
}

// With several items marked, t opens empty and adds what is typed to the tags each item already
// carries, since no one line could stand for several different sets.
func TestTagAddsToEveryMarkedItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "down", "x", "t")
	if got := m.prompt.text(); got != "" {
		t.Errorf("the batch prompt opened on %q, want it empty", got)
	}
	if view := plain(m.View()); !strings.Contains(view, "add tags (2 items)") {
		t.Fatalf("the prompt does not say it adds to several items:\n%s", view)
	}

	m, _ = press(t, m, "bug", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"nktt","tags":["promoted","bug"]}`,
		`{"ts":"` + ts + `","ev":"update","id":"sga9","tags":["bug"]}`,
	})

	view := plain(m.View())
	if !strings.Contains(view, "tagged 2 items  bug") {
		t.Errorf("the footer carries no summary of the batch:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v after the batch, want the marks cleared", got)
	}

	m, _ = press(t, m, "u")
	view = plain(m.View())
	if row := rowFor(t, view, "nktt"); !strings.Contains(row, "[promoted]") {
		t.Errorf("the undone row is %q, want only the tag it carried before", row)
	}
	if row := rowFor(t, view, "sga9"); strings.Contains(row, "bug") {
		t.Errorf("the undone row is %q, want the added tag gone", row)
	}
}

// A blank answer to the batch prompt is an abandoned edit — it must not clear every item's tags the
// way a blank answer clears one item's — and so is an answer holding only separators.
func TestABlankBatchTagAnswerWritesNothing(t *testing.T) {
	for _, answer := range [][]string{{"enter"}, {", ,", "enter"}} {
		m, path := working(t)

		m, _ = press(t, m, "down", "x", "down", "x", "t")
		m, _ = press(t, m, answer...)

		if log := written(t, path); len(log) != 0 {
			t.Errorf("the answer %v wrote %v", answer, log)
		}
		if got := markedIds(m); len(got) != 2 {
			t.Errorf("the answer %v left the marks %v, want both standing", answer, got)
		}
	}
}

// One marked item is edited the way the cursor row is: the prompt opens on its set and replaces it.
func TestTagOnOneMarkedItemReplacesItsSet(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "down", "x", "g", "t")
	if got, want := m.prompt.text(), "promoted"; got != want {
		t.Fatalf("the prompt opened on %q, want the marked item's tags %q", got, want)
	}

	press(t, m, "ctrl+u", "bug", "enter")
	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"update","id":"nktt","tags":["bug"]}`})
}
