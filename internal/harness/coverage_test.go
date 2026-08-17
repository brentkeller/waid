package harness_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/command"
	"github.com/brentkeller/waid/internal/harness"
)

// goOnlyCommands are the commands the port adds, which Node has no counterpart for and so cannot be
// diffed against it. They are covered by internal/command's own tests, and by the usage diff, which
// records them as expected Go-only lines.
var goOnlyCommands = []string{"transcript", "undismiss"}

// writeCommands are the commands that append to events.jsonl. They are diffed by the log they leave
// behind rather than by what they print, since the log is the artifact that outlives the run.
var writeCommands = []string{"add", "done", "reopen", "note", "dismiss", "promote"}

// Every command in the dispatch table is diffed against Node somewhere in this package. A command
// added without a comparison is the failure this guards against: the port is only verified where the
// two builds are actually run side by side.
func TestEveryCommandIsDiffedAgainstNode(t *testing.T) {
	covered := commandsIn(
		goldenReadInvocations(),
		detectionInvocations(ghFixture(t)),
		writeInvocations(),
		promoteInvocations(ghFixture(t)),
	)

	for name := range command.Registry {
		if slices.Contains(goOnlyCommands, name) {
			continue
		}
		if !slices.Contains(covered, name) {
			t.Errorf("%s is in the dispatch table but is never diffed against Node", name)
		}
	}
}

// Every read command is additionally diffed against a copy of the real data directory, where the
// inputs are the ones on this machine rather than the ones a fixture author thought of.
func TestEveryReadCommandIsDiffedOnTheRealHome(t *testing.T) {
	covered := commandsIn(realHomeInvocations(t))

	for name := range command.Registry {
		if slices.Contains(goOnlyCommands, name) || slices.Contains(writeCommands, name) {
			continue
		}
		if !slices.Contains(covered, name) {
			t.Errorf("%s is never diffed against the real data directory", name)
		}
	}
}

// Every write command is diffed by the log it leaves behind, in fixture-home mode.
func TestEveryWriteCommandIsDiffedByItsLog(t *testing.T) {
	covered := commandsIn(writeInvocations(), promoteInvocations(ghFixture(t)))

	for _, name := range writeCommands {
		if _, known := command.Registry[name]; !known {
			t.Errorf("%s is diffed as a write command but is not in the dispatch table", name)
		}
		if !slices.Contains(covered, name) {
			t.Errorf("%s never has its log diffed against Node", name)
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
