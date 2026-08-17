package harness_test

import (
	"testing"

	"github.com/brentkeller/waid/internal/harness"
)

// The four write commands are diffed against Node by the log they leave behind, which is the only
// artifact that outlives the run.
func TestWriteCommandsMatchNode(t *testing.T) {
	invocations := map[string]harness.Invocation{
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
	}

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			harness.CompareWrite(t, harness.GoldenHome, invocation)
		})
	}
}
