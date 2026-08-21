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

	// G puts the cursor on the last row of the list, which is the fixture's waiting item.
	m, _ = press(t, m, "G")

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
	if got, want := m.counts[tabLoops], 5; got != want {
		t.Errorf("the Loops badge is %d after the close, want %d", got, want)
	}
}

// The inverse of a close is a reopen, and a reopen is the one event that clears what an item was
// waiting on — so an item that was waiting on someone has that put back behind it (§3).
func TestUndoOfADoneRestoresTheItemItClosed(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "G", "x", "u")

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

	// j steps off the heading the tab opens on and onto the first row folded under it.
	m, _ = press(t, m, "j", "w")
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

	m, _ = press(t, m, "j", "w", "maria", "enter", "u")

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

	m, _ = press(t, m, "j", "e")
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

	m, _ = press(t, m, "G", "n")
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

	m, _ = press(t, m, "j", "x")
	if !strings.Contains(plain(m.View()), "u undo") {
		t.Fatalf("the close does not offer an undo:\n%s", plain(m.View()))
	}

	m, _ = press(t, m, "n", "the migration landed", "enter")
	if view := plain(m.View()); strings.Contains(view, "u undo") {
		t.Errorf("the note offers an undo that would reverse the close before it:\n%s", view)
	}
}

