package command_test

import (
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
)

func TestDoneClosesAnItem(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Ship the thing")

	run := waid(t, home, "done", id, "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record struct {
		Id     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
		Ts     string `json:"ts"`
	}
	run.decode(t, &record)

	if record.Id != id || record.Title != "Ship the thing" || record.Status != "done" {
		t.Errorf("record = %+v, want the closed item", record)
	}
	if record.Ts == "" {
		t.Error("ts is empty")
	}
	if got := item(t, home, id).Status; got != "done" {
		t.Errorf("folded status = %q, want done", got)
	}
}

func TestReopenClearsWaitingOn(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Approve the copy", "--waiting-on", "Dan")
	if got := item(t, home, id).Status; got != "waiting" {
		t.Fatalf("folded status = %q, want waiting", got)
	}

	run := waid(t, home, "reopen", id, "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record struct {
		Status string `json:"status"`
	}
	run.decode(t, &record)
	if record.Status != "open" {
		t.Errorf("status = %q, want open", record.Status)
	}

	reopened := item(t, home, id)
	if reopened.Status != "open" {
		t.Errorf("folded status = %q, want open", reopened.Status)
	}
	if reopened.WaitingOn != nil {
		t.Errorf("waitingOn = %q, want null", *reopened.WaitingOn)
	}
}

func TestReopenBringsADoneItemBack(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Ship the thing")
	waid(t, home, "done", id)

	run := waid(t, home, "reopen", id)

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got := item(t, home, id).Status; got != "open" {
		t.Errorf("folded status = %q, want open", got)
	}
}

func TestNoteAppendsTextToTheItem(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart legend overflows")

	run := waid(t, home, "note", id, "blocked", "on", "the", "API", "--json")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	var record struct {
		Text string `json:"text"`
	}
	run.decode(t, &record)
	if record.Text != "blocked on the API" {
		t.Errorf("text = %q, want the positionals joined", record.Text)
	}

	notes := item(t, home, id).Notes
	if len(notes) != 1 || notes[0].Text != "blocked on the API" {
		t.Fatalf("notes = %+v, want the one note", notes)
	}
	if notes[0].Ts == "" {
		t.Error("note ts is empty")
	}
}

func TestNotesAccumulateInOrder(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart legend overflows")

	waid(t, home, "note", id, "first")
	waid(t, home, "note", id, "second")

	notes := item(t, home, id).Notes
	if len(notes) != 2 || notes[0].Text != "first" || notes[1].Text != "second" {
		t.Fatalf("notes = %+v, want first then second", notes)
	}
}

func TestMutationsRejectAnUnknownId(t *testing.T) {
	for _, name := range []string{"done", "reopen", "note"} {
		t.Run(name, func(t *testing.T) {
			home := makeHome(t)
			addItem(t, home, "Ship the thing")
			before := len(logLines(t, home))

			run := waid(t, home, name, "zzzz", "some text")

			if run.code != cli.ExitUser {
				t.Fatalf("exit code = %d, want %d", run.code, cli.ExitUser)
			}
			if !strings.Contains(run.err, "unknown item id: zzzz") {
				t.Errorf("stderr = %q, want it to name the unknown id", run.err)
			}
			if got := len(logLines(t, home)); got != before {
				t.Errorf("log lines = %d, want %d — nothing written", got, before)
			}
		})
	}
}

func TestMutationsRequireAnId(t *testing.T) {
	for _, name := range []string{"done", "reopen", "note"} {
		t.Run(name, func(t *testing.T) {
			home := makeHome(t)

			run := waid(t, home, name)

			if run.code != cli.ExitUser {
				t.Fatalf("exit code = %d, want %d", run.code, cli.ExitUser)
			}
			if !strings.Contains(run.err, "item id") {
				t.Errorf("stderr = %q, want it to name the missing id", run.err)
			}
		})
	}
}

func TestNoteRequiresText(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart legend overflows")
	before := len(logLines(t, home))

	run := waid(t, home, "note", id, "   ")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d", run.code, cli.ExitUser)
	}
	if !strings.Contains(run.err, "text") {
		t.Errorf("stderr = %q, want it to name the missing text", run.err)
	}
	if got := len(logLines(t, home)); got != before {
		t.Errorf("log lines = %d, want %d — nothing written", got, before)
	}
}

func TestMutationsRenderOneLineEach(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Ship the thing")

	closed := waid(t, home, "done", id)
	if got, want := closed.out, "closed "+id+"  Ship the thing\n"; got != want {
		t.Errorf("done stdout = %q, want %q", got, want)
	}

	reopened := waid(t, home, "reopen", id)
	if got, want := reopened.out, "reopened "+id+"  Ship the thing\n"; got != want {
		t.Errorf("reopen stdout = %q, want %q", got, want)
	}

	noted := waid(t, home, "note", id, "still going")
	if got, want := noted.out, "noted "+id+"  still going\n"; got != want {
		t.Errorf("note stdout = %q, want %q", got, want)
	}
}

// The three mutations append the exact lines Node appends, ampersand and all.
func TestMutationsAppendTheExactLines(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd")

	addItem(t, home, "Ship the thing")
	waid(t, home, "done", "abcd")
	waid(t, home, "reopen", "abcd")
	waid(t, home, "note", "abcd", "blocked on <legends> & labels")

	want := []string{
		`{"ts":"2026-08-17T12:00:00.000Z","ev":"add","id":"abcd","title":"Ship the thing",` +
			`"status":"open","origin":null,"session":null,"tags":[],"waitingOn":null}`,
		`{"ts":"2026-08-17T12:00:00.000Z","ev":"close","id":"abcd"}`,
		`{"ts":"2026-08-17T12:00:00.000Z","ev":"reopen","id":"abcd"}`,
		`{"ts":"2026-08-17T12:00:00.000Z","ev":"note","id":"abcd","text":"blocked on <legends> & labels"}`,
	}
	got := logLines(t, home)
	if len(got) != len(want) {
		t.Fatalf("log = %q, want %d lines", got, len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("line %d =\n  %s\nwant\n  %s", index+1, got[index], want[index])
		}
	}
}
