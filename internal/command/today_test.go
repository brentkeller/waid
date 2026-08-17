package command_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/sessions"
)

const (
	drProject   = `C:\dev\dr\devresults`
	waidProject = `C:\dev\waid`
)

// sessionSpec is one seeded session, with everything the day windows do not depend on defaulted.
type sessionSpec struct {
	id      string
	title   string
	project *string
	started string
	ended   string
	prompts int
}

// at renders a local wall-clock instant as ISO, so day windows land the same way in every timezone.
func at(ymd string, hour, minute int) string {
	parsed, err := time.ParseInLocation("2006-01-02", ymd, time.Local)
	if err != nil {
		panic(err)
	}
	moment := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), hour, minute, 0, 0, time.Local)
	return moment.UTC().Format("2006-01-02T15:04:05.000Z")
}

func cachedSession(spec sessionSpec) sessions.CachedSession {
	title := spec.title
	if title == "" {
		title = "session " + spec.id
	}
	project := spec.project
	if project == nil {
		project = ptr(waidProject)
	}
	ended := spec.ended
	if ended == "" {
		ended = spec.started
	}

	return sessions.CachedSession{
		Session: sessions.Session{
			Id:      spec.id,
			Title:   title,
			Project: project,
			Branch:  nil,
			Started: ptr(spec.started),
			Ended:   ptr(ended),
			Prompts: spec.prompts,
		},
		File: sessions.FileStat{Path: `C:\transcripts\` + spec.id + `.jsonl`, MtimeNs: 1, Size: 1},
	}
}

func ptr[T any](value T) *T { return &value }

// seedSessions writes a cache stamped now, so the implicit sync sees it as fresh and leaves it
// alone.
func seedSessions(t *testing.T, home string, specs ...sessionSpec) {
	t.Helper()

	cached := make([]sessions.CachedSession, 0, len(specs))
	for _, spec := range specs {
		cached = append(cached, cachedSession(spec))
	}

	cache := sessions.Cache{
		Version:  sessions.CacheVersion,
		SyncedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Sessions: cached,
	}
	raw, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("encoding the cache: %v", err)
	}

	dir := filepath.Join(home, "cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), raw, 0o644); err != nil {
		t.Fatalf("writing the cache: %v", err)
	}
}

// seedLog writes the event log directly, so a test can place events on days a mutation could not
// reach.
func seedLog(t *testing.T, home string, events ...map[string]any) {
	t.Helper()

	lines := make([]string, 0, len(events))
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("encoding an event: %v", err)
		}
		lines = append(lines, string(raw))
	}
	if err := os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("writing the log: %v", err)
	}
}

// todayRecord is the --json payload today prints.
type todayRecord struct {
	Date     string `json:"date"`
	Sessions []struct {
		Id      string  `json:"id"`
		Title   string  `json:"title"`
		Project *string `json:"project"`
		Prompts int     `json:"prompts"`
	} `json:"sessions"`
	Projects []struct {
		Project  *string `json:"project"`
		Sessions int     `json:"sessions"`
		Prompts  int     `json:"prompts"`
	} `json:"projects"`
	Items struct {
		Added  []struct{ Id, Title string } `json:"added"`
		Closed []struct{ Id, Title string } `json:"closed"`
		Noted  []struct{ Id, Title string } `json:"noted"`
	} `json:"items"`
}

func (r todayRecord) sessionIds() []string {
	ids := make([]string, 0, len(r.Sessions))
	for _, session := range r.Sessions {
		ids = append(ids, session.Id)
	}
	return ids
}

func today(t *testing.T, home string, args ...string) todayRecord {
	t.Helper()

	run := waid(t, home, append(append([]string{"today"}, args...), "--json")...)
	if run.code != cli.ExitOK {
		t.Fatalf("today exited %d: %s", run.code, run.err)
	}
	var record todayRecord
	run.decode(t, &record)
	return record
}

func assertStrings(t *testing.T, got []string, want ...string) {
	t.Helper()

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTodayGroupsSessionsByProjectWithPromptCounts(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home,
		sessionSpec{id: "a", started: at("2026-08-10", 9, 0), ended: at("2026-08-10", 10, 0), prompts: 12},
		sessionSpec{id: "b", started: at("2026-08-10", 14, 0), ended: at("2026-08-10", 15, 0), prompts: 18},
		sessionSpec{id: "c", project: ptr(drProject), started: at("2026-08-10", 11, 0), ended: at("2026-08-10", 12, 0), prompts: 40},
		sessionSpec{id: "d", started: at("2026-08-09", 9, 0), ended: at("2026-08-09", 10, 0), prompts: 99},
	)

	data := today(t, home, "--date", "2026-08-10")

	if data.Date != "2026-08-10" {
		t.Fatalf("date = %q, want %q", data.Date, "2026-08-10")
	}
	assertStrings(t, data.sessionIds(), "a", "c", "b")

	if len(data.Projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(data.Projects))
	}
	if *data.Projects[0].Project != drProject || data.Projects[0].Sessions != 1 || data.Projects[0].Prompts != 40 {
		t.Fatalf("first project = %+v", data.Projects[0])
	}
	if *data.Projects[1].Project != waidProject || data.Projects[1].Sessions != 2 || data.Projects[1].Prompts != 30 {
		t.Fatalf("second project = %+v", data.Projects[1])
	}
}

func TestTodayOmitsZeroPromptSessions(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home,
		sessionSpec{id: "real", started: at("2026-08-10", 9, 0), ended: at("2026-08-10", 10, 0), prompts: 4},
		sessionSpec{id: "idle", started: at("2026-08-10", 9, 0), ended: at("2026-08-10", 9, 0), prompts: 0},
	)

	data := today(t, home, "--date", "2026-08-10")

	assertStrings(t, data.sessionIds(), "real")
	if len(data.Projects) != 1 || data.Projects[0].Prompts != 4 || data.Projects[0].Sessions != 1 {
		t.Fatalf("projects = %+v", data.Projects)
	}
}

func TestTodayListsASessionSpanningMidnightOnBothDays(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home, sessionSpec{
		id:      "owl",
		started: at("2026-08-10", 23, 30),
		ended:   at("2026-08-11", 0, 30),
		prompts: 7,
	})

	assertStrings(t, today(t, home, "--date", "2026-08-10").sessionIds(), "owl")
	assertStrings(t, today(t, home, "--date", "2026-08-11").sessionIds(), "owl")
}

func TestTodayDefaultsToTheCurrentDay(t *testing.T) {
	home := makeHome(t)
	now := time.Now()
	seedSessions(t, home,
		sessionSpec{
			id:      "now",
			started: now.UTC().Format("2006-01-02T15:04:05.000Z"),
			ended:   now.UTC().Format("2006-01-02T15:04:05.000Z"),
			prompts: 5,
		},
		sessionSpec{id: "then", started: at("2026-08-10", 9, 0), ended: at("2026-08-10", 10, 0), prompts: 2},
	)

	current := today(t, home)
	if current.Date != now.Local().Format("2006-01-02") {
		t.Fatalf("date = %q, want today", current.Date)
	}
	assertStrings(t, current.sessionIds(), "now")

	assertStrings(t, today(t, home, "--date", "2026-08-10").sessionIds(), "then")
}

func TestTodayListsItemsAddedClosedAndNoted(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home)
	seedLog(t, home,
		map[string]any{"ts": at("2026-08-09", 9, 0), "ev": "add", "id": "zz11", "title": "Ship the thing", "project": waidProject},
		map[string]any{"ts": at("2026-08-09", 9, 0), "ev": "add", "id": "p2vn", "title": "Decide on the repo", "project": waidProject},
		map[string]any{"ts": at("2026-08-10", 9, 0), "ev": "add", "id": "k3f9", "title": "Chart legend overflows", "project": drProject},
		map[string]any{"ts": at("2026-08-10", 10, 0), "ev": "close", "id": "zz11"},
		map[string]any{"ts": at("2026-08-10", 11, 0), "ev": "note", "id": "p2vn", "text": "leaning yes"},
		map[string]any{"ts": at("2026-08-10", 12, 0), "ev": "note", "id": "p2vn", "text": "still leaning yes"},
		map[string]any{"ts": at("2026-08-11", 9, 0), "ev": "add", "id": "x9x9", "title": "Tomorrow", "project": waidProject},
	)

	data := today(t, home, "--date", "2026-08-10")

	if len(data.Items.Added) != 1 || data.Items.Added[0].Id != "k3f9" {
		t.Fatalf("added = %+v", data.Items.Added)
	}
	if data.Items.Added[0].Title != "Chart legend overflows" {
		t.Fatalf("added title = %q", data.Items.Added[0].Title)
	}
	if len(data.Items.Closed) != 1 || data.Items.Closed[0].Id != "zz11" {
		t.Fatalf("closed = %+v", data.Items.Closed)
	}
	// Two notes on one item still list it once.
	if len(data.Items.Noted) != 1 || data.Items.Noted[0].Id != "p2vn" {
		t.Fatalf("noted = %+v", data.Items.Noted)
	}
}

func TestTodayLeavesTheCacheStatFieldsOut(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home, sessionSpec{
		id:      "a",
		started: at("2026-08-10", 9, 0),
		ended:   at("2026-08-10", 10, 0),
		prompts: 3,
	})

	run := waid(t, home, "today", "--date", "2026-08-10", "--json")
	if strings.Contains(run.out, "_file") {
		t.Fatalf("the cache stat fields reached the output:\n%s", run.out)
	}
}

func TestTodayWithNothingRecordedSaysSoAndSucceeds(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home)

	run := waid(t, home, "today", "--date", "2026-08-10")
	if run.code != cli.ExitOK {
		t.Fatalf("today exited %d: %s", run.code, run.err)
	}
	if !strings.Contains(run.out, "nothing recorded for 2026-08-10") {
		t.Fatalf("output = %q", run.out)
	}
}

func TestTodayRendersSessionsUnderProjectHeadingsWithItemActivity(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home,
		sessionSpec{
			id:      "a",
			title:   "Fix budget chart legend overflow",
			started: at("2026-08-10", 9, 0),
			ended:   at("2026-08-10", 10, 0),
			prompts: 12,
		},
		sessionSpec{
			id:      "c",
			title:   "Migrate the aspx charts",
			project: ptr(drProject),
			started: at("2026-08-10", 11, 0),
			ended:   at("2026-08-10", 12, 0),
			prompts: 40,
		},
	)
	seedLog(t, home, map[string]any{
		"ts": at("2026-08-10", 9, 0), "ev": "add", "id": "k3f9",
		"title": "Chart legend overflows", "project": drProject,
	})

	run := waid(t, home, "today", "--date", "2026-08-10")
	if run.code != cli.ExitOK {
		t.Fatalf("today exited %d: %s", run.code, run.err)
	}

	lines := strings.Split(strings.TrimRight(run.out, "\n"), "\n")
	if lines[0] != "TODAY  2026-08-10" {
		t.Fatalf("first line = %q", lines[0])
	}

	headings := []string{}
	for _, line := range lines {
		if regexp.MustCompile(`^ {2}\S`).MatchString(line) {
			headings = append(headings, line)
		}
	}
	assertStrings(t, headings,
		"  "+drProject+"  1 session, 40 prompts",
		"  "+waidProject+"  1 session, 12 prompts",
		"  ITEMS",
	)

	if !containsMatch(lines, `^ {4}Migrate the aspx charts`) {
		t.Fatalf("the session title is missing:\n%s", run.out)
	}
	if !containsMatch(lines, `^ {4}added {3}k3f9 {2}Chart legend overflows$`) {
		t.Fatalf("the item activity row is missing:\n%s", run.out)
	}
}

func TestTodayRejectsAnUnparseableDate(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home)

	run := waid(t, home, "today", "--date", "yesterday")
	if run.code != cli.ExitUser {
		t.Fatalf("today exited %d, want %d", run.code, cli.ExitUser)
	}
	if !strings.Contains(run.err, "--date") {
		t.Fatalf("stderr = %q", run.err)
	}
}

func TestTodayRejectsAnImpossibleDate(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home)

	run := waid(t, home, "today", "--date", "2026-02-31")
	if run.code != cli.ExitUser {
		t.Fatalf("today exited %d, want %d", run.code, cli.ExitUser)
	}
}

func containsMatch(lines []string, pattern string) bool {
	expression := regexp.MustCompile(pattern)
	for _, line := range lines {
		if expression.MatchString(line) {
			return true
		}
	}
	return false
}
