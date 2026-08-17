package harness_test

import (
	"testing"

	"github.com/brentkeller/waid/internal/harness"
)

// The read commands are diffed against Node by what they print, in both the machine and the human
// shape, since the two are views of the same value and either one can drift alone.
func TestReadCommandsMatchNode(t *testing.T) {
	invocations := map[string]harness.Invocation{
		"list":                         {Args: []string{"list"}},
		"list json":                    {Args: []string{"list", "--json"}},
		"list all":                     {Args: []string{"list", "--all"}},
		"list all json":                {Args: []string{"list", "--all", "--json"}},
		"list by status":               {Args: []string{"list", "--status", "done", "--json"}},
		"list by tag":                  {Args: []string{"list", "--tag", "bug", "--json"}},
		"list by project":              {Args: []string{"list", "-p", "program files"}},
		"list by partial project json": {Args: []string{"list", "-p", "waid", "--json"}},
		"list unknown status":          {Args: []string{"list", "--status", "nope"}},
		"list unknown status json":     {Args: []string{"list", "--status", "nope", "--json"}},
		"list unknown project":         {Args: []string{"list", "-p", "nope", "--json"}},
		"show":                         {Args: []string{"show", "a1b2"}},
		"show json":                    {Args: []string{"show", "a1b2", "--json"}},
		"show a reopened item":         {Args: []string{"show", "e5f6"}},
		"show without notes json":      {Args: []string{"show", "c3d4", "--json"}},
		"show unknown id":              {Args: []string{"show", "zzzz"}},
		"show unknown id json":         {Args: []string{"show", "zzzz", "--json"}},
		"show without an id":           {Args: []string{"show"}},
	}

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, harness.GoldenHome, invocation)
		})
	}
}

// The same commands against a copy of the real data directory, where the notes carry characters no
// synthetic fixture thought to include.
func TestReadCommandsMatchNodeOnTheRealHome(t *testing.T) {
	invocations := map[string]harness.Invocation{
		"list":      {Args: []string{"list"}},
		"list json": {Args: []string{"list", "--json"}},
		"list all":  {Args: []string{"list", "--all"}},
	}

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, harness.RealHome, invocation)
		})
	}
}
