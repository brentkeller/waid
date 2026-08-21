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

	m, _ = press(t, m, "ctrl+u", "Design template", "enter")

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

	m, _ = press(t, m, "e", "ctrl+u", "q")
	if m.prompt.value != "q" {
		t.Errorf("q typed into the prompt left %q, want it treated as text", m.prompt.value)
	}
}

// adding is a model over a temp home holding every tab's fixture, left on the tab named, so an add
// can be made from any of the three against a log the test reads back.
func adding(t *testing.T, on tab) (Model, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	opts := Options{Cfg: config.Config{EventsPath: path}, Now: loopsNow, Ids: ids.Sequence("7k3m")}

	sized, _ := New(opts).Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	m := offline(sized.(Model), scanFixture())
	m.clock = func() time.Time { return loopsNow }

	for _, msg := range []tea.Msg{loopsFixture(), scanLoadedMsg{result: scanFixture(), at: scannedAt}, reviewFixture()} {
		next, _ := m.Update(msg)
		m = next.(Model)
	}

	m, _ = press(t, m, string(rune('1'+on)))
	return m, path
}

// a declares an item from any tab, filed under the project the row the cursor is on belongs to (§4).
// Review's cursor is on a fold, since its projects are collapsed by default — a project on one line
// is still a project to file against.
func TestAddDeclaresAnItemFromAnyTab(t *testing.T) {
	cases := []struct {
		tab     tab
		to      []string
		project string
		logged  string
	}{
		{tabLoops, nil, `C:\dev\dr\devresults\devresults`, `"C:\\dev\\dr\\devresults\\devresults"`},
		{tabScan, []string{"j", "j"}, `C:\dev\waid`, `"C:\\dev\\waid"`},
		{tabReview, nil, `C:\dev\dr\devresults\devresults`, `"C:\\dev\\dr\\devresults\\devresults"`},
	}

	for _, c := range cases {
		t.Run(tabTitles[c.tab], func(t *testing.T) {
			m, path := adding(t, c.tab)
			m, _ = press(t, m, c.to...)

			m, _ = press(t, m, "a")
			if m.prompt.kind != promptAdd {
				t.Fatalf("a opened no prompt for the title")
			}
			if view := plain(m.View()); !strings.Contains(view, "add "+projectLabel(&c.project)) {
				t.Errorf("the prompt does not name the project the item lands in:\n%s", view)
			}

			m, cmd := press(t, m, "Ship the add key", "enter")
			if cmd != nil {
				t.Error("the answer issued a command, want the write made on the keypress")
			}

			assertLog(t, path, []string{
				`{"ts":"` + loopsStamped() + `","ev":"add","id":"7k3m","title":"Ship the add key","status":"open",` +
					`"origin":` + c.logged + `,"session":null,"tags":[],"waitingOn":null}`,
			})
			if view := plain(m.View()); !strings.Contains(view, "added 7k3m  Ship the add key") {
				t.Errorf("the footer carries no receipt for the add:\n%s", view)
			}
		})
	}
}

// The declared item is on the Loops tab in the frame it was added in, wherever it was added from,
// rather than waiting for the next read of the log to bring it in.
func TestAddPutsTheItemInTheListItLandsIn(t *testing.T) {
	m, _ := adding(t, tabScan)
	before := m.counts[tabLoops]

	m, _ = press(t, m, "a", "Ship the add key", "enter", "1")

	view := plain(m.View())
	if !listed(view, "7k3m") {
		t.Errorf("the declared item is not in the list:\n%s", view)
	}
	if got, want := m.counts[tabLoops], before+1; got != want {
		t.Errorf("the Loops badge is %d after the add, want %d", got, want)
	}
	if row := rowFor(t, view, "7k3m"); !strings.Contains(row, "open") || !strings.Contains(row, "Ship the add key") {
		t.Errorf("the row is %q, want the declared item open", row)
	}
}

// An add against a row belonging to no project declares an item belonging to none, which is what
// `waid add` outside a checkout writes.
func TestAddWithoutAProjectDeclaresOneWithoutAProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	m := offline(chrome(t, 140), detect.Result{})
	m.opts.Cfg.EventsPath, m.opts.Ids = path, ids.Sequence("7k3m")
	m.clock = func() time.Time { return loopsNow }

	m, _ = press(t, m, "a", "Ship the add key", "enter")

	assertLog(t, path, []string{
		`{"ts":"` + loopsStamped() + `","ev":"add","id":"7k3m","title":"Ship the add key","status":"open",` +
			`"origin":null,"session":null,"tags":[],"waitingOn":null}`,
	})
}

// The log holds no event that takes an item out of it, so undoing an add closes the item it declared
// and leaves the reversal visible in the log (§3).
func TestUndoOfAnAddClosesTheItem(t *testing.T) {
	m, path := adding(t, tabLoops)

	m, _ = press(t, m, "a", "Ship the add key", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"add","id":"7k3m","title":"Ship the add key","status":"open",` +
			`"origin":"C:\\dev\\dr\\devresults\\devresults","session":null,"tags":[],"waitingOn":null}`,
		`{"ts":"` + ts + `","ev":"close","id":"7k3m"}`,
	})

	view := plain(m.View())
	if listed(view, "7k3m") {
		t.Errorf("the item the undo closed is still in the list:\n%s", view)
	}
	if !strings.Contains(view, "closed 7k3m  Ship the add key") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
}

// esc abandons the title without writing anything, and an empty answer is an abandoned add rather
// than an item titled with nothing.
func TestAddAbandonsWithoutWriting(t *testing.T) {
	for _, ending := range []string{"esc", "enter"} {
		m, path := adding(t, tabLoops)

		m, _ = press(t, m, "a", ending)
		if m.prompt.kind != promptNone {
			t.Errorf("a left the prompt open after %s", ending)
		}
		if log := written(t, path); len(log) != 0 {
			t.Errorf("an add abandoned with %s wrote %v", ending, log)
		}
	}
}

// e opens the input on the title it is correcting, so an edit that changes a word is a word's worth
// of typing rather than a retype of the whole line.
func TestEditOpensOnTheTitleItCorrects(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "e")

	if got, want := m.prompt.value, "Deploy blocked until the migration is approved"; got != want {
		t.Errorf("the edit opened on %q, want the title it corrects %q", got, want)
	}
	if view := plain(m.View()); !strings.Contains(view, "edit 4h2k Deploy blocked") {
		t.Errorf("the prefilled title is not in the footer:\n%s", view)
	}
}

// Committing a prefilled title unchanged is not a write: the log would hold an update saying nothing.
func TestEditCommittedUnchangedWritesNothing(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "e", "enter")

	assertLog(t, path, nil)
	if _, written := m.lastReceipt(); written {
		t.Error("an unchanged title left a receipt, want the edit treated as abandoned")
	}
}

// ctrl-u empties the input, which is what makes a prefilled prompt rewritable from nothing. It is not
// shift-backspace: terminals send the same byte for that as for a plain backspace.
func TestCtrlUClearsThePromptInput(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "e", "ctrl+u")

	if m.prompt.value != "" {
		t.Errorf("ctrl-u left %q in the input, want it emptied", m.prompt.value)
	}
	if m.prompt.kind != promptRename {
		t.Error("ctrl-u closed the prompt, want it left open to type into")
	}
}

// ctrl-u clears whatever prompt is open, not only the one that opens prefilled.
func TestCtrlUClearsAnAddInProgress(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "a", "h", "i", "ctrl+u")

	if m.prompt.value != "" {
		t.Errorf("ctrl-u left %q in the add, want it emptied", m.prompt.value)
	}
}
