package command_test

import (
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/ids"
)

// addRecord is the --json payload add echoes back.
type addRecord struct {
	Id        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Project   *string  `json:"project"`
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
	if record.Project == nil || *record.Project != `C:\dev\waid` {
		t.Errorf("project = %v, want C:\\dev\\waid", record.Project)
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
	if record.Project != nil {
		t.Errorf("project = %q, want null", *record.Project)
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

func TestAddResolvesAPartialProjectAgainstAnEarlierAdd(t *testing.T) {
	home := makeHome(t)
	add(t, home, "First", "-p", `C:\dev\dr\devresults`)

	record := add(t, home, "Second", "-p", "devresults")

	if record.Project == nil || *record.Project != `C:\dev\dr\devresults` {
		t.Errorf("project = %v, want the path the earlier add used", record.Project)
	}
}

func TestAddRejectsAnAmbiguousPartialProject(t *testing.T) {
	home := makeHome(t)
	add(t, home, "First", "-p", `C:\dev\alpha`)
	add(t, home, "Second", "-p", `C:\dev\beta`)

	run := waid(t, home, "add", "Third", "-p", "dev")

	if run.code != cli.ExitUser {
		t.Fatalf("exit code = %d, want %d", run.code, cli.ExitUser)
	}
	for _, candidate := range []string{`C:\dev\alpha`, `C:\dev\beta`} {
		if !strings.Contains(run.err, candidate) {
			t.Errorf("stderr = %q, want it to list %s", run.err, candidate)
		}
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
