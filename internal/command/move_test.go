package command_test

import (
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
)

// moveRecord is the --json payload move echoes back.
type moveRecord struct {
	Id     string  `json:"id"`
	Title  string  `json:"title"`
	Parent *string `json:"parent"`
	Ts     string  `json:"ts"`
}

func TestMoveFilesAnItemUnderAParentNamedByAFragment(t *testing.T) {
	home := makeHome(t)
	parent := addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows")

	run := waid(t, home, "move", child, "-p", "chart", "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record moveRecord
	run.decode(t, &record)
	if record.Id != child || record.Title != "Legend overflows" {
		t.Errorf("record = %+v, want the moved item", record)
	}
	if record.Parent == nil || *record.Parent != parent {
		t.Errorf("parent = %v, want %s", record.Parent, parent)
	}
	if record.Ts == "" {
		t.Error("ts is empty")
	}
	if got := item(t, home, child).Parent; got == nil || *got != parent {
		t.Errorf("folded parent = %v, want %s", got, parent)
	}
}

// The write is an update line carrying the parent, so the fold reads it like any other update.
func TestMoveAppendsTheExactLine(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd,efgh")
	addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows")

	waid(t, home, "move", child, "-p", "chart")

	want := `{"ts":"2026-08-17T12:00:00.000Z","ev":"update","id":"efgh","parent":"abcd"}`
	if got := logLines(t, home); len(got) != 3 || got[2] != want {
		t.Fatalf("log = %q,\nwant the third line %q", got, want)
	}
}

// --top writes the parent as an explicit null, which is how the fold hears "top level".
func TestMoveToTopLevelWritesANullParent(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd,efgh")
	addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows", "-p", "chart")

	run := waid(t, home, "move", child, "--top", "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record moveRecord
	run.decode(t, &record)
	if record.Parent != nil {
		t.Errorf("parent = %v, want null", *record.Parent)
	}
	want := `{"ts":"2026-08-17T12:00:00.000Z","ev":"update","id":"efgh","parent":null}`
	if got := logLines(t, home); len(got) != 3 || got[2] != want {
		t.Fatalf("log = %q,\nwant the third line %q", got, want)
	}
	if got := item(t, home, child).Parent; got != nil {
		t.Errorf("folded parent = %v, want null", *got)
	}
}

func TestMoveRejectsAnItemOntoItself(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite")

	run := waid(t, home, "move", id, "-p", "chart")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "itself") {
		t.Errorf("stderr = %q, want it to say an item cannot sit under itself", run.err)
	}
	if got := len(logLines(t, home)); got != 1 {
		t.Errorf("log lines = %d, want only the add", got)
	}
}

func TestMoveRejectsAnItemOntoItsOwnDescendant(t *testing.T) {
	home := makeHome(t)
	root := addItem(t, home, "Chart rewrite")
	middle := addItem(t, home, "Legend work", "-p", "chart rewrite")
	addItem(t, home, "Overflow at narrow widths", "-p", "legend work")

	run := waid(t, home, "move", root, "-p", "overflow")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "descendant") {
		t.Errorf("stderr = %q, want it to name the descendant rule", run.err)
	}
	if got := len(logLines(t, home)); got != 3 {
		t.Errorf("log lines = %d, want only the three adds", got)
	}
	if got := item(t, home, middle).Parent; got == nil || *got != root {
		t.Errorf("folded parent = %v, want the tree untouched", got)
	}
}

// A path is provenance, not a place, so move refuses it rather than quietly doing nothing.
func TestMoveRejectsAnAbsolutePath(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Legend overflows")

	run := waid(t, home, "move", id, "-p", `C:\dev\waid`)

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, `C:\dev\waid`) {
		t.Errorf("stderr = %q, want it to name the path", run.err)
	}
	if got := len(logLines(t, home)); got != 1 {
		t.Errorf("log lines = %d, want only the add", got)
	}
}

func TestMoveRejectsADotAsADestination(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Legend overflows")

	run := waid(t, home, "move", id, "-p", ".")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if got := len(logLines(t, home)); got != 1 {
		t.Errorf("log lines = %d, want only the add", got)
	}
}

