package command_test

import (
	"os"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/ids"
	"github.com/brentkeller/waid/internal/project"
)

// addRecord is the --json payload add echoes back.
type addRecord struct {
	Id        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Parent    *string  `json:"parent"`
	Origin    *string  `json:"origin"`
	Tags      []string `json:"tags"`
	WaitingOn *string  `json:"waitingOn"`
	Session   *string  `json:"session"`
	Created   string   `json:"created"`
}

func add(t *testing.T, home string, args ...string) addRecord {
	t.Helper()

	run := waid(t, home, append(append([]string{"add"}, args...), "--json")...)
	if run.code != cli.ExitOK {
		t.Fatalf("add exited %d: %s", run.code, run.err)
	}
	var record addRecord
	run.decode(t, &record)
	return record
}

func TestAddReturnsACompleteRecord(t *testing.T) {
	home := makeHome(t)

	record := add(t, home,
		"Chart legend overflows",
		"-p", `C:\dev\waid`,
		"--tag", "bug",
		"--tag", "ui",
		"--session", "sess-1",
	)

	if record.Title != "Chart legend overflows" {
		t.Errorf("title = %q", record.Title)
	}
	if record.Status != "open" {
		t.Errorf("status = %q, want open", record.Status)
	}
	if record.Origin == nil || *record.Origin != `C:\dev\waid` {
		t.Errorf("origin = %v, want C:\\dev\\waid", record.Origin)
	}
	if record.Parent != nil {
		t.Errorf("parent = %v, want null", *record.Parent)
	}
	if len(record.Tags) != 2 || record.Tags[0] != "bug" || record.Tags[1] != "ui" {
		t.Errorf("tags = %v, want [bug ui]", record.Tags)
	}
	if record.WaitingOn != nil {
		t.Errorf("waitingOn = %v, want null", *record.WaitingOn)
	}
	if record.Session == nil || *record.Session != "sess-1" {
		t.Errorf("session = %v, want sess-1", record.Session)
	}
	if len(record.Id) != ids.Length {
		t.Errorf("id = %q, want %d characters", record.Id, ids.Length)
	}
	for _, char := range record.Id {
		if !strings.ContainsRune(ids.Alphabet, char) {
			t.Errorf("unexpected id char %q in %q", char, record.Id)
		}
	}
	if record.Created == "" {
		t.Error("created is empty")
	}
}

// Tags are an empty array rather than null when none were given, matching what the log holds.
func TestAddWithoutTagsCarriesAnEmptyArray(t *testing.T) {
	home := makeHome(t)

	run := waid(t, home, "add", "Ship the thing", "--json")

	if !strings.Contains(run.out, `"tags": []`) {
		t.Errorf("stdout = %q, want an empty tags array", run.out)
	}
}

func TestAddJoinsItsPositionalsIntoTheTitle(t *testing.T) {
	home := makeHome(t)

	record := add(t, home, "Decide", "whether", "to", "ship")

	if record.Title != "Decide whether to ship" {
		t.Errorf("title = %q, want the positionals joined", record.Title)
	}
	if record.Origin != nil {
		t.Errorf("origin = %q, want null", *record.Origin)
	}
}

func TestAddWaitingOnYieldsWaitingStatus(t *testing.T) {
	home := makeHome(t)

	record := add(t, home, "Approve the copy", "--waiting-on", "Dan")

	if record.Status != "waiting" {
		t.Errorf("status = %q, want waiting", record.Status)
	}
	if record.WaitingOn == nil || *record.WaitingOn != "Dan" {
		t.Errorf("waitingOn = %v, want Dan", record.WaitingOn)
	}
}

// -p reads as "parent" now: a fragment naming one item files the new item under it.
func TestAddFilesUnderAParentNamedByAFragment(t *testing.T) {
	home := makeHome(t)
	parent := addItem(t, home, "Chart rewrite")

	record := add(t, home, "Legend overflows", "-p", "chart")

	if record.Parent == nil || *record.Parent != parent {
		t.Errorf("parent = %v, want %s", record.Parent, parent)
	}
	if record.Origin != nil {
		t.Errorf("origin = %v, want null", *record.Origin)
	}
	if got := item(t, home, record.Id); got.Parent == nil || *got.Parent != parent {
		t.Errorf("folded parent = %v, want %s", got.Parent, parent)
	}
}

