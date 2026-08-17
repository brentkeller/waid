package tui

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/detect"
)

// ghUnavailable is the note detection produces when gh cannot answer, which is the degradation the
// app actually meets (§7).
const ghUnavailable = "GitHub signals unavailable: gh: command not found"

// A degradation is a note under the list, not a failure: the list still shows what was detected, and
// the note is the CLI's own line, dimmed (§7).
func TestDegradationNotesRenderAsADimStrip(t *testing.T) {
	result := scanFixture()
	result.Notes = []string{ghUnavailable}

	m := scanned(t, 140, result)
	view := m.View()

	// The wording is the CLI's, indented the way `waid scan` indents it, and dim.
	strip := m.theme.Dim.Render("  " + ghUnavailable)
	if !strings.Contains(view, strip) {
		t.Fatalf("the note is missing or is not dimmed:\n%s", plain(view))
	}

	plainView := plain(view)
	note, lastRow := strings.Index(plainView, ghUnavailable), strings.Index(plainView, "3 uncommitted files")
	if lastRow < 0 {
		t.Fatalf("the signals are missing under a degraded pass:\n%s", plainView)
	}
	if note < lastRow {
		t.Errorf("the note strip is above the list, want it below:\n%s", plainView)
	}
}

// A note is not an error: nothing about the counts or the tab bar changes because gh was
// unavailable.
func TestDegradationNotesLeaveTheRestOfTheViewAlone(t *testing.T) {
	result := scanFixture()
	result.Notes = []string{ghUnavailable}

	m := scanned(t, 140, result)
	if m.counts[tabScan] != 4 {
		t.Errorf("the Scan tab carries the count %d under a note, want 4", m.counts[tabScan])
	}
	if got := m.barSlot(); got != "" {
		t.Errorf("the tab bar says %q under a note, want the slot empty", got)
	}
}

// A refresh that fails costs the refresh and nothing else: what was on screen stays there, and the
// tab bar carries the failure beside the age of what is being shown (§7).
func TestFailedRefreshKeepsTheLastGoodData(t *testing.T) {
	m := scanned(t, 140, scanFixture())

	refreshing, _ := press(t, m, "r")
	next, _ := refreshing.Update(scanLoadedMsg{err: errors.New("detection failed (gh exploded)")})
	failed := next.(Model)

	if failed.scan.loading {
		t.Error("the model is still loading after the refresh failed")
	}
	view := plain(failed.View())
	for _, want := range []string{"#6886", "3 uncommitted files", "4 signals"} {
		if !strings.Contains(view, want) {
			t.Errorf("the last good pass lost %q when the refresh failed:\n%s", want, view)
		}
	}
	if failed.counts[tabScan] != 4 {
		t.Errorf("the Scan tab carries the count %d after a failed refresh, want 4", failed.counts[tabScan])
	}

	bar := strings.Split(view, "\n")[1]
	for _, want := range []string{"detection failed", "2m ago"} {
		if !strings.Contains(bar, want) {
			t.Errorf("the tab bar does not carry %q after a failed refresh:\n%s", want, bar)
		}
	}
}

// The failure stands until a pass replaces it, and a pass that lands clears it.
func TestASuccessfulRefreshClearsTheFailure(t *testing.T) {
	m := scanned(t, 140, scanFixture())

	next, _ := m.Update(scanLoadedMsg{err: errors.New("detection failed (gh exploded)")})
	failed := next.(Model)
	if failed.scan.failure == "" {
		t.Fatal("the failed pass left no failure on the model")
	}

	next, _ = failed.Update(scanLoadedMsg{result: scanFixture(), at: scannedAt})
	recovered := next.(Model)
	if recovered.scan.failure != "" {
		t.Errorf("the failure %q survived a pass that landed", recovered.scan.failure)
	}
	if got := recovered.barSlot(); got != "" {
		t.Errorf("the tab bar still says %q after a pass landed", got)
	}
}

