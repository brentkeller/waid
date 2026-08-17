package command_test

import (
	"strings"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/cli"
)

// monday returns the Monday of the week offsetWeeks away from the current one, at local midnight.
func monday(offsetWeeks int) time.Time {
	now := time.Now().Local()
	daysSinceMonday := (int(now.Weekday()) + 6) % 7
	return time.Date(
		now.Year(), now.Month(), now.Day()-daysSinceMonday+offsetWeeks*7,
		0, 0, 0, 0, time.Local,
	)
}

// inWeek renders a local wall-clock instant inside a week as ISO, with day counted from Monday (0)
// to Sunday (6).
func inWeek(offsetWeeks, day, hour int) string {
	start := monday(offsetWeeks)
	moment := time.Date(start.Year(), start.Month(), start.Day()+day, hour, 0, 0, 0, time.Local)
	return moment.UTC().Format("2006-01-02T15:04:05.000Z")
}

// ymdOf reads the local calendar date of an ISO instant a test built with inWeek.
func ymdOf(t *testing.T, iso string) string {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t.Fatalf("parsing %q: %v", iso, err)
	}
	return parsed.Local().Format("2006-01-02")
}

// weekRecord is the --json payload week prints.
type weekRecord struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Projects []struct {
		Project     *string  `json:"project"`
		Sessions    int      `json:"sessions"`
		Prompts     int      `json:"prompts"`
		Titles      []string `json:"titles"`
		ItemsClosed int      `json:"itemsClosed"`
	} `json:"projects"`
	Totals struct {
		Sessions int `json:"sessions"`
		Prompts  int `json:"prompts"`
		Closed   int `json:"closed"`
	} `json:"totals"`
}

func week(t *testing.T, home string, args ...string) weekRecord {
	t.Helper()

	run := waid(t, home, append(append([]string{"week"}, args...), "--json")...)
	if run.code != cli.ExitOK {
		t.Fatalf("week exited %d: %s", run.code, run.err)
	}
	var record weekRecord
	run.decode(t, &record)
	return record
}

func TestWeekCoversMondayToSunday(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home,
		sessionSpec{id: "mon", started: inWeek(0, 0, 9), prompts: 1},
		sessionSpec{id: "sun", started: inWeek(0, 6, 23), prompts: 1},
		sessionSpec{id: "last-sun", started: inWeek(-1, 6, 23), prompts: 1},
		sessionSpec{id: "next-mon", started: inWeek(1, 0, 0), prompts: 1},
	)

	data := week(t, home)

	if data.Start != monday(0).Format("2006-01-02") {
		t.Fatalf("start = %q, want %q", data.Start, monday(0).Format("2006-01-02"))
	}
	if data.End != ymdOf(t, inWeek(0, 6, 12)) {
		t.Fatalf("end = %q, want %q", data.End, ymdOf(t, inWeek(0, 6, 12)))
	}
	assertStrings(t, data.Projects[0].Titles, "session mon", "session sun")
	if data.Totals.Sessions != 2 {
		t.Fatalf("sessions = %d, want 2", data.Totals.Sessions)
	}
}

func TestWeekLastSelectsThePreviousWindow(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home,
		sessionSpec{id: "this", started: inWeek(0, 1, 9), prompts: 1},
		sessionSpec{id: "prev", started: inWeek(-1, 1, 9), prompts: 1},
	)

	data := week(t, home, "--last")

	if data.Start != monday(-1).Format("2006-01-02") {
		t.Fatalf("start = %q, want %q", data.Start, monday(-1).Format("2006-01-02"))
	}
	if data.End != ymdOf(t, inWeek(-1, 6, 12)) {
		t.Fatalf("end = %q, want %q", data.End, ymdOf(t, inWeek(-1, 6, 12)))
	}
	assertStrings(t, data.Projects[0].Titles, "session prev")
	if data.Totals.Sessions != 1 {
		t.Fatalf("sessions = %d, want 1", data.Totals.Sessions)
	}
}

func TestWeekOrdersProjectsByPromptCount(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home,
		sessionSpec{id: "a", title: "Fix the legend", started: inWeek(0, 0, 9), prompts: 5},
		sessionSpec{id: "b", title: "Ship the rollup", started: inWeek(0, 2, 9), prompts: 6},
		sessionSpec{id: "c", title: "Migrate the aspx charts", project: ptr(drProject), started: inWeek(0, 1, 9), prompts: 40},
	)

	data := week(t, home)

	if len(data.Projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(data.Projects))
	}
	first := data.Projects[0]
	if *first.Project != drProject || first.Sessions != 1 || first.Prompts != 40 || first.ItemsClosed != 0 {
		t.Fatalf("first project = %+v", first)
	}
	assertStrings(t, first.Titles, "Migrate the aspx charts")
	assertStrings(t, data.Projects[1].Titles, "Fix the legend", "Ship the rollup")
}

