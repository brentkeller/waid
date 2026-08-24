package command_test

import (
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
)

// headingRecord is the --json payload heading echoes back.
type headingRecord struct {
	Id      string `json:"id"`
	Title   string `json:"title"`
	Heading bool   `json:"heading"`
	Ts      string `json:"ts"`
}

func TestHeadingMarksAnItem(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite")

	run := waid(t, home, "heading", id, "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record headingRecord
	run.decode(t, &record)
	if record.Id != id || record.Title != "Chart rewrite" {
		t.Errorf("record = %+v, want the marked item", record)
	}
	if !record.Heading {
		t.Error("heading = false, want true")
	}
	if record.Ts == "" {
		t.Error("ts is empty")
	}
	if !item(t, home, id).Heading {
		t.Error("folded heading = false, want the item marked")
	}
}

// The write is an update line carrying the flag, so the fold reads it like any other update.
func TestHeadingAppendsTheExactLine(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-24T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd")
	id := addItem(t, home, "Chart rewrite")

	waid(t, home, "heading", id)

	want := `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"abcd","heading":true}`
	if got := logLines(t, home); len(got) != 2 || got[1] != want {
		t.Fatalf("log = %q,\nwant the second line %q", got, want)
	}
}

func TestHeadingOffUnmarksAnItem(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-24T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd")
	id := addItem(t, home, "Chart rewrite")
	waid(t, home, "heading", id)

	run := waid(t, home, "heading", id, "--off", "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record headingRecord
	run.decode(t, &record)
	if record.Heading {
		t.Error("heading = true, want false")
	}
	want := `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"abcd","heading":false}`
	if got := logLines(t, home); len(got) != 3 || got[2] != want {
		t.Fatalf("log = %q,\nwant the third line %q", got, want)
	}
	if item(t, home, id).Heading {
		t.Error("folded heading = true, want the item unmarked")
	}
}

// The log is a history of what was asked for, so a second mark writes a second line rather than
// deciding the request was redundant.
func TestHeadingWritesEvenWhenTheItemAlreadyHoldsTheValue(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite")
	waid(t, home, "heading", id)

	run := waid(t, home, "heading", id)

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got := len(logLines(t, home)); got != 3 {
		t.Errorf("log lines = %d, want the add and both marks", got)
	}
	if !item(t, home, id).Heading {
		t.Error("folded heading = false, want the item still marked")
	}
}

func TestHeadingOffWritesForAnItemThatWasNeverAHeading(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite")

	run := waid(t, home, "heading", id, "--off")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got := len(logLines(t, home)); got != 2 {
		t.Errorf("log lines = %d, want the add and the unmark", got)
	}
	if item(t, home, id).Heading {
		t.Error("folded heading = true, want it unmarked")
	}
}

func TestHeadingRejectsAnUnknownId(t *testing.T) {
	home := makeHome(t)
	addItem(t, home, "Chart rewrite")

	run := waid(t, home, "heading", "zzzz")

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

func TestHeadingWithoutAnIdExitsOne(t *testing.T) {
	home := makeHome(t)

	run := waid(t, home, "heading")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "item id") {
		t.Errorf("stderr = %q, want it to name the missing id", run.err)
	}
}

func TestHeadingHumanOutputIsOneLine(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite")

	run := waid(t, home, "heading", id)

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got, want := run.out, "marked "+id+"  Chart rewrite  a heading\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestHeadingOffHumanOutputNamesTheUnmark(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite")

	run := waid(t, home, "heading", id, "--off")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got, want := run.out, "unmarked "+id+"  Chart rewrite\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
