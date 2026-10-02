package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/ids"
)

// triaging is a Scan tab over a temp home: the writes land in a log the test reads back, and the ids
// come from a fixed sequence so a receipt can be asserted verbatim.
func triaging(t *testing.T) (Model, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	opts := Options{Cfg: config.Config{EventsPath: path}, Now: scannedAt, Ids: ids.Sequence("7k3m", "9xz1")}

	sized, _ := New(opts).Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	m := offline(sized.(Model), scanFixture())
	m, _ = press(t, m, "2")

	loaded, _ := m.Update(m.scan.load())
	return loaded.(Model), path
}

// written reads the log back as the lines it holds, which is what a write is ultimately judged on.
func written(t *testing.T, path string) []string {
	t.Helper()

	return events.ReadLines(path)
}

// stamped is the timestamp every write in these tests carries, since the model's clock is pinned.
func stamped() string { return events.FormatTs(scanNow) }

// d appends one dismiss and the row leaves the list on the keypress, with no confirm step (§3).
func TestDismissWritesTheKeyAndDropsTheRow(t *testing.T) {
	m, path := triaging(t)

	m, cmd := press(t, m, "d")
	if cmd != nil {
		t.Error("d issued a command, want the write made on the keypress")
	}

	want := `{"ts":"` + stamped() + `","ev":"dismiss","key":"review:DevResults/DevResults#6886"}`
	log := written(t, path)
	if len(log) != 1 {
		t.Fatalf("the log holds %d lines, want 1:\n%s", len(log), strings.Join(log, "\n"))
	}
	if log[0] != want {
		t.Errorf("appended\n%s\nwant\n%s", log[0], want)
	}

	view := plain(m.View())
	if strings.Contains(view, "Add SharedDashboards") {
		t.Errorf("the dismissed signal is still listed:\n%s", view)
	}
	if !strings.Contains(view, "dismissed review:DevResults/DevResults#6886") {
		t.Errorf("the footer carries no receipt for the dismissal:\n%s", view)
	}
	if !strings.Contains(view, "3 signals") || !strings.Contains(view, "2 dismissed") {
		t.Errorf("the header does not count the dismissal:\n%s", view)
	}
	if m.counts[tabScan] != 3 {
		t.Errorf("the Scan tab carries the count %d, want 3", m.counts[tabScan])
	}
}

// p appends the add the signal becomes and the dismiss that stops it being reported twice, in that
// order, and the receipt names the new item (§3).
func TestPromoteWritesTheAddAndDismissPair(t *testing.T) {
	m, path := triaging(t)

	m, cmd := press(t, m, "down", "down", "p")
	if cmd != nil {
		t.Error("p issued a command, want the write made on the keypress")
	}

	ts := stamped()
	want := []string{
		`{"ts":"` + ts + `","ev":"add","id":"7k3m","title":"1 unpushed commit on tui in waid","status":"open",` +
			`"origin":"C:\\dev\\waid","session":null,"tags":["promoted"],"waitingOn":null}`,
		`{"ts":"` + ts + `","ev":"dismiss","key":"ahead:C:\\dev\\waid:tui"}`,
	}

	log := written(t, path)
	if len(log) != len(want) {
		t.Fatalf("the log holds %d lines, want %d:\n%s", len(log), len(want), strings.Join(log, "\n"))
	}
	for i := range want {
		if log[i] != want[i] {
			t.Errorf("line %d is\n%s\nwant\n%s", i+1, log[i], want[i])
		}
	}

	view := plain(m.View())
	if strings.Contains(view, "1 commit ahead") {
		t.Errorf("the promoted signal is still listed:\n%s", view)
	}
	if !strings.Contains(view, "promoted 7k3m  1 unpushed commit on tui in waid") {
		t.Errorf("the footer carries no receipt for the promotion:\n%s", view)
	}
}

// The two writes go through the same code the promote command does, so a signal promoted from either
// lands as the same pair of events.
func TestTriageLeavesTheKeyDismissedInTheFoldedLog(t *testing.T) {
	m, path := triaging(t)

	m, _ = press(t, m, "d", "p")

	state := events.Load(path)
	if len(state.Dismissed) != 2 {
		t.Fatalf("the folded log hides %v, want both triaged keys", state.Dismissed)
	}
	if len(state.Items) != 1 || state.Items[0].Id != "7k3m" {
		t.Fatalf("the folded log holds %d items, want the promoted one", len(state.Items))
	}
	if tags := state.Items[0].Tags; len(tags) != 1 || tags[0] != "promoted" {
		t.Errorf("the promoted item carries tags %v, want [promoted]", tags)
	}
}

// A project heading is not a signal, so neither action writes anything from one and the footer says
// why (§4).
func TestTriageIsInertOnAProjectHeading(t *testing.T) {
	m, path := triaging(t)

	// enter folds the project, which puts the cursor on the heading standing in for its children.
	folded, _ := press(t, m, "enter")

	for _, pressed := range []string{"p", "d"} {
		next, _ := press(t, folded, pressed)
		if log := written(t, path); len(log) != 0 {
			t.Errorf("%s on a project heading wrote %v", pressed, log)
		}
		if !strings.Contains(next.hint, "project") {
			t.Errorf("%s on a project heading left the hint %q, want it to name the row", pressed, next.hint)
		}
		if len(next.receipts) != 0 {
			t.Errorf("%s on a project heading left a receipt", pressed)
		}
	}
}

// A refresh re-reads the log detection already excludes the dismissed keys from, so the triaged set
// is not carried across a pass and counted twice.
func TestARefreshClearsTheLocallyTriagedKeys(t *testing.T) {
	m, _ := triaging(t)

	m, _ = press(t, m, "d")
	if len(m.scan.triaged) != 1 {
		t.Fatalf("the dismissed key is not held locally: %v", m.scan.triaged)
	}

	next, _ := m.Update(m.scan.load())
	if got := next.(Model).scan.triaged; len(got) != 0 {
		t.Errorf("the refresh kept the triaged keys %v, want them cleared", got)
	}
}
