package harness_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/harness"
)

// writeInvocations are judged by the log they leave behind, which is the only artifact that
// outlives the run.
func writeInvocations() map[string]harness.Invocation {
	return map[string]harness.Invocation{
		"add": {
			Args: []string{"add", "Chart legend overflows", "-p", `C:\dev\waid`, "--tag", "bug", "--tag", "ui", "--session", "sess-1"},
			Ids:  []string{"9xk2"},
		},
		"add waiting on": {
			Args: []string{"add", "Approve the copy", "--waiting-on", "Dan"},
			Ids:  []string{"9xk2"},
		},
		"add resolving a partial project": {
			Args: []string{"add", "Second", "-p", "program files"},
			Ids:  []string{"9xk2"},
		},
		"add skipping a taken id": {
			Args: []string{"add", "Third"},
			Ids:  []string{"a1b2", "9xk2"},
		},
		"add with json": {
			Args: []string{"add", "Ship the thing", "--json"},
			Ids:  []string{"9xk2"},
		},
		"done":               {Args: []string{"done", "a1b2"}},
		"done with json":     {Args: []string{"done", "a1b2", "--json"}},
		"reopen":             {Args: []string{"reopen", "c3d4"}},
		"reopen with json":   {Args: []string{"reopen", "c3d4", "--json"}},
		"note":               {Args: []string{"note", "a1b2", "blocked on <the> API & the copy"}},
		"note with json":     {Args: []string{"note", "a1b2", "still going", "--json"}},
		"add without title":  {Args: []string{"add"}},
		"add blank title":    {Args: []string{"add", "   "}},
		"unknown project":    {Args: []string{"add", "Third", "-p", "nope"}},
		"done unknown id":    {Args: []string{"done", "zzzz"}},
		"done without an id": {Args: []string{"done"}},
		"note without text":  {Args: []string{"note", "a1b2", "   "}},
		"note unknown id":    {Args: []string{"note", "zzzz", "text"}},
		"reopen unknown id":  {Args: []string{"reopen", "zzzz", "--json"}},
		// dismiss takes a key on faith, so it needs no detection to reach the log.
		"dismiss":                    {Args: []string{"dismiss", "pr:octo/widgets#41"}},
		"dismiss with json":          {Args: []string{"dismiss", "pr:octo/widgets#41", "--json"}},
		"dismiss an already hidden":  {Args: []string{"dismiss", "review:DevResults/DevResults#6886"}},
		"dismiss a restored key":     {Args: []string{"dismiss", "pr:DevResults/DevResults#7237", "--json"}},
		"dismiss without a key":      {Args: []string{"dismiss"}},
		"dismiss with a blank key":   {Args: []string{"dismiss", "   ", "--json"}},
		"promote without a key":      {Args: []string{"promote"}},
		"promote with a blank key":   {Args: []string{"promote", "   ", "--json"}},
		"promote an undetected key":  {Args: []string{"promote", "pr:octo/widgets#41"}},
		"promote an undetected json": {Args: []string{"promote", "pr:octo/widgets#41", "--json"}},
		// The fixture log dismisses this key and never restores it.
		"undismiss":                  {Args: []string{"undismiss", "review:DevResults/DevResults#6886"}},
		"undismiss with json":        {Args: []string{"undismiss", "review:DevResults/DevResults#6886", "--json"}},
		"undismiss an unknown key":   {Args: []string{"undismiss", "pr:octo/widgets#41"}},
		"undismiss without a key":    {Args: []string{"undismiss"}},
		"undismiss with a blank key": {Args: []string{"undismiss", "   ", "--json"}},
	}
}

func TestWriteCommandsLeaveAReadableLog(t *testing.T) {
	for name, invocation := range writeInvocations() {
		t.Run(name, func(t *testing.T) {
			exerciseWrite(t, harness.GoldenHome(t), invocation)
		})
	}
}

// promoteInvocations run on the detection home, because the signal they name has to still be
// detectable before anything is written at all.
func promoteInvocations(fixture string) map[string]harness.Invocation {
	return map[string]harness.Invocation{
		"promote":           {Args: []string{"promote", "review:octo/widgets#41"}, Ids: []string{"9xk2"}, GhFixture: fixture},
		"promote with json": {Args: []string{"promote", "review:octo/widgets#41", "--json"}, Ids: []string{"9xk2"}, GhFixture: fixture},
		"promote a draft":   {Args: []string{"promote", "review:octo/gadgets#42", "--json"}, Ids: []string{"9xk2"}, GhFixture: fixture},
		"promote skipping a taken id": {
			Args:      []string{"promote", "review:octo/widgets#41", "--json"},
			Ids:       []string{"a1b2", "9xk2"},
			GhFixture: fixture,
		},
		"promote a dismissed key": {
			Args:      []string{"promote", "review:DevResults/DevResults#6886", "--json"},
			GhFixture: fixture,
		},
	}
}

func TestPromoteLeavesAReadableLog(t *testing.T) {
	for name, invocation := range promoteInvocations(ghFixture(t)) {
		t.Run(name, func(t *testing.T) {
			exerciseWrite(t, detectionHome(t), invocation)
		})
	}
}

// exerciseWrite is exercise with the two rules that govern the log itself: a run that reports a user
// error appends nothing, and every line a run does append folds cleanly — a malformed append would
// show up as a new problem the next time the log is read, long after the run that wrote it.
func exerciseWrite(t *testing.T, home string, inv harness.Invocation) {
	t.Helper()

	before := logState(t, home)
	out := exercise(t, home, inv)
	after := logState(t, home)

	if out.Code == cli.ExitUser && after.lines != before.lines {
		t.Errorf(
			"waid %s reported a user error but appended %d line(s)",
			strings.Join(inv.Args, " "), after.lines-before.lines,
		)
	}
	if after.lines < before.lines {
		t.Errorf("waid %s shortened the log; it is append-only", strings.Join(inv.Args, " "))
	}
	if after.problems != before.problems {
		t.Errorf(
			"waid %s left the log with %d problems, up from %d",
			strings.Join(inv.Args, " "), after.problems, before.problems,
		)
	}
}

type logSummary struct {
	lines    int
	problems int
}

func logState(t *testing.T, home string) logSummary {
	t.Helper()

	path := filepath.Join(home, "events.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return logSummary{}
		}
		t.Fatalf("reading %s: %v", path, err)
	}

	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	return logSummary{lines: len(lines), problems: len(events.Load(path).Problems)}
}
