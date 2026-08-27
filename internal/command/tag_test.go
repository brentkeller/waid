package command_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
)

// tagRecord is the --json payload tag echoes back.
type tagRecord struct {
	Id    string   `json:"id"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Ts    string   `json:"ts"`
}

func TestTagAddsToTheSetTheItemAlreadyCarries(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "ui")

	run := waid(t, home, "tag", id, "-t", "bug", "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record tagRecord
	run.decode(t, &record)
	if record.Id != id || record.Title != "Chart rewrite" {
		t.Errorf("record = %+v, want the tagged item", record)
	}
	if want := []string{"ui", "bug"}; !slices.Equal(record.Tags, want) {
		t.Errorf("tags = %q, want %q", record.Tags, want)
	}
	if record.Ts == "" {
		t.Error("ts is empty")
	}
	if got := item(t, home, id).Tags; !slices.Equal(got, []string{"ui", "bug"}) {
		t.Errorf("folded tags = %q, want the added tag beside the one it had", got)
	}
}

// Several -t flags land in one write, in the order they were given.
func TestTagAddsEveryTagInOneWrite(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite")

	run := waid(t, home, "tag", id, "-t", "bug", "-t", "ui")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got := len(logLines(t, home)); got != 2 {
		t.Errorf("log lines = %d, want the add and one tag write", got)
	}
	if got := item(t, home, id).Tags; !slices.Equal(got, []string{"bug", "ui"}) {
		t.Errorf("folded tags = %q, want both, in order", got)
	}
}

// The write is an update line carrying the whole resulting set, so the fold reads it like any other
// update and the log records an outcome rather than a delta.
func TestTagAppendsTheExactLine(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-24T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd")
	id := addItem(t, home, "Chart rewrite", "--tag", "ui")

	waid(t, home, "tag", id, "-t", "bug")

	want := `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"abcd","tags":["ui","bug"]}`
	if got := logLines(t, home); len(got) != 2 || got[1] != want {
		t.Fatalf("log = %q,\nwant the second line %q", got, want)
	}
}

func TestTagRemoveTakesOneOff(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "bug", "--tag", "ui")

	run := waid(t, home, "tag", id, "--remove", "ui", "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record tagRecord
	run.decode(t, &record)
	if want := []string{"bug"}; !slices.Equal(record.Tags, want) {
		t.Errorf("tags = %q, want %q", record.Tags, want)
	}
	if got := item(t, home, id).Tags; !slices.Equal(got, []string{"bug"}) {
		t.Errorf("folded tags = %q, want the tag gone", got)
	}
}

func TestTagRemoveIsRepeatable(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "bug", "--tag", "ui", "--tag", "perf")

	waid(t, home, "tag", id, "--remove", "ui", "--remove", "perf")

	if got := item(t, home, id).Tags; !slices.Equal(got, []string{"bug"}) {
		t.Errorf("folded tags = %q, want both removals applied", got)
	}
}

// Removals are applied before additions, so a tag named to both ends up present: a -t passed to add
// something should never leave it absent.
func TestTagAppliesRemovalsBeforeAdditions(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "bug", "--tag", "ui")

	waid(t, home, "tag", id, "--remove", "ui", "--remove", "bug", "-t", "bug")

	if got := item(t, home, id).Tags; !slices.Equal(got, []string{"bug"}) {
		t.Errorf("folded tags = %q, want the addition to win", got)
	}
}

func TestTagOffClearsTheSet(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-24T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd")
	id := addItem(t, home, "Chart rewrite", "--tag", "bug", "--tag", "ui")

	run := waid(t, home, "tag", id, "--off", "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record tagRecord
	run.decode(t, &record)
	if len(record.Tags) != 0 {
		t.Errorf("tags = %q, want none", record.Tags)
	}
	want := `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"abcd","tags":[]}`
	if got := logLines(t, home); len(got) != 2 || got[1] != want {
		t.Fatalf("log = %q,\nwant the second line %q", got, want)
	}
	if got := item(t, home, id).Tags; len(got) != 0 {
		t.Errorf("folded tags = %q, want the set cleared", got)
	}
}

// --off says what the set is; -t and --remove say how to change it. Asking for both at once is two
// different intentions, so it is refused rather than resolved by a rule nobody would remember.
func TestTagOffRefusesToCombineWithAnEdit(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "bug")

	for _, edit := range [][]string{{"-t", "ui"}, {"--remove", "bug"}} {
		run := waid(t, home, append([]string{"tag", id, "--off"}, edit...)...)

		if run.code != cli.ExitUser {
			t.Fatalf("%q: exit code = %d, want %d: %s", edit, run.code, cli.ExitUser, run.err)
		}
		if !strings.Contains(run.err, "--off") {
			t.Errorf("%q: stderr = %q, want it to name the conflict", edit, run.err)
		}
	}
	if got := len(logLines(t, home)); got != 1 {
		t.Errorf("log lines = %d, want only the add", got)
	}
}

// A bare tag command states no intention, so it is refused rather than silently wiping the set.
func TestTagWithoutAnEditExitsOne(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "bug")

	run := waid(t, home, "tag", id)

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "--tag") || !strings.Contains(run.err, "--off") {
		t.Errorf("stderr = %q, want it to name the flags that would say something", run.err)
	}
	if got := item(t, home, id).Tags; !slices.Equal(got, []string{"bug"}) {
		t.Errorf("folded tags = %q, want them left alone", got)
	}
}

// The log is a history of what was asked for, so a removal that changes nothing writes a line like
// any other rather than deciding the request was redundant.
func TestTagWritesEvenWhenTheSetDoesNotChange(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "bug")

	run := waid(t, home, "tag", id, "--remove", "perf")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got := len(logLines(t, home)); got != 2 {
		t.Errorf("log lines = %d, want the add and the write", got)
	}
	if got := item(t, home, id).Tags; !slices.Equal(got, []string{"bug"}) {
		t.Errorf("folded tags = %q, want them unchanged", got)
	}
}

func TestTagNormalisesWhatItWrites(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "bug")

	waid(t, home, "tag", id, "-t", "  ui  ", "-t", "bug", "-t", "ui")

	if got := item(t, home, id).Tags; !slices.Equal(got, []string{"bug", "ui"}) {
		t.Errorf("folded tags = %q, want the tag trimmed and the repeats dropped", got)
	}
}

func TestTagRejectsAnUnknownId(t *testing.T) {
	home := makeHome(t)
	addItem(t, home, "Chart rewrite")

	run := waid(t, home, "tag", "zzzz", "-t", "bug")

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

func TestTagWithoutAnIdExitsOne(t *testing.T) {
	home := makeHome(t)

	run := waid(t, home, "tag", "-t", "bug")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	if !strings.Contains(run.err, "item id") {
		t.Errorf("stderr = %q, want it to name the missing id", run.err)
	}
}

func TestTagHumanOutputIsOneLine(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "ui")

	run := waid(t, home, "tag", id, "-t", "bug")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got, want := run.out, "tagged "+id+"  Chart rewrite  ui, bug\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestTagHumanOutputNamesAnItemLeftWithNone(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart rewrite", "--tag", "ui")

	run := waid(t, home, "tag", id, "--off")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got, want := run.out, "untagged "+id+"  Chart rewrite\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
