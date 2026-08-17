package command_test

import (
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/gh"
)

// undismissRecord is the --json payload undismiss prints.
type undismissRecord struct {
	Key         string `json:"key"`
	Undismissed bool   `json:"undismissed"`
}

// undismissJson runs undismiss --json and decodes what it printed.
func undismissJson(t *testing.T, home string, seams cli.Seams, key string) (result, undismissRecord) {
	t.Helper()

	run := waidWith(t, home, seams, "undismiss", key, "--no-sync", "--json")
	var record undismissRecord
	run.decode(t, &record)
	return run, record
}

func TestUndismissAppendsOneEventAndTheKeyReappearsInScan(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	home := detectHome(t, root)
	seams := cli.Seams{
		Git: fakeGit{repo: {branch: ptr("main"), dirty: 4}},
		Gh:  fakeGh{review: []gh.Pr{pr("acme/web", 123, nil)}},
	}

	dismissJson(t, home, seams, "review:acme/web#123")
	_, hidden := scanJson(t, home, seams)
	assertKeys(t, signalKeys(hidden.Signals), []string{"dirty:" + repo})

	run, record := undismissJson(t, home, seams, "review:acme/web#123")
	if run.code != cli.ExitOK {
		t.Fatalf("undismiss exited %d: %s", run.code, run.err)
	}
	if record.Key != "review:acme/web#123" || !record.Undismissed {
		t.Errorf("undismiss reported %+v", record)
	}

	log := logEvents(t, home)
	if len(log) != 2 {
		t.Fatalf("the log holds %d events, want 2", len(log))
	}
	if log[1]["ev"] != "undismiss" || log[1]["key"] != "review:acme/web#123" {
		t.Errorf("appended %v", log[1])
	}

	_, restored := scanJson(t, home, seams)
	assertKeys(t, signalKeys(restored.Signals), []string{"review:acme/web#123", "dirty:" + repo})
	if restored.DismissedCount != 0 {
		t.Errorf("dismissed count is %d, want 0", restored.DismissedCount)
	}
}

func TestUndismissRendersTheKeyItRestored(t *testing.T) {
	home := detectHome(t, t.TempDir())
	seams := cli.Seams{Git: fakeGit{}, Gh: fakeGh{}}

	dismissJson(t, home, seams, "pr:acme/web#9")
	run := waidWith(t, home, seams, "undismiss", "pr:acme/web#9", "--no-sync")

	if run.code != cli.ExitOK {
		t.Fatalf("undismiss exited %d: %s", run.code, run.err)
	}
	if run.out != "undismissed pr:acme/web#9\n" {
		t.Errorf("stdout = %q", run.out)
	}
}

func TestUndismissingAKeyThatWasNeverDismissedExitsOneAndWritesNothing(t *testing.T) {
	home := detectHome(t, t.TempDir())
	seams := cli.Seams{Git: fakeGit{}, Gh: fakeGh{}}

	run := waidWith(t, home, seams, "undismiss", "pr:acme/web#404", "--no-sync")

	if run.code != cli.ExitUser {
		t.Fatalf("undismiss exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `unknown dismissed key: pr:acme/web#404`)
	if log := logEvents(t, home); len(log) != 0 {
		t.Errorf("the log holds %d events, want none", len(log))
	}
}

func TestUndismissingTwiceExitsOneOnTheSecondCall(t *testing.T) {
	home := detectHome(t, t.TempDir())
	seams := cli.Seams{Git: fakeGit{}, Gh: fakeGh{}}

	dismissJson(t, home, seams, "pr:acme/web#9")
	if first, _ := undismissJson(t, home, seams, "pr:acme/web#9"); first.code != cli.ExitOK {
		t.Fatalf("undismiss exited %d: %s", first.code, first.err)
	}

	second := waidWith(t, home, seams, "undismiss", "pr:acme/web#9", "--no-sync")
	if second.code != cli.ExitUser {
		t.Fatalf("the second undismiss exited %d, want %d", second.code, cli.ExitUser)
	}
	if log := logEvents(t, home); len(log) != 2 {
		t.Errorf("the log holds %d events, want 2", len(log))
	}
}

func TestUndismissWithNoKeyExitsOne(t *testing.T) {
	home := detectHome(t, t.TempDir())

	run := waidWith(t, home, cli.Seams{}, "undismiss", "--no-sync")
	if run.code != cli.ExitUser {
		t.Fatalf("undismiss exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `undismiss requires a signal key`)
}