func TestMoveWithoutADestinationExitsOne(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Legend overflows")

	run := waid(t, home, "move", id)

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "--top") {
		t.Errorf("stderr = %q, want it to name the two destinations", run.err)
	}
	if got := len(logLines(t, home)); got != 1 {
		t.Errorf("log lines = %d, want only the add", got)
	}
}

func TestMoveWithoutAnIdExitsOne(t *testing.T) {
	home := makeHome(t)

	run := waid(t, home, "move")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "item id") {
		t.Errorf("stderr = %q, want it to name the missing id", run.err)
	}
}

func TestMoveRejectsAnUnknownId(t *testing.T) {
	home := makeHome(t)
	addItem(t, home, "Chart rewrite")

	run := waid(t, home, "move", "zzzz", "-p", "chart")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "zzzz") {
		t.Errorf("stderr = %q, want it to name the unknown id", run.err)
	}
	if got := len(logLines(t, home)); got != 1 {
		t.Errorf("log lines = %d, want only the add", got)
	}
}

func TestMoveRejectsAnAmbiguousFragmentAndListsTheMatches(t *testing.T) {
	home := makeHome(t)
	first := addItem(t, home, "Chart rewrite")
	second := addItem(t, home, "Chart legend")
	id := addItem(t, home, "Legend overflows")

	run := waid(t, home, "move", id, "-p", "chart")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	for _, candidate := range []string{first, "Chart rewrite", second, "Chart legend"} {
		if !strings.Contains(run.err, candidate) {
			t.Errorf("stderr = %q, want it to list %s", run.err, candidate)
		}
	}
	if got := len(logLines(t, home)); got != 3 {
		t.Errorf("log lines = %d, want only the three adds", got)
	}
}

func TestMoveHumanOutputIsOneLine(t *testing.T) {
	home := makeHome(t)
	addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows")

	run := waid(t, home, "move", child, "-p", "chart")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got, want := run.out, "moved "+child+"  Legend overflows  under Chart rewrite\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestMoveToTopLevelHumanOutputNamesTheTopLevel(t *testing.T) {
	home := makeHome(t)
	addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows", "-p", "chart")

	run := waid(t, home, "move", child, "--top")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got, want := run.out, "moved "+child+"  Legend overflows  to the top level\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// --parent takes an id literally, so a parent whose title other items share is still nameable.
func TestMoveFilesAnItemUnderAParentNamedById(t *testing.T) {
	home := makeHome(t)
	parent := addItem(t, home, "Chart rewrite")
	addItem(t, home, "Chart legend")
	child := addItem(t, home, "Legend overflows")

	run := waid(t, home, "move", child, "--parent", parent, "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record moveRecord
	run.decode(t, &record)
	if record.Parent == nil || *record.Parent != parent {
		t.Errorf("parent = %v, want %s", record.Parent, parent)
	}
	if got := item(t, home, child).Parent; got == nil || *got != parent {
		t.Errorf("folded parent = %v, want %s", got, parent)
	}
}

func TestMoveRejectsAnUnknownParentId(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Legend overflows")

	run := waid(t, home, "move", id, "--parent", "zzzz")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "zzzz") {
		t.Errorf("stderr = %q, want it to name the unknown id", run.err)
	}
	if got := len(logLines(t, home)); got != 1 {
		t.Errorf("log lines = %d, want only the add", got)
	}
}

func TestMoveRejectsAnItemOntoItselfById(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite")

	run := waid(t, home, "move", id, "--parent", id)

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "itself") {
		t.Errorf("stderr = %q, want it to say an item cannot sit under itself", run.err)
	}
}

func TestMoveRejectsMoreThanOneDestination(t *testing.T) {
	home := makeHome(t)
	parent := addItem(t, home, "Chart rewrite")
	child := addItem(t, home, "Legend overflows")

	for name, args := range map[string][]string{
		"fragment and id":  {"-p", "chart", "--parent", parent},
		"id and top":       {"--parent", parent, "--top"},
		"fragment and top": {"-p", "chart", "--top"},
	} {
		t.Run(name, func(t *testing.T) {
			run := waid(t, home, append([]string{"move", child}, args...)...)

			if run.code != cli.ExitUser {
				t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
			}
			if got := len(logLines(t, home)); got != 2 {
				t.Errorf("log lines = %d, want only the two adds", got)
			}
		})
	}
}
