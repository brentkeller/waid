package harness_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/command"
	"github.com/brentkeller/waid/internal/harness"
)

// writeCommands are the commands that append to events.jsonl. They are judged by the log they leave
// behind rather than by what they print, since the log is the artifact that outlives the run.
var writeCommands = []string{
	"add", "done", "move", "heading", "tag", "reopen", "note", "dismiss", "promote", "undismiss",
}

// undispatchable are the commands this package cannot run. `ui` is a long-running terminal app, so
// there is no one-shot invocation to capture: with the piped stdio a spawned run gets, the only
// thing it can do is refuse to start, which smoke_test.go asserts instead.
var undispatchable = []string{"ui"}

// Every command in the dispatch table is run by this package as a real process somewhere. A command
// added without an invocation is the failure this guards against: unit tests exercise a command's
// own package, and only these runs put the whole binary over a populated data directory.
func TestEveryCommandIsExercised(t *testing.T) {
	covered := commandsIn(
		goldenReadInvocations(),
		detectionInvocations(ghFixture(t)),
		writeInvocations(),
		promoteInvocations(ghFixture(t)),
	)

	for name := range command.Registry {
		if slices.Contains(undispatchable, name) {
			continue
		}
		if !slices.Contains(covered, name) {
			t.Errorf("%s is in the dispatch table but is never run by the harness", name)
		}
	}
}

// Every read command is additionally run against a copy of the real data directory, where the
// inputs are the ones on this machine rather than the ones a fixture author thought of.
func TestEveryReadCommandIsExercisedOnTheRealHome(t *testing.T) {
	covered := commandsIn(realHomeInvocations(t))

	for name := range command.Registry {
		if slices.Contains(writeCommands, name) || slices.Contains(undispatchable, name) {
			continue
		}
		if !slices.Contains(covered, name) {
			t.Errorf("%s is never run against the real data directory", name)
		}
	}
}

// Every write command has the log it leaves behind checked, in fixture-home mode.
func TestEveryWriteCommandHasItsLogChecked(t *testing.T) {
	covered := commandsIn(writeInvocations(), promoteInvocations(ghFixture(t)))

	for _, name := range writeCommands {
		if _, known := command.Registry[name]; !known {
			t.Errorf("%s is exercised as a write command but is not in the dispatch table", name)
		}
		if !slices.Contains(covered, name) {
			t.Errorf("%s never has the log it leaves behind checked", name)
		}
	}
}

// commandsIn names the commands a set of invocations exercises, reading the command out of each
// argument list the way the CLI does: the first positional that is not a flag.
func commandsIn(sets ...map[string]harness.Invocation) []string {
	named := []string{}
	for _, set := range sets {
		for _, invocation := range set {
			for _, arg := range invocation.Args {
				if !strings.HasPrefix(arg, "-") {
					named = append(named, arg)
					break
				}
			}
		}
	}
	return named
}
