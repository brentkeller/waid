package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/events"
)

// The inverse of a dismiss is an undismiss of the same key, appended to the log rather than erasing
// the dismiss, and the row comes back to the list it left (§3).
func TestUndoOfADismissAppendsAnUndismiss(t *testing.T) {
	m, path := triaging(t)

	m, _ = press(t, m, "d")
	m, cmd := press(t, m, "u")
	if cmd != nil {
		t.Error("u issued a command, want the write made on the keypress")
	}

	ts := stamped()
	want := []string{
		`{"ts":"` + ts + `","ev":"dismiss","key":"review:DevResults/DevResults#6886"}`,
		`{"ts":"` + ts + `","ev":"undismiss","key":"review:DevResults/DevResults#6886"}`,
	}
	assertLog(t, path, want)

	view := plain(m.View())
	if !strings.Contains(view, "Add SharedDashboards") {
		t.Errorf("the undismissed signal is still missing from the list:\n%s", view)
	}
	if !strings.Contains(view, "undismissed review:DevResults/DevResults#6886") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
	if !strings.Contains(view, "4 signals") || !strings.Contains(view, "1 dismissed") {
		t.Errorf("the header does not count the restored signal:\n%s", view)
	}
	if m.counts[tabScan] != 4 {
		t.Errorf("the Scan tab carries the count %d, want 4", m.counts[tabScan])
	}
	if len(m.undos) != 0 {
		t.Errorf("the undo stack holds %d entries after the undo, want it empty", len(m.undos))
	}
}

// A promotion is two writes, so its inverse is two: the new item is closed and the signal's key is
// restored, in that order (§3).
func TestUndoOfAPromoteClosesTheItemAndRestoresTheKey(t *testing.T) {
	m, path := triaging(t)

	m, _ = press(t, m, "down", "down", "p")
	m, _ = press(t, m, "u")

	ts := stamped()
	want := []string{
		`{"ts":"` + ts + `","ev":"add","id":"7k3m","title":"1 unpushed commit on tui in waid","status":"open",` +
			`"origin":"C:\\dev\\waid","session":null,"tags":["promoted"],"waitingOn":null}`,
		`{"ts":"` + ts + `","ev":"dismiss","key":"ahead:C:\\dev\\waid:tui"}`,
		`{"ts":"` + ts + `","ev":"close","id":"7k3m"}`,
		`{"ts":"` + ts + `","ev":"undismiss","key":"ahead:C:\\dev\\waid:tui"}`,
	}
	assertLog(t, path, want)

	state := events.Load(path)
	if len(state.Dismissed) != 0 {
		t.Errorf("the folded log still hides %v, want the key restored", state.Dismissed)
	}
	if len(state.Items) != 1 || state.Items[0].Status != events.StatusDone {
		t.Errorf("the folded log holds %v, want the promoted item closed", state.Items)
	}

	view := plain(m.View())
	if !strings.Contains(view, "1 commit ahead") {
		t.Errorf("the signal did not come back to the list:\n%s", view)
	}
	if !strings.Contains(view, "unpromoted 7k3m") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
}

// The inverse of closing an item is reopening it.
func TestUndoOfADoneAppendsAReopen(t *testing.T) {
	m, path := triaging(t)

	m = m.pushUndo(undoDone(events.Item{Id: "sga9", Title: "Design template", Status: events.StatusOpen}))
	m, _ = press(t, m, "u")

	assertLog(t, path, []string{`{"ts":"` + stamped() + `","ev":"reopen","id":"sga9"}`})
	if got := plain(m.View()); !strings.Contains(got, "reopened sga9  Design template") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", got)
	}
}

// The inverse of a waiting write restores the status and the waiting-on the item held before it. An
// item that was plain open needs only the reopen that clears both; one that was waiting on someone
// else needs the status and the name put back.
func TestUndoOfAWaitingRestoresThePriorStatusAndWaitingOn(t *testing.T) {
	ts := stamped()
	who := "Dan"

	cases := map[string]struct {
		prior events.Item
		want  []string
	}{
		"open": {
			prior: events.Item{Id: "sga9", Title: "Design template", Status: events.StatusOpen},
			want:  []string{`{"ts":"` + ts + `","ev":"reopen","id":"sga9"}`},
		},
		"waiting on someone else": {
			prior: events.Item{Id: "sga9", Title: "Design template", Status: events.StatusWaiting, WaitingOn: &who},
			want: []string{
				`{"ts":"` + ts + `","ev":"reopen","id":"sga9"}`,
				`{"ts":"` + ts + `","ev":"update","id":"sga9","status":"waiting","waitingOn":"Dan"}`,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m, path := triaging(t)

			m = m.pushUndo(undoWaiting(tc.prior))
			m, _ = press(t, m, "u")

			assertLog(t, path, tc.want)
			if got := plain(m.View()); !strings.Contains(got, "restored sga9  Design template") {
				t.Errorf("the footer carries no receipt for the undo:\n%s", got)
			}
		})
	}
}

