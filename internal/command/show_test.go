package command_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/render"
)

// showRecord is the --json payload show echoes back.
type showRecord struct {
	Item struct {
		Id      string  `json:"id"`
		Status  string  `json:"status"`
		Origin  *string `json:"origin"`
		Heading bool    `json:"heading"`
		Notes   []struct {
			Text string `json:"text"`
		} `json:"notes"`
	} `json:"item"`
	History []struct {
		Ts   string `json:"ts"`
		Ev   string `json:"ev"`
		Line int    `json:"line"`
	} `json:"history"`
}

func (r showRecord) events() []string {
	evs := make([]string, 0, len(r.History))
	for _, entry := range r.History {
		evs = append(evs, entry.Ev)
	}
	return evs
}

func show(t *testing.T, home string, args ...string) showRecord {
	t.Helper()

	run := waid(t, home, append(append([]string{"show"}, args...), "--json")...)
	if run.code != cli.ExitOK {
		t.Fatalf("show exited %d: %s", run.code, run.err)
	}
	var record showRecord
	run.decode(t, &record)
	return record
}

func TestShowReturnsTheItemWithNotesAndHistory(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Chart legend overflows", "-p", `C:\dev\waid`)
	waid(t, home, "note", id, "blocked on the API")
	waid(t, home, "done", id)

	record := show(t, home, id)

	if record.Item.Id != id {
		t.Errorf("item.id = %q, want %q", record.Item.Id, id)
	}
	if record.Item.Status != "done" {
		t.Errorf("item.status = %q, want done", record.Item.Status)
	}
	if record.Item.Origin == nil || *record.Item.Origin != `C:\dev\waid` {
		t.Errorf("item.origin = %v, want C:\\dev\\waid", record.Item.Origin)
	}
	if len(record.Item.Notes) != 1 || record.Item.Notes[0].Text != "blocked on the API" {
		t.Errorf("item.notes = %v", record.Item.Notes)
	}
	if !slices.Equal(record.events(), []string{"add", "note", "close"}) {
		t.Errorf("history events = %v, want [add note close]", record.events())
	}

	lines := make([]int, 0, len(record.History))
	for _, entry := range record.History {
		lines = append(lines, entry.Line)
		if _, ok := render.ParseTime(entry.Ts); !ok {
			t.Errorf("history ts is not a timestamp: %q", entry.Ts)
		}
	}
	if !slices.Equal(lines, []int{1, 2, 3}) {
		t.Errorf("history lines = %v, want [1 2 3]", lines)
	}
}

func TestShowHistoryIgnoresOtherItems(t *testing.T) {
	home := makeHome(t)
	mine := addItem(t, home, "Mine")
	other := addItem(t, home, "Other")
	waid(t, home, "note", other, "not mine")

	record := show(t, home, mine)

	if !slices.Equal(record.events(), []string{"add"}) {
		t.Errorf("history events = %v, want [add]", record.events())
	}
}

func TestShowRejectsAnUnknownId(t *testing.T) {
	home := makeHome(t)
	addItem(t, home, "Chart legend overflows")

	run := waid(t, home, "show", "zzzz")
	if run.code != cli.ExitUser {
		t.Fatalf("code = %d, want %d", run.code, cli.ExitUser)
	}
	if !strings.Contains(run.err, "unknown item id: zzzz") {
		t.Errorf("stderr = %q", run.err)
	}
}

func TestShowRequiresAnId(t *testing.T) {
	home := makeHome(t)

	run := waid(t, home, "show")
	if run.code != cli.ExitUser {
		t.Fatalf("code = %d, want %d", run.code, cli.ExitUser)
	}
	if !strings.Contains(run.err, "item id") {
		t.Errorf("stderr = %q, want it to name the missing item id", run.err)
	}
}

func TestShowRendersTheItemItsNotesAndItsHistory(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Approve report copy", "--waiting-on", "Dan", "--tag", "copy")
	waid(t, home, "note", id, "pinged him again")

	run := waid(t, home, "show", id)
	if run.code != cli.ExitOK {
		t.Fatalf("show exited %d: %s", run.code, run.err)
	}

	if !regexp.MustCompile(id + `\s+Approve report copy`).MatchString(run.out) {
		t.Errorf("stdout does not head with the id and title:\n%s", run.out)
	}
	for _, part := range []string{"waiting", "Dan", "copy", "origin", "pinged him again", "note", "history", "notes"} {
		if !strings.Contains(run.out, part) {
			t.Errorf("stdout does not carry %q:\n%s", part, run.out)
		}
	}
}

func TestShowOmitsTheNotesBlockWhenThereAreNone(t *testing.T) {
	home := makeHome(t)
	id := addItem(t, home, "Nothing noted")

	run := waid(t, home, "show", id)
	if strings.Contains(run.out, "  notes") {
		t.Errorf("stdout carries an empty notes block:\n%s", run.out)
	}
	if !strings.Contains(run.out, "  history") {
		t.Errorf("stdout carries no history block:\n%s", run.out)
	}
}

func TestShowCarriesTheHeadingFlag(t *testing.T) {
	home := makeHome(t)
	shelf := addItem(t, home, "Localized notifications", "--heading")
	task := addItem(t, home, "Read the port design once more")

	if !show(t, home, shelf).Item.Heading {
		t.Error("item.heading = false, want the marked item to carry it")
	}
	if show(t, home, task).Item.Heading {
		t.Error("item.heading = true, want an ordinary item to carry false")
	}
}

func TestShowNamesAHeadingBesideTheStatus(t *testing.T) {
	home := makeHome(t)
	shelf := addItem(t, home, "Localized notifications", "--heading")
	task := addItem(t, home, "Read the port design once more")

	marked := statusLine(t, waid(t, home, "show", shelf).out)
	if !strings.Contains(marked, "open") || !strings.Contains(marked, "heading") {
		t.Errorf("status line = %q, want it to name the status and the heading", marked)
	}

	if plain := statusLine(t, waid(t, home, "show", task).out); strings.Contains(plain, "heading") {
		t.Errorf("status line = %q, want no heading on an ordinary item", plain)
	}
}

// statusLine is the line show hangs the status off, which is where a heading is named.
func statusLine(t *testing.T, out string) string {
	t.Helper()

	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "status") {
			return line
		}
	}
	t.Fatalf("no status line in:\n%s", out)
	return ""
}