// The (unassigned) bucket is a rendering artifact rather than an item, so none of the four writes
// anything from one and the footer says why (§4).
func TestLoopsWritesAreInertOnTheBucket(t *testing.T) {
	m, path := working(t)

	// The bucket is the row under the heading's two children, and the cursor rests on it like any
	// other row.
	folded, _ := press(t, m, "j", "j", "j")

	for _, pressed := range []string{"x", "w", "e", "n"} {
		next, _ := press(t, folded, pressed)
		if log := written(t, path); len(log) != 0 {
			t.Errorf("%s on the bucket wrote %v", pressed, log)
		}
		if !strings.Contains(next.hint, "project") {
			t.Errorf("%s on the bucket left the hint %q, want it to name the row", pressed, next.hint)
		}
		if next.prompt.kind != promptNone {
			t.Errorf("%s on the bucket opened a prompt", pressed)
		}
		if len(next.receipts) != 0 {
			t.Errorf("%s on the bucket left a receipt", pressed)
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

// addedLine is the add these tests type, carrying whichever target the write resolved: a parent id
// on Loops, omitted when the item lands at the top level, and an origin path elsewhere.
func addedLine(parent, origin string) string {
	if parent != "" {
		parent = `"parent":"` + parent + `",`
	}
	if origin == "" {
		origin = "null"
	}
	return `{"ts":"` + loopsStamped() + `","ev":"add","id":"7k3m","title":"Ship the add key","status":"open",` +
		parent + `"origin":` + origin + `,"session":null,"tags":[],"waitingOn":null}`
}

// a declares an item from any tab, beside the row the cursor is on: under that row's parent on
// Loops, and under the project the row belongs to on Repos and Agents, which is what those two have
// always done (§7.2). Agents' cursor is on a fold, since its projects are collapsed by default — a
// project on one line is still a project to file against.
func TestAddDeclaresAnItemFromAnyTab(t *testing.T) {
	cases := []struct {
		name   string
		tab    tab
		to     []string
		lands  string
		logged string
	}{
		{"Loops", tabLoops, nil, topLevel, addedLine("", "")},
		{"Repos", tabScan, []string{"j", "j"}, "waid", addedLine("", `"C:\\dev\\waid"`)},
		{"Repos without a checkout", tabScan, nil, noProject, addedLine("", "")},
		{"Agents", tabReview, nil, "devresults", addedLine("", `"C:\\dev\\dr\\devresults\\devresults"`)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, path := adding(t, c.tab)
			m, _ = press(t, m, c.to...)

			m, _ = press(t, m, "a")
			if m.prompt.kind != promptAdd {
				t.Fatalf("a opened no prompt for the title")
			}
			if view := plain(m.View()); !strings.Contains(view, "add "+c.lands) {
				t.Errorf("the prompt does not name where the item lands:\n%s", view)
			}

			m, cmd := press(t, m, "Ship the add key", "enter")
			if cmd != nil {
				t.Error("the answer issued a command, want the write made on the keypress")
			}

			assertLog(t, path, []string{c.logged})
			if view := plain(m.View()); !strings.Contains(view, "added 7k3m  Ship the add key") {
				t.Errorf("the footer carries no receipt for the add:\n%s", view)
			}
		})
	}
}

// On Loops the two add keys differ by a tier: `a` puts the item beside the row under the cursor and
// `A` puts it underneath, which is how a tier is created (§7.2). Both name the parent in the prompt.
func TestAddKeysFileBesideAndUnderTheRow(t *testing.T) {
	cases := []struct {
		name    string
		to      []string
		pressed string
		lands   string
		parent  string
	}{
		{"a at the top level", nil, "a", topLevel, ""},
		{"a under a parent", []string{"j"}, "a", "Localized notifications", "vq2n"},
		{"A on a parent", nil, "A", "Localized notifications", "vq2n"},
		{"A on a leaf", []string{"j"}, "A", "Design template", "sga9"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, path := adding(t, tabLoops)
			m, _ = press(t, m, c.to...)

			m, _ = press(t, m, c.pressed)
			if view := plain(m.View()); !strings.Contains(view, "add "+c.lands) {
				t.Errorf("the prompt does not name where the item lands:\n%s", view)
			}

			m, _ = press(t, m, "Ship the add key", "enter")
			assertLog(t, path, []string{addedLine(c.parent, "")})
		})
	}
}

// A leaf is a parent the moment something is added under it: the tier is created by the add rather
// than declared first and filled afterwards (§7.2).
func TestAddChildTurnsALeafIntoAParent(t *testing.T) {
	m, _ := adding(t, tabLoops)

	m, _ = press(t, m, "j", "A", "Ship the add key", "enter")

	view := plain(m.View())
	if !listed(view, "7k3m") {
		t.Errorf("the declared item is not in the list:\n%s", view)
	}
	row := rowFor(t, view, "Design template")
	if !strings.Contains(row, foldOpen) || !strings.Contains(row, "1 open") {
		t.Errorf("the row the item landed under is %q, want a fold holding one open loop", row)
	}
}

// The `(unassigned)` bucket gathers a fold's leaves and is not an item, so it is not a parent
// either: both keys pressed on it land under the fold it hangs beneath (§4).
func TestAddOnTheUnassignedBucketFilesUnderWhatGathersIt(t *testing.T) {
	for _, pressed := range []string{"a", "A"} {
		m, path := adding(t, tabLoops)

		m, _ = press(t, m, "j", "j", "j", pressed)
		if view := plain(m.View()); !strings.Contains(view, "add "+topLevel) {
			t.Errorf("%s on the bucket does not land at the top level:\n%s", pressed, view)
		}

		m, _ = press(t, m, "Ship the add key", "enter")
		assertLog(t, path, []string{addedLine("", "")})
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

// An add with nothing under the cursor at all declares an item under nothing, which is what `waid
// add` outside a checkout and with no `-p` writes.
func TestAddWithNothingUnderTheCursorLandsAtTheTopLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	m := offline(chrome(t, 140), detect.Result{})
	m.opts.Cfg.EventsPath, m.opts.Ids = path, ids.Sequence("7k3m")
	m.clock = func() time.Time { return loopsNow }

	m, _ = press(t, m, "a", "Ship the add key", "enter")

	assertLog(t, path, []string{addedLine("", "")})
}

// The log holds no event that takes an item out of it, so undoing an add closes the item it declared
// and leaves the reversal visible in the log (§3). Both add keys push the same inverse.
func TestUndoOfAnAddClosesTheItem(t *testing.T) {
	cases := map[string]string{"a": "", "A": "vq2n"}

	for pressed, parent := range cases {
		t.Run(pressed, func(t *testing.T) {
			m, path := adding(t, tabLoops)

			m, _ = press(t, m, pressed, "Ship the add key", "enter", "u")

			assertLog(t, path, []string{
				addedLine(parent, ""),
				`{"ts":"` + loopsStamped() + `","ev":"close","id":"7k3m"}`,
			})

			view := plain(m.View())
			if listed(view, "7k3m") {
				t.Errorf("the item the undo closed is still in the list:\n%s", view)
			}
			if !strings.Contains(view, "closed 7k3m  Ship the add key") {
				t.Errorf("the footer carries no receipt for the undo:\n%s", view)
			}
		})
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

	m, _ = press(t, m, "G", "e")

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

// x obeys the same guard `waid done` does: a heading with open work beneath it does not close, and
// the footer says what is holding it open (§5).
func TestDoneRefusesAnItemWithOpenDescendants(t *testing.T) {
	msg := loopsFixture()
	parent := msg.items[0]

	m := looped(t, 140, msg)
	m.loops.load = func() tea.Msg { return msg }

	path := filepath.Join(t.TempDir(), "events.jsonl")
	m.opts.Cfg.EventsPath = path

	if item, ok := m.loopsTree(m.viewWidth()).SelectedItem(); !ok || item.Id != parent.Id {
		t.Fatalf("the cursor is on %v, want the parent %s", item, parent.Id)
	}

	m, _ = press(t, m, "x")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("x on a parent with open work wrote %v", log)
	}
	if !strings.Contains(m.hint, "cannot close "+parent.Id) {
		t.Errorf("hint = %q, want it to refuse the close", m.hint)
	}
	if !strings.Contains(m.hint, "2 items are still open beneath it") {
		t.Errorf("hint = %q, want it to say what is holding the heading open", m.hint)
	}
}

// A heading is an item, so the writes that act on the row under the cursor act on it as they do on
// any other row. Only the close guard tells the two apart (§7).
func TestLoopsWritesAddressAHeading(t *testing.T) {
	m, path := working(t)

	if item, ok := m.loopsTree(m.viewWidth()).SelectedItem(); !ok || item.Id != "vq2n" {
		t.Fatalf("the tab opens on %v, want the heading vq2n", item)
	}

	for _, c := range []struct {
		pressed string
		kind    promptKind
	}{{"w", promptWaiting}, {"e", promptRename}, {"n", promptNote}} {
		next, _ := press(t, m, c.pressed)
		if next.prompt.kind != c.kind {
			t.Errorf("%s on a heading opened prompt %d, want %d", c.pressed, next.prompt.kind, c.kind)
		}
		if next.prompt.subject != "vq2n" {
			t.Errorf("%s on a heading addressed %q, want the heading", c.pressed, next.prompt.subject)
		}
	}

	// The answer lands against the heading itself rather than against anything folded under it.
	noted, _ := press(t, m, "n", "the strings are extracted", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"note","id":"vq2n","text":"the strings are extracted"}`})
	if pane := detailPane(t, noted.View()); !strings.Contains(pane, "the strings are extracted") {
		t.Errorf("the detail pane does not carry the note written against the heading:\n%s", pane)
	}
}
