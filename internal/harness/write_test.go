package harness_test

import (
	"testing"

	"github.com/brentkeller/waid/internal/harness"
)

// writeInvocations are diffed against Node by the log they leave behind, which is the only artifact
// that outlives the run.
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
	}
}

func TestWriteCommandsMatchNode(t *testing.T) {
	for name, invocation := range writeInvocations() {
		t.Run(name, func(t *testing.T) {
			harness.CompareWrite(t, harness.GoldenHome, invocation)
		})
	}
}

// promoteInvocations are diffed on the detection home, because the signal they name has to still be
// detectable before either build will write anything at all.
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

func TestPromoteMatchesNode(t *testing.T) {
	for name, invocation := range promoteInvocations(ghFixture(t)) {
		t.Run(name, func(t *testing.T) {
			harness.CompareWrite(t, detectionHome, invocation)
		})
	}
}