// An absolute path is provenance, so it lands as origin with the item left at top level.
func TestAddWithAnAbsolutePathRecordsAnOriginAndNoParent(t *testing.T) {
	home := makeHome(t)
	addItem(t, home, "Chart rewrite")

	record := add(t, home, "Legend overflows", "-p", `C:\dev\waid`)

	if record.Origin == nil || *record.Origin != `C:\dev\waid` {
		t.Errorf("origin = %v, want C:\\dev\\waid", record.Origin)
	}
	if record.Parent != nil {
		t.Errorf("parent = %v, want null", *record.Parent)
	}
	if got := item(t, home, record.Id); got.Parent != nil {
		t.Errorf("folded parent = %v, want null", *got.Parent)
	}
}

// `.` is the current directory, which is the form the agent snippet uses.
func TestAddWithADotRecordsTheWorkingDirectoryAsOrigin(t *testing.T) {
	home := makeHome(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	record := add(t, home, "Legend overflows", "-p", ".")

	if want := project.NormalizePath(cwd); record.Origin == nil || *record.Origin != want {
		t.Errorf("origin = %v, want %s", record.Origin, want)
	}
	if record.Parent != nil {
		t.Errorf("parent = %v, want null", *record.Parent)
	}
}

// No -p at all leaves both fields unset rather than guessing at either.
func TestAddWithoutAParentRecordsNeitherField(t *testing.T) {
	home := makeHome(t)

	record := add(t, home, "Legend overflows")

	if record.Parent != nil {
		t.Errorf("parent = %v, want null", *record.Parent)
	}
	if record.Origin != nil {
		t.Errorf("origin = %v, want null", *record.Origin)
	}
}

func TestAddRejectsAFragmentMatchingNoItem(t *testing.T) {
	home := makeHome(t)
	addItem(t, home, "Chart rewrite")

	run := waid(t, home, "add", "Legend overflows", "-p", "invoicing")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d", run.code, cli.ExitUser)
	}
	if !strings.Contains(run.err, "invoicing") {
		t.Errorf("stderr = %q, want it to name the fragment", run.err)
	}
	if got := len(logLines(t, home)); got != 1 {
		t.Errorf("log lines = %d, want only the parent's add", got)
	}
}

func TestAddRejectsAnAmbiguousFragmentAndListsTheMatches(t *testing.T) {
	home := makeHome(t)
	first := addItem(t, home, "Chart rewrite")
	second := addItem(t, home, "Chart legend")

	run := waid(t, home, "add", "Third", "-p", "chart")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d", run.code, cli.ExitUser)
	}
	for _, candidate := range []string{first, "Chart rewrite", second, "Chart legend"} {
		if !strings.Contains(run.err, candidate) {
			t.Errorf("stderr = %q, want it to list %s", run.err, candidate)
		}
	}
	if got := len(logLines(t, home)); got != 2 {
		t.Errorf("log lines = %d, want only the two parents", got)
	}
}

func TestAddWithoutATitleExitsOne(t *testing.T) {
	for name, args := range map[string][]string{
		"no title":    {"add"},
		"blank title": {"add", "   "},
	} {
		t.Run(name, func(t *testing.T) {
			home := makeHome(t)

			run := waid(t, home, args...)

			if run.code != cli.ExitUser {
				t.Fatalf("exit code = %d, want %d", run.code, cli.ExitUser)
			}
			if !strings.Contains(run.err, "title") {
				t.Errorf("stderr = %q, want it to name the missing title", run.err)
			}
			if len(logLines(t, home)) != 0 {
				t.Errorf("log = %v, want nothing written", logLines(t, home))
			}
		})
	}
}