func TestWeekCountsItemsClosedInTheWindowPerProject(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home, sessionSpec{id: "a", started: inWeek(0, 0, 9), prompts: 3})
	seedLog(t, home,
		map[string]any{"ts": inWeek(-2, 0, 9), "ev": "add", "id": "zz11", "title": "Ship the thing", "project": waidProject},
		map[string]any{"ts": inWeek(-2, 0, 9), "ev": "add", "id": "k3f9", "title": "Chart legend overflows", "project": drProject},
		map[string]any{"ts": inWeek(-2, 0, 9), "ev": "add", "id": "p2vn", "title": "Old news", "project": drProject},
		map[string]any{"ts": inWeek(0, 1, 10), "ev": "close", "id": "zz11"},
		map[string]any{"ts": inWeek(0, 2, 10), "ev": "close", "id": "k3f9"},
		// Closed before the window opened, so it belongs to an earlier week.
		map[string]any{"ts": inWeek(-1, 2, 10), "ev": "close", "id": "p2vn"},
	)

	data := week(t, home)

	groups := map[string]int{}
	sessionCounts := map[string]int{}
	for _, group := range data.Projects {
		if group.Project == nil {
			continue
		}
		groups[*group.Project] = group.ItemsClosed
		sessionCounts[*group.Project] = group.Sessions
	}

	if groups[waidProject] != 1 || groups[drProject] != 1 {
		t.Fatalf("closures = %v", groups)
	}
	// A project with a closure but no sessions still earns a group.
	if sessionCounts[drProject] != 0 {
		t.Fatalf("%s sessions = %d, want 0", drProject, sessionCounts[drProject])
	}
}

func TestWeekTotalsAreTheSumOfTheGroups(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home,
		sessionSpec{id: "a", started: inWeek(0, 0, 9), prompts: 5},
		sessionSpec{id: "b", started: inWeek(0, 2, 9), prompts: 6},
		sessionSpec{id: "c", project: ptr(drProject), started: inWeek(0, 1, 9), prompts: 40},
	)
	seedLog(t, home,
		map[string]any{"ts": inWeek(-2, 0, 9), "ev": "add", "id": "zz11", "title": "Ship the thing", "project": waidProject},
		map[string]any{"ts": inWeek(0, 1, 10), "ev": "close", "id": "zz11"},
	)

	data := week(t, home)

	sessions, prompts, closed := 0, 0, 0
	for _, group := range data.Projects {
		sessions += group.Sessions
		prompts += group.Prompts
		closed += group.ItemsClosed
	}

	if data.Totals.Sessions != sessions || data.Totals.Prompts != prompts || data.Totals.Closed != closed {
		t.Fatalf("totals %+v do not sum the groups (%d, %d, %d)", data.Totals, sessions, prompts, closed)
	}
	if data.Totals.Sessions != 3 || data.Totals.Prompts != 51 || data.Totals.Closed != 1 {
		t.Fatalf("totals = %+v", data.Totals)
	}
}

func TestWeekWithNothingRecordedSaysSoAndSucceeds(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home)

	run := waid(t, home, "week")
	if run.code != cli.ExitOK {
		t.Fatalf("week exited %d: %s", run.code, run.err)
	}
	if !containsMatch(strings.Split(run.out, "\n"), `^nothing recorded for the week of \d{4}-\d{2}-\d{2}$`) {
		t.Fatalf("output = %q", run.out)
	}
}

func TestWeekRendersProjectRollupsWithTitlesAndATotal(t *testing.T) {
	home := makeHome(t)
	seedSessions(t, home,
		sessionSpec{id: "a", title: "Fix the legend", started: inWeek(0, 0, 9), prompts: 5},
		sessionSpec{id: "c", title: "Migrate the aspx charts", project: ptr(drProject), started: inWeek(0, 1, 9), prompts: 40},
	)
	seedLog(t, home,
		map[string]any{"ts": inWeek(-2, 0, 9), "ev": "add", "id": "zz11", "title": "Ship the thing", "project": waidProject},
		map[string]any{"ts": inWeek(0, 1, 10), "ev": "close", "id": "zz11"},
	)

	run := waid(t, home, "week")
	if run.code != cli.ExitOK {
		t.Fatalf("week exited %d: %s", run.code, run.err)
	}

	lines := strings.Split(strings.TrimRight(run.out, "\n"), "\n")
	want := "WEEK  " + monday(0).Format("2006-01-02") + " → " + ymdOf(t, inWeek(0, 6, 12))
	if lines[0] != want {
		t.Fatalf("first line = %q, want %q", lines[0], want)
	}

	headings := []string{}
	for _, line := range lines {
		if len(line) > 2 && strings.HasPrefix(line, "  ") && line[2] != ' ' {
			headings = append(headings, line)
		}
	}
	assertStrings(t, headings,
		"  "+drProject+"  1 session, 40 prompts, 0 closed",
		"  "+waidProject+"  1 session, 5 prompts, 1 closed",
		"  TOTAL  2 sessions, 45 prompts, 1 closed",
	)

	if !containsMatch(lines, `^ {4}Migrate the aspx charts$`) {
		t.Fatalf("the session title is missing:\n%s", run.out)
	}
}
