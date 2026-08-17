package harness_test

import (
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/harness"
)

// The lines Node's usage carries that the Go usage drops, each with the reason it went.
var retiredUsageLines = map[string]string{
	`  waid loops [-p <project>] [-i]      Declared open/waiting items, then detected signals`: "-i is superseded by the app's Scan tab",
	`  waid scan [-p <project>] [-i]       Detected signals only`:                              "-i is superseded by the app's Scan tab",
	`  -i, --interactive                   Mark rows and apply in one keystroke (loops, scan)`: "-i is superseded by the app's Scan tab",
}

// The lines the Go usage carries that Node's does not, each with the reason it is there.
var addedUsageLines = map[string]string{
	`  waid loops [-p <project>]           Declared open/waiting items, then detected signals`: "the same row with -i removed",
	`  waid scan [-p <project>]            Detected signals only`:                              "the same row with -i removed",
	`  waid transcript <id>                A session's turns, oldest first`:                    "a command the port adds",
	`  waid undismiss <key>                Restore a dismissed signal`:                         "a command the port adds",
	`  --version                           Print the version`:                                  "Node parses --version but never answers it",
}

// The help text is the one place the two builds are expected to disagree, so it is diffed line by
// line against a recorded list rather than compared whole: anything not on the list is drift.
func TestHelpMatchesNodeApartFromRecordedDifferences(t *testing.T) {
	invocation := harness.Invocation{Args: []string{"--help"}}

	node := harness.RunNode(t, harness.GoldenHome(t), invocation)
	built := harness.RunGo(t, harness.GoldenHome(t), invocation)

	if node.Code != built.Code {
		t.Errorf("exit code: node %d, go %d", node.Code, built.Code)
	}
	if node.Stderr != built.Stderr {
		t.Errorf("stderr: node %q, go %q", node.Stderr, built.Stderr)
	}

	nodeLines := strings.Split(node.Stdout, "\n")
	goLines := strings.Split(built.Stdout, "\n")
	assertRecorded(t, "node only", missingFrom(nodeLines, goLines), retiredUsageLines)
	assertRecorded(t, "go only", missingFrom(goLines, nodeLines), addedUsageLines)
}

// assertRecorded checks that the observed differences are exactly the recorded ones — an unrecorded
// difference is a failure, and a recorded one that no longer occurs is a stale entry to delete.
func assertRecorded(t *testing.T, label string, observed []string, recorded map[string]string) {
	t.Helper()

	seen := map[string]bool{}
	for _, line := range observed {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if _, justified := recorded[line]; !justified {
			t.Errorf("unrecorded %s difference:\n  %q", label, line)
			continue
		}
		seen[line] = true
	}
	for line := range recorded {
		if !seen[line] {
			t.Errorf("recorded %s difference no longer occurs:\n  %q", label, line)
		}
	}
}

// missingFrom returns the lines of one stream that the other does not carry, counting duplicates, so
// a line that merely moved between sections is reported by neither side.
func missingFrom(lines, other []string) []string {
	counts := map[string]int{}
	for _, line := range other {
		counts[line]++
	}

	missing := []string{}
	for _, line := range lines {
		if counts[line] > 0 {
			counts[line]--
			continue
		}
		missing = append(missing, line)
	}
	return missing
}
