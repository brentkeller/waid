package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/ids"
)

// working is a Loops tab over a temp home holding §1.1's log: the writes land in a file the test
// reads back, and the clock is pinned so every appended line can be asserted verbatim.
func working(t *testing.T) (Model, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	opts := Options{Cfg: config.Config{EventsPath: path}, Now: loopsNow, Ids: ids.Sequence("7k3m")}

	sized, _ := New(opts).Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	m := offline(sized.(Model), detect.Result{})
	m.clock = func() time.Time { return loopsNow }
	m.loops.load = func() tea.Msg { return loopsFixture() }

	loaded, _ := m.Update(m.loops.load())
	return loaded.(Model), path
}

// loopsStamped is the timestamp every write in these tests carries, since the model's clock is pinned.
func loopsStamped() string { return events.FormatTs(loopsNow) }

// listed reports whether the tab's body is still drawing this item. The footer's receipt names the
// item too, so a row that left the list has to be looked for above the footer alone.
func listed(view, id string) bool {
	lines := strings.Split(plain(view), "\n")
	for _, line := range lines[:max(len(lines)-3, 0)] {
		if strings.Contains(line, id) {
			return true
		}
	}
	return false
}

// x closes the item under the cursor on the keypress, with no confirm step, and the row leaves the
// list the moment it is written (§3).
func TestDoneClosesTheItemUnderTheCursor(t *testing.T) {
	m, path := working(t)

	m, cmd := press(t, m, "x")
	if cmd != nil {
		t.Error("x issued a command, want the write made on the keypress")
	}

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"close","id":"4h2k"}`})

	view := plain(m.View())
	if listed(view, "4h2k") {
		t.Errorf("the closed item is still in the list:\n%s", view)
	}
	if !strings.Contains(view, "closed 4h2k  Deploy blocked until the migration is approved") {
		t.Errorf("the footer carries no receipt for the close:\n%s", view)
	}
	if got, want := m.counts[tabLoops], 4; got != want {
		t.Errorf("the Loops badge is %d after the close, want %d", got, want)
	}
}

// The inverse of a close is a reopen, and a reopen is the one event that clears what an item was
// waiting on — so an item that was waiting on someone has that put back behind it (§3).
func TestUndoOfADoneRestoresTheItemItClosed(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "x", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"close","id":"4h2k"}`,
		`{"ts":"` + ts + `","ev":"reopen","id":"4h2k"}`,
		`{"ts":"` + ts + `","ev":"update","id":"4h2k","status":"waiting","waitingOn":"maria"}`,
	})

	view := plain(m.View())
	if !listed(view, "4h2k") {
		t.Errorf("the reopened item did not come back to the list:\n%s", view)
	}
	if row := rowFor(t, view, "4h2k"); !strings.Contains(row, "waiting") || !strings.Contains(row, "@maria") {
		t.Errorf("the restored row is %q, want it waiting on maria again", row)
	}
	if !strings.Contains(view, "reopened 4h2k") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
}

// The done and all segments list items that are already closed, so x says so there rather than
// writing a second close (§4).
func TestDoneIsInertOnAnAlreadyClosedItem(t *testing.T) {
	m, path := working(t)

	// Two presses of s put the status row on done, which is the fixture's one closed item.
	m, _ = press(t, m, "s", "s", "x")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("x on a closed item wrote %v", log)
	}
	if !strings.Contains(m.hint, "already closed") {
		t.Errorf("x on a closed item left the hint %q, want it to say the item is closed", m.hint)
	}
}

// w asks who the item is waiting on, then writes the status and the name together.
func TestWaitingAsksWhoAndWritesTheUpdate(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "w")
	if m.prompt.kind != promptWaiting {
		t.Fatalf("w opened no prompt for who the item waits on")
	}
	if view := plain(m.View()); !strings.Contains(view, "waiting on sga9") {
		t.Errorf("the prompt does not name the item it is asking about:\n%s", view)
	}

	m, cmd := press(t, m, "maria", "enter")
	if cmd != nil {
		t.Error("the answer issued a command, want the write made on the keypress")
	}

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"sga9","status":"waiting","waitingOn":"maria"}`})

	view := plain(m.View())
	if row := rowFor(t, view, "sga9"); !strings.Contains(row, "waiting") || !strings.Contains(row, "@maria") {
		t.Errorf("the row is %q, want it waiting on maria", row)
	}
	if !strings.Contains(view, "waiting sga9  maria") {
		t.Errorf("the footer carries no receipt for the write:\n%s", view)
	}
}

// The inverse of a waiting write is the status the item held before it, which for a plain open item
// is the reopen that clears both fields (§3).
func TestUndoOfAWaitingReopensTheItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "w", "maria", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"sga9","status":"waiting","waitingOn":"maria"}`,
		`{"ts":"` + ts + `","ev":"reopen","id":"sga9"}`,
	})

	view := plain(m.View())
	if row := rowFor(t, view, "sga9"); strings.Contains(row, "@maria") {
		t.Errorf("the row is %q, want the waiting-on cleared", row)
	}
	if !strings.Contains(view, "restored sga9") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
}

