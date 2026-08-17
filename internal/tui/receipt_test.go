package tui

import (
	"bytes"
	"strings"
	"testing"
)

// The three writes §3.1 documents, in the order it prints them.
func documented() []receipt {
	return []receipt{
		{verb: "promoted", subject: "7k3m", detail: "Activity Compendium prototype"},
		{verb: "dismissed", subject: `ahead:C:\dev\waid:tui`},
		{verb: "closed", subject: "sga9", detail: "Design template + args persistence…"},
	}
}

// Every write leaves a receipt, and the log keeps them in the order they were written rather than
// collapsing to the last one (§3.1).
func TestReceiptsAreLoggedInTheOrderWritten(t *testing.T) {
	m := New(Options{})
	for _, r := range documented() {
		m = m.record(r)
	}

	want := []string{
		"promoted 7k3m  Activity Compendium prototype",
		`dismissed ahead:C:\dev\waid:tui`,
		"closed sga9  Design template + args persistence…",
	}
	if len(m.receipts) != len(want) {
		t.Fatalf("logged %d receipts, want %d", len(m.receipts), len(want))
	}
	for i, line := range want {
		if got := m.receipts[i].line(); got != line {
			t.Errorf("receipt %d is %q, want %q", i, got, line)
		}
	}
}

// Model is copied by value through the update loop, so two models grown from the same log must not
// share the array the last write lands in.
func TestRecordDoesNotShareItsLogBetweenModels(t *testing.T) {
	m := New(Options{}).record(receipt{verb: "closed", subject: "sga9"})

	first := m.record(receipt{verb: "dismissed", subject: "dirty:alpha"})
	second := m.record(receipt{verb: "noted", subject: "7k3m", detail: "blocked on review"})

	if got := first.receipts[1].line(); got != "dismissed dirty:alpha" {
		t.Errorf("the first branch's last receipt is %q, want the dismiss it recorded", got)
	}
	if got := second.receipts[1].line(); got != "noted 7k3m  blocked on review" {
		t.Errorf("the second branch's last receipt is %q, want the note it recorded", got)
	}
}

// The footer carries the most recent receipt, so a write that removes a row from the list still says
// what it did (§3).
func TestTheFooterShowsTheMostRecentReceipt(t *testing.T) {
	m := chrome(t, 100)
	if got := plain(m.View()); strings.Contains(got, "dismissed") {
		t.Fatalf("a receipt is showing before anything was written:\n%s", got)
	}

	m = m.record(receipt{verb: "dismissed", subject: "dirty:alpha"})
	m = m.record(receipt{verb: "promoted", subject: "7k3m", detail: "Activity Compendium prototype"})

	view := plain(m.View())
	if !strings.Contains(view, "promoted 7k3m  Activity Compendium prototype") {
		t.Errorf("the latest receipt is not in the footer:\n%s", view)
	}
	if strings.Contains(view, "dismissed dirty:alpha") {
		t.Errorf("the footer shows an earlier receipt as well as the latest:\n%s", view)
	}

	// A key that did nothing is the fresher message and takes the line back.
	inert, _ := press(t, m, "z")
	if got := plain(inert.View()); !strings.Contains(got, inert.hint) || strings.Contains(got, "promoted 7k3m") {
		t.Errorf("an inert key did not replace the receipt in the footer:\n%s", got)
	}
}

// The replay is the block printed to the restored terminal, matching the lines §3.1 documents.
func TestReplayMatchesTheDocumentedLines(t *testing.T) {
	want := strings.Join([]string{
		"promoted 7k3m  Activity Compendium prototype",
		`dismissed ahead:C:\dev\waid:tui`,
		"closed   sga9  Design template + args persistence…",
	}, "\n")

	if got := replay(documented()); got != want {
		t.Errorf("replay =\n%s\nwant\n%s", got, want)
	}
}

// A title long enough to wrap the footer is cut to the receipt's column, the way the picker's lines
// were.
func TestALongDetailIsTruncated(t *testing.T) {
	long := strings.Repeat("a", receiptDetailMax+20)
	line := receipt{verb: "promoted", subject: "7k3m", detail: long}.line()

	if !strings.HasSuffix(line, "…") {
		t.Errorf("receipt line %q does not end in an ellipsis, want the detail cut", line)
	}
	if got := len([]rune(line)); got != len("promoted 7k3m  ")+receiptDetailMax {
		t.Errorf("receipt line is %d columns, want the detail cut to %d", got, receiptDetailMax)
	}
}

// Reading the week writes nothing, so quitting out of it prints nothing at all (§3.1).
func TestNothingIsReplayedWhenNothingWasWritten(t *testing.T) {
	if got := replay(nil); got != "" {
		t.Errorf("replay of an empty log = %q, want nothing", got)
	}

	var out bytes.Buffer
	replayTo(&out, New(Options{}).receipts)
	if out.Len() != 0 {
		t.Errorf("quitting with nothing written printed %q, want nothing", out.String())
	}

	replayTo(&out, []receipt{{verb: "dismissed", subject: "dirty:alpha"}})
	if got, want := out.String(), "dismissed dirty:alpha\n"; got != want {
		t.Errorf("the replay printed %q, want %q", got, want)
	}
}
