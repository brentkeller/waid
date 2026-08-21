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

// Several ids close in one invocation, in the order they were given.
func TestDoneClosesSeveralIds(t *testing.T) {
	home := makeHome(t)
	first := addItem(t, home, "Legend overflows")
	second := addItem(t, home, "Ticks collide")
	third := addItem(t, home, "Axis labels wrap")

	run := waid(t, home, "done", first, second, third)

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	for _, id := range []string{first, second, third} {
		if got := item(t, home, id).Status; got != "done" {
			t.Errorf("status of %s = %q, want done", id, got)
		}
	}
	closes := []string{}
	for _, line := range logLines(t, home) {
		if strings.Contains(line, `"ev":"close"`) {
			closes = append(closes, line)
		}
	}
	if len(closes) != 3 {
		t.Fatalf("wrote %d close lines, want 3", len(closes))
	}
	for position, id := range []string{first, second, third} {
		if !strings.Contains(closes[position], id) {
			t.Errorf("close %d = %q, want it to carry %s", position, closes[position], id)
		}
	}
}

// Every id is validated before anything is written, so a bad id late in the list cannot leave a
// half-finished close behind.
func TestDoneRejectsAnUnknownIdBeforeWritingAnything(t *testing.T) {
	home := makeHome(t)
	first := addItem(t, home, "Legend overflows")
	second := addItem(t, home, "Ticks collide")

	before := len(logLines(t, home))
	run := waid(t, home, "done", first, "zzzz", second)

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "zzzz") {
		t.Errorf("stderr = %q, want it to name the unknown id", run.err)
	}
	if got := len(logLines(t, home)); got != before {
		t.Errorf("the log grew to %d lines, want the refusal to write nothing", got)
	}
	if got := item(t, home, first).Status; got != "open" {
		t.Errorf("status of %s = %q, want open", first, got)
	}
}

// The close guard applies to each id in the list, and refuses the whole run.
func TestDoneRefusesWhenOneOfSeveralIdsHoldsAnOpenDescendant(t *testing.T) {
	home := makeHome(t)
	leaf := addItem(t, home, "Ticks collide")
	parent := addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows", "-p", "chart")

	before := len(logLines(t, home))
	run := waid(t, home, "done", leaf, parent)

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	for _, want := range []string{parent, child, "Legend overflows"} {
		if !strings.Contains(run.err, want) {
			t.Errorf("stderr = %q, want it to name %q", run.err, want)
		}
	}
	if got := len(logLines(t, home)); got != before {
		t.Errorf("the log grew to %d lines, want the refusal to write nothing", got)
	}
}

// Closing the children first is what the list is for, so a run naming both a parent and the
// descendants holding it open is allowed.
func TestDoneClosesAParentAlongsideTheDescendantsHoldingItOpen(t *testing.T) {
	home := makeHome(t)
	parent := addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows", "-p", "chart")
	grandchild := addItem(t, home, "Ticks collide", "-p", "legend")

	run := waid(t, home, "done", parent, child, grandchild)

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	for _, id := range []string{parent, child, grandchild} {
		if got := item(t, home, id).Status; got != "done" {
			t.Errorf("status of %s = %q, want done", id, got)
		}
	}
}

// --json is an array, one entry per closed item, whether one id was given or several.
func TestDoneJsonIsAnArray(t *testing.T) {
	home := makeHome(t)
	first := addItem(t, home, "Legend overflows")
	second := addItem(t, home, "Ticks collide")

	var closed []struct {
		Id     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
		Ts     string `json:"ts"`
	}
	run := waid(t, home, "done", first, second, "--json")
	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	run.decode(t, &closed)
	if len(closed) != 2 {
		t.Fatalf("closed = %d entries, want 2", len(closed))
	}
	if closed[0].Id != first || closed[0].Title != "Legend overflows" || closed[0].Status != "done" {
		t.Errorf("first entry = %+v, want %s / Legend overflows / done", closed[0], first)
	}
	if closed[1].Id != second {
		t.Errorf("second entry = %+v, want %s", closed[1], second)
	}
	if closed[0].Ts == "" {
		t.Error("entries carry no ts, want the close timestamp")
	}

	home = makeHome(t)
	only := addItem(t, home, "Legend overflows")
	run = waid(t, home, "done", only, "--json")
	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	closed = nil
	run.decode(t, &closed)
	if len(closed) != 1 || closed[0].Id != only {
		t.Errorf("one id yielded %+v, want a single-entry array for %s", closed, only)
	}
}

// The human output names every item it closed.
func TestDoneRendersALineForEachId(t *testing.T) {
	home := makeHome(t)
	first := addItem(t, home, "Legend overflows")
	second := addItem(t, home, "Ticks collide")

	run := waid(t, home, "done", first, second)

	want := "closed " + first + "  Legend overflows\nclosed " + second + "  Ticks collide\n"
	if run.out != want {
		t.Errorf("stdout = %q, want %q", run.out, want)
	}
}
