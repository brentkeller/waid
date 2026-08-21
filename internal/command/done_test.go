package command_test

import (
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
)

// A parent cannot be closed while anything beneath it is open (§5). The refusal reaches the whole
// subtree, so a closed child with an open child of its own still holds the heading open.
func TestDoneRefusesAParentWithAnOpenGrandchild(t *testing.T) {
	home := makeHome(t)
	parent := addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows", "-p", "chart")
	waid(t, home, "done", child)
	grandchild := addItem(t, home, "Ticks collide", "-p", "legend")

	before := len(logLines(t, home))
	run := waid(t, home, "done", parent)

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, parent) {
		t.Errorf("stderr = %q, want it to name the item being closed", run.err)
	}
	for _, want := range []string{grandchild, "Ticks collide"} {
		if !strings.Contains(run.err, want) {
			t.Errorf("stderr = %q, want it to name the open descendant %q", run.err, want)
		}
	}
	if strings.Contains(run.err, child) {
		t.Errorf("stderr = %q, want the closed child left out", run.err)
	}
	if got := len(logLines(t, home)); got != before {
		t.Errorf("the log grew to %d lines, want the refusal to write nothing", got)
	}
}

// Once the subtree is finished the heading closes like any other item.
func TestDoneClosesAParentWhoseDescendantsAreAllDone(t *testing.T) {
	home := makeHome(t)
	parent := addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows", "-p", "chart")
	waid(t, home, "done", child)

	run := waid(t, home, "done", parent)

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got := item(t, home, parent).Status; got != "done" {
		t.Errorf("folded status = %q, want done", got)
	}
}

// --json callers get the refusal as a payload, with the open descendants as its candidates.
func TestDoneRefusalCarriesTheOpenDescendantsAsJson(t *testing.T) {
	home := makeHome(t)
	parent := addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows", "-p", "chart")

	run := waid(t, home, "done", parent, "--json")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	var payload struct {
		Error      string   `json:"error"`
		Candidates []string `json:"candidates"`
	}
	run.decode(t, &payload)
	if !strings.Contains(payload.Error, "cannot close "+parent) {
		t.Errorf("error = %q, want it to refuse the close", payload.Error)
	}
	if got, want := payload.Candidates, child+"  Legend overflows"; len(got) != 1 || got[0] != want {
		t.Errorf("candidates = %q, want [%q]", got, want)
	}
}
