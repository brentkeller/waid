package tui

import (
	"fmt"
	"io"
	"strings"
)

// receipt is one write, in the wording of the command that appends the same events: the verb, the id
// or key the write addressed, and the text that identifies it afterwards. Writes land as keys are
// pressed with no confirm step, so the receipt is the only thing that says what a press did (§3).
type receipt struct {
	verb    string
	subject string
	detail  string
}

// The two verbs the footer's rename affordance keys off, since what `e` offers depends on what the
// last write was (§3).
const (
	verbPromoted = "promoted"
	verbRetitled = "retitled"
)

// verbFiled is what a move between projects reads as, in both directions: filing an item under a
// project and filing it back under none are the same write with a different answer.
const verbFiled = "filed"

const (
	// receiptDetailMax is the column the trailing text is cut to, so a long title cannot push the
	// footer onto a second line and shift the list above it.
	receiptDetailMax = 48

	// receiptVerbColumn is the width the verb takes in the replay, which lines the ids up down the
	// page. A verb wider than the column takes the extra rather than widening it for every line.
	receiptVerbColumn = 8
)

// line is the receipt as the footer shows it: `promoted 7k3m  Activity Compendium prototype`.
func (r receipt) line() string {
	return strings.TrimRight(r.verb+" "+r.rest(), " ")
}

// rest is everything after the verb — the subject, and the detail in its own column when there is
// one, since a key-only write like a dismiss has nothing to say beyond the key.
func (r receipt) rest() string {
	if r.detail == "" {
		return r.subject
	}
	return r.subject + "  " + truncate(r.detail, receiptDetailMax)
}

// record appends a receipt to the log. The append is capped to the log's own length so it allocates
// a fresh array: Model is copied by value through the update loop, and two copies sharing a backing
// array would overwrite each other's last write.
func (m Model) record(r receipt) Model {
	m.receipts = append(m.receipts[:len(m.receipts):len(m.receipts)], r)
	return m
}

// lastReceipt is the most recent write, which is what the footer carries until something fresher
// displaces it.
func (m Model) lastReceipt() (receipt, bool) {
	if len(m.receipts) == 0 {
		return receipt{}, false
	}
	return m.receipts[len(m.receipts)-1], true
}

// replay is the summary the app prints on quit: the same receipt lines, in the order they were
// written. The alternate screen takes the session's own trace with it when it goes, so without this
// a triage pass leaves nothing in scrollback (§3.1).
func replay(receipts []receipt) string {
	lines := make([]string, 0, len(receipts))
	for _, r := range receipts {
		line := fmt.Sprintf("%-*s %s", receiptVerbColumn, r.verb, r.rest())
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return strings.Join(lines, "\n")
}

// replayTo writes the summary to the restored terminal. Nothing is printed when nothing was written,
// so reading the week stays silent (§3.1).
func replayTo(out io.Writer, receipts []receipt) {
	if text := replay(receipts); text != "" {
		fmt.Fprintln(out, text)
	}
}