// Detection reaches git and the network, so it is the one read that can fail outright. It comes back
// as a failed pass rather than taking the app down with it.
func TestAPanickingPassComesBackAsAFailure(t *testing.T) {
	msg := guarded(func() scanLoadedMsg { panic("gh exploded") })()

	pass, ok := msg.(scanLoadedMsg)
	if !ok {
		t.Fatalf("a panicking pass produced %T, want a scanLoadedMsg", msg)
	}
	if pass.err == nil {
		t.Fatal("the panicking pass came back without an error")
	}
	for _, want := range []string{"detection failed", "gh exploded"} {
		if !strings.Contains(pass.err.Error(), want) {
			t.Errorf("the failure is %q, want it to carry %q", pass.err, want)
		}
	}
	if len(pass.result.Signals) != 0 {
		t.Errorf("the panicking pass returned %d signals, want none", len(pass.result.Signals))
	}
}

// A pass that returns normally is not touched by the guard.
func TestGuardedPassesThroughASuccessfulPass(t *testing.T) {
	want := scanLoadedMsg{result: scanFixture(), at: scannedAt}

	msg := guarded(func() scanLoadedMsg { return want })()
	pass, ok := msg.(scanLoadedMsg)
	if !ok {
		t.Fatalf("the guard produced %T, want a scanLoadedMsg", msg)
	}
	if pass.err != nil || len(pass.result.Signals) != len(want.result.Signals) {
		t.Errorf("the guard altered the pass: %d signals, err %v", len(pass.result.Signals), pass.err)
	}
}

// panicView is a model that fails where it hurts: mid-render, with the alt screen already entered.
type panicView struct{}

func (panicView) Init() tea.Cmd                       { return nil }
func (panicView) Update(tea.Msg) (tea.Model, tea.Cmd) { return panicView{}, nil }
func (panicView) View() string                        { panic("the render blew up") }

// The failure mode that actually strands a terminal is a panic with the alt screen up. Bubble Tea
// restores from a deferred call, and this is the assertion that it stays that way: the alt screen is
// left and the cursor is shown before a line of the stack reaches the screen (§7).
func TestPanicMidRenderRestoresTheTerminalBeforeTheStack(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("opening the pipe: %v", err)
	}

	// The panic banner and the stack are printed by Bubble Tea itself, to os.Stdout and os.Stderr, so
	// the program has to render to those same streams for the order of the two to be observable at
	// all. Capturing both also keeps the stack out of the test's own output.
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = write, write
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()

	captured := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(read)
		captured <- string(out)
	}()

	_, runErr := start(panicView{}, tea.WithOutput(write), tea.WithInput(strings.NewReader("")))
	write.Close()
	os.Stdout, os.Stderr = stdout, stderr
	stream := <-captured

	if !errors.Is(runErr, tea.ErrProgramPanic) {
		t.Fatalf("the program returned %v, want a recovered panic", runErr)
	}

	stack := strings.Index(stream, "Caught panic")
	if stack < 0 {
		t.Fatalf("the panic was never reported:\n%q", stream)
	}

	restores := []struct {
		what string
		seq  string
	}{
		{"the alt screen was left", "\x1b[?1049l"},
		{"the cursor was shown", "\x1b[?25h"},
	}
	for _, restore := range restores {
		at := strings.Index(stream, restore.seq)
		if at < 0 {
			t.Errorf("%s never happened:\n%q", restore.what, stream)
			continue
		}
		if at > stack {
			t.Errorf("%s only after the stack printed:\n%q", restore.what, stream)
		}
	}
}

// A failed first pass has no last good data to keep, and says so without an age it cannot know.
func TestAFailedFirstPassHasNoAge(t *testing.T) {
	m := offline(chrome(t, 140), detect.Result{})
	m, _ = press(t, m, "2")

	next, _ := m.Update(scanLoadedMsg{err: errors.New("detection failed (gh exploded)")})
	failed := next.(Model)

	slot := failed.barSlot()
	if !strings.Contains(slot, "detection failed") {
		t.Errorf("the tab bar says %q, want the failure", slot)
	}
	if strings.Contains(slot, "⏱") {
		t.Errorf("the tab bar says %q, want no age for data that never loaded", slot)
	}
}