func TestAddHumanOutputIsOneLine(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvIds, "abcd")

	run := waid(t, home, "add", "Ship the thing")

	if run.code != cli.ExitOK {
		t.Fatalf("exit code = %d: %s", run.code, run.err)
	}
	if got, want := run.out, "added abcd  Ship the thing\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestAddAppendsOneEventPerCallWithUniqueIds(t *testing.T) {
	home := makeHome(t)

	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		seen[add(t, home, "Item").Id] = true
	}

	if len(seen) != 5 {
		t.Errorf("distinct ids = %d, want 5", len(seen))
	}
	if got := len(logLines(t, home)); got != 5 {
		t.Errorf("log lines = %d, want 5", got)
	}
	if seen[add(t, home, "Sixth").Id] {
		t.Error("the sixth add reused an id already taken")
	}
}

// A pinned id already in the log is skipped rather than duplicated.
func TestAddSkipsAPinnedIdAlreadyTaken(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvIds, "abcd,efgh")

	first := add(t, home, "First")
	second := add(t, home, "Second")

	if first.Id != "abcd" || second.Id != "efgh" {
		t.Fatalf("ids = %q, %q, want abcd, efgh", first.Id, second.Id)
	}
}

// The appended line is what the port is ultimately judged on: the key order, the raw backslashes and
// the millisecond timestamp all have to match what Node wrote into the same log.
func TestAddAppendsTheExactLine(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd")

	add(t, home,
		"Chart legend overflows",
		"-p", `C:\dev\waid`,
		"--tag", "bug",
		"--tag", "ui",
		"--session", "sess-1",
		"--waiting-on", "Dan",
	)

	want := `{"ts":"2026-08-17T12:00:00.000Z","ev":"add","id":"abcd","title":"Chart legend overflows",` +
		`"status":"waiting","origin":"C:\\dev\\waid","session":"sess-1","tags":["bug","ui"],"waitingOn":"Dan"}`
	if got := logLines(t, home); len(got) != 1 || got[0] != want {
		t.Fatalf("log = %q,\nwant [%q]", got, want)
	}
}

// An add carrying nothing optional still writes every key, with null and [] where Node writes them.
func TestAddAppendsTheExactLineWithoutOptions(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd")

	add(t, home, "Ship the thing")

	want := `{"ts":"2026-08-17T12:00:00.000Z","ev":"add","id":"abcd","title":"Ship the thing",` +
		`"status":"open","origin":null,"session":null,"tags":[],"waitingOn":null}`
	if got := logLines(t, home); len(got) != 1 || got[0] != want {
		t.Fatalf("log = %q,\nwant [%q]", got, want)
	}
}

// An add filed under a parent carries the key, and only then.
func TestAddUnderAParentAppendsTheExactLine(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd,efgh")
	addItem(t, home, "Chart rewrite")

	add(t, home, "Legend overflows", "-p", "chart")

	want := `{"ts":"2026-08-17T12:00:00.000Z","ev":"add","id":"efgh","title":"Legend overflows",` +
		`"status":"open","parent":"abcd","origin":null,"session":null,"tags":[],"waitingOn":null}`
	if got := logLines(t, home); len(got) != 2 || got[1] != want {
		t.Fatalf("log = %q,\nwant the second line %q", got, want)
	}
}

// --heading creates a landmark outright, so a fresh project need not be marked in a second step.
func TestAddHeadingWritesTheKey(t *testing.T) {
	home := makeHome(t)
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00Z")
	t.Setenv(cli.EnvIds, "abcd")

	record := add(t, home, "Chart rewrite", "--heading")

	want := `{"ts":"2026-08-17T12:00:00.000Z","ev":"add","id":"abcd","title":"Chart rewrite",` +
		`"status":"open","origin":null,"session":null,"tags":[],"waitingOn":null,"heading":true}`
	if got := logLines(t, home); len(got) != 1 || got[0] != want {
		t.Fatalf("log = %q,\nwant [%q]", got, want)
	}
	if !item(t, home, record.Id).Heading {
		t.Error("folded heading = false, want the item marked")
	}
}

// The key is omitted when it was not asked for, so a plain add re-encodes as it always has.
func TestAddWithoutHeadingWritesNoKey(t *testing.T) {
	home := makeHome(t)

	record := add(t, home, "Chart rewrite")

	if got := logLines(t, home); len(got) != 1 || strings.Contains(got[0], "heading") {
		t.Fatalf("log = %q, want no heading key", got)
	}
	if item(t, home, record.Id).Heading {
		t.Error("folded heading = true, want it unmarked")
	}
}