// e corrects the title in place, and its inverse puts the old one back.
func TestEditRewritesTheTitle(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "e")
	if view := plain(m.View()); !strings.Contains(view, "edit sga9") {
		t.Errorf("e opened no prompt for the title:\n%s", view)
	}

	m, _ = press(t, m, "Design template", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"sga9","title":"Design template"}`})

	view := plain(m.View())
	if row := rowFor(t, view, "sga9"); !strings.Contains(row, "Design template  ") {
		t.Errorf("the row is %q, want the corrected title", row)
	}
	if !strings.Contains(view, "retitled sga9  Design template") {
		t.Errorf("the footer carries no receipt for the edit:\n%s", view)
	}

	m, _ = press(t, m, "u")
	prior := "Design template + args persistence for localized persisted strings"
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"sga9","title":"Design template"}`,
		`{"ts":"` + ts + `","ev":"update","id":"sga9","title":"` + prior + `"}`,
	})
	if row := rowFor(t, plain(m.View()), "sga9"); !strings.Contains(row, "Design template + args") {
		t.Errorf("the row is %q, want the title the edit replaced", row)
	}
}

// n appends a note to the item under the cursor, which the detail pane shows without a re-read.
func TestNoteAppendsToTheItemUnderTheCursor(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "n")
	if view := plain(m.View()); !strings.Contains(view, "note 4h2k") {
		t.Errorf("n opened no prompt for the text:\n%s", view)
	}

	m, _ = press(t, m, "the migration landed", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"note","id":"4h2k","text":"the migration landed"}`})

	view := plain(m.View())
	if pane := detailPane(t, m.View()); !strings.Contains(pane, "the migration landed") {
		t.Errorf("the detail pane does not carry the note just written:\n%s", pane)
	}
	if !strings.Contains(view, "noted 4h2k  the migration landed") {
		t.Errorf("the footer carries no receipt for the note:\n%s", view)
	}
}

// The log holds no event that removes a note, so a note leaves no inverse behind and the footer stops
// offering the undo rather than offering one that would reverse the write before it (§3).
func TestANoteLeavesNothingToUndo(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "x")
	if !strings.Contains(plain(m.View()), "u undo") {
		t.Fatalf("the close does not offer an undo:\n%s", plain(m.View()))
	}

	m, _ = press(t, m, "n", "the migration landed", "enter")
	if view := plain(m.View()); strings.Contains(view, "u undo") {
		t.Errorf("the note offers an undo that would reverse the close before it:\n%s", view)
	}
}

// A project heading is not an item, so none of the four writes anything from one and the footer says
// why (§4).
func TestLoopsWritesAreInertOnAProjectHeading(t *testing.T) {
	m, path := working(t)

	// enter folds the project, which puts the cursor on the heading standing in for its children.
	folded, _ := press(t, m, "enter")

	for _, pressed := range []string{"x", "w", "e", "n"} {
		next, _ := press(t, folded, pressed)
		if log := written(t, path); len(log) != 0 {
			t.Errorf("%s on a project heading wrote %v", pressed, log)
		}
		if !strings.Contains(next.hint, "project") {
			t.Errorf("%s on a project heading left the hint %q, want it to name the row", pressed, next.hint)
		}
		if next.prompt.kind != promptNone {
			t.Errorf("%s on a project heading opened a prompt", pressed)
		}
		if len(next.receipts) != 0 {
			t.Errorf("%s on a project heading left a receipt", pressed)
		}
	}
}

// A list with nothing in it has no row to describe, so the reason names the empty list instead (§4).
func TestLoopsWritesSayWhenThereIsNothingToActOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	m := offline(chrome(t, 140), detect.Result{})
	m.opts.Cfg.EventsPath = path

	for _, pressed := range []string{"x", "w", "e", "n"} {
		next, _ := press(t, m, pressed)
		if !strings.Contains(next.hint, noItems) {
			t.Errorf("%s on an empty list left the hint %q, want %q", pressed, next.hint, noItems)
		}
		if log := written(t, path); len(log) != 0 {
			t.Errorf("%s on an empty list wrote %v", pressed, log)
		}
	}
}

// esc abandons an answer without writing anything, and an empty answer is an abandoned edit rather
// than a write of nothing.
func TestLoopsPromptsAbandonWithoutWriting(t *testing.T) {
	for _, ending := range []string{"esc", "enter"} {
		for _, pressed := range []string{"w", "e", "n"} {
			m, path := working(t)

			m, _ = press(t, m, pressed, ending)
			if m.prompt.kind != promptNone {
				t.Errorf("%s left the prompt open after %s", pressed, ending)
			}
			if log := written(t, path); len(log) != 0 {
				t.Errorf("%s abandoned with %s wrote %v", pressed, ending, log)
			}
		}
	}
}

// A write holds the keyboard while its answer is being typed, so a title containing `q` cannot quit
// the app out from under the person typing it (§4).
func TestLoopsPromptsHoldTheKeyboard(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "e", "q")
	if m.prompt.value != "q" {
		t.Errorf("q typed into the prompt left %q, want it treated as text", m.prompt.value)
	}
}