// The stack is bounded, so a long triage pass cannot grow it without limit, and the entries it drops
// are the oldest ones.
func TestTheUndoStackIsBounded(t *testing.T) {
	m, path := triaging(t)

	for i := range undoLimit + 2 {
		m = m.pushUndo(undoDone(events.Item{Id: fmt.Sprintf("id%02d", i), Title: "an item", Status: events.StatusOpen}))
	}
	if len(m.undos) != undoLimit {
		t.Fatalf("the stack holds %d entries, want it capped at %d", len(m.undos), undoLimit)
	}

	for range undoLimit {
		m, _ = press(t, m, "u")
	}
	if len(m.undos) != 0 {
		t.Fatalf("the stack holds %d entries after undoing all of them, want it empty", len(m.undos))
	}

	log := written(t, path)
	if len(log) != undoLimit {
		t.Fatalf("the log holds %d lines, want %d:\n%s", len(log), undoLimit, strings.Join(log, "\n"))
	}
	if !strings.Contains(log[0], fmt.Sprintf(`"id":"id%02d"`, undoLimit+1)) {
		t.Errorf("the first undo wrote %s, want the newest entry", log[0])
	}
	for _, dropped := range []string{"id00", "id01"} {
		if strings.Contains(strings.Join(log, "\n"), dropped) {
			t.Errorf("%s was undone, want the oldest entries dropped from the stack", dropped)
		}
	}
}

// An empty stack says so rather than doing nothing silently (§4).
func TestUndoOnAnEmptyStackSaysSo(t *testing.T) {
	m, path := triaging(t)

	m, _ = press(t, m, "u")
	if !strings.Contains(m.hint, "undo") {
		t.Errorf("u on an empty stack left the hint %q, want it to say there is nothing to undo", m.hint)
	}
	if log := written(t, path); len(log) != 0 {
		t.Errorf("u on an empty stack wrote %v", log)
	}
	if len(m.receipts) != 0 {
		t.Error("u on an empty stack left a receipt")
	}
}

// Promote is the one action carrying text, so the receipt offers `e` beside `u` and the title is
// corrected in one key (§3).
func TestPromoteReceiptOffersRenameAndEditsTheTitle(t *testing.T) {
	m, path := triaging(t)

	m, _ = press(t, m, "down", "down", "p")
	view := plain(m.View())
	for _, want := range []string{"u undo", "e rename"} {
		if !strings.Contains(view, want) {
			t.Errorf("the promote receipt does not offer %q:\n%s", want, view)
		}
	}

	m, _ = press(t, m, "e")
	if !strings.Contains(plain(m.View()), "rename 7k3m") {
		t.Errorf("e did not open the rename prompt:\n%s", plain(m.View()))
	}

	m, _ = press(t, m, "ctrl+u", "Push the tui branch", "q")
	if m.prompt.text() != "Push the tui branchq" {
		t.Errorf("q typed into the prompt left %q, want it treated as text", m.prompt.text())
	}

	m, _ = press(t, m, "backspace", "enter")
	assertLog(t, path, []string{
		`{"ts":"` + stamped() + `","ev":"add","id":"7k3m","title":"1 unpushed commit on tui in waid","status":"open",` +
			`"origin":"C:\\dev\\waid","session":null,"tags":["promoted"],"waitingOn":null}`,
		`{"ts":"` + stamped() + `","ev":"dismiss","key":"ahead:C:\\dev\\waid:tui"}`,
		`{"ts":"` + stamped() + `","ev":"update","id":"7k3m","title":"Push the tui branch"}`,
	})

	if got := plain(m.View()); !strings.Contains(got, "retitled 7k3m  Push the tui branch") {
		t.Errorf("the footer carries no receipt for the rename:\n%s", got)
	}

	// The rename is a write of its own, so it is what the next undo reverses.
	m, _ = press(t, m, "u")
	log := written(t, path)
	if want := `{"ts":"` + stamped() + `","ev":"update","id":"7k3m","title":"1 unpushed commit on tui in waid"}`; log[len(log)-1] != want {
		t.Errorf("the undo appended\n%s\nwant\n%s", log[len(log)-1], want)
	}
}

// esc abandons the prompt without writing anything.
func TestEscAbandonsTheRenamePrompt(t *testing.T) {
	m, path := triaging(t)

	m, _ = press(t, m, "down", "down", "p", "e", "Nope", "esc")
	if m.prompt.kind != promptNone {
		t.Error("esc left the prompt open")
	}
	if log := written(t, path); len(log) != 2 {
		t.Errorf("the abandoned rename wrote %v", log)
	}
}

// There is nothing to rename until something has been promoted, and the footer says so rather than
// opening a prompt over nothing (§4).
func TestRenameIsInertWithoutAPromoteReceipt(t *testing.T) {
	m, _ := triaging(t)

	m, _ = press(t, m, "e")
	if m.prompt.kind != promptNone {
		t.Error("e opened a rename prompt with nothing promoted")
	}
	if !strings.Contains(m.hint, "promote") {
		t.Errorf("e with nothing promoted left the hint %q, want it to name what it renames", m.hint)
	}
}

// assertLog reads the log back and compares it line for line, which is what a write is ultimately
// judged on.
func assertLog(t *testing.T, path string, want []string) {
	t.Helper()

	log := written(t, path)
	if len(log) != len(want) {
		t.Fatalf("the log holds %d lines, want %d:\n%s", len(log), len(want), strings.Join(log, "\n"))
	}
	for i := range want {
		if log[i] != want[i] {
			t.Errorf("line %d is\n%s\nwant\n%s", i+1, log[i], want[i])
		}
	}
}
