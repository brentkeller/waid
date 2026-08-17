package command

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/sessions"
)

// DayProject is one project's share of a day: how many sessions ran in it and how many prompts they
// took.
type DayProject struct {
	Project  *string `json:"project"`
	Sessions int     `json:"sessions"`
	Prompts  int     `json:"prompts"`
}

// DayItems are the items a day touched, split by the kind of event that touched them.
type DayItems struct {
	Added  []events.Item `json:"added"`
	Closed []events.Item `json:"closed"`
	Noted  []events.Item `json:"noted"`
}

// TodayResult is one local calendar day: the sessions that ran, the projects they rolled up into,
// and the items the day's events touched.
type TodayResult struct {
	// Date is the local calendar day the rest of the record describes, as `YYYY-MM-DD`.
	Date string `json:"date"`
	// Sessions overlap the day, oldest first; zero-prompt sessions are left out.
	Sessions []sessions.Session `json:"sessions"`
	// Projects are the same sessions rolled up by project, busiest first.
	Projects []DayProject `json:"projects"`
	Items    DayItems     `json:"items"`
}

var ymdPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Column widths for the day's rows: a session title matches the item rows list renders, and the
// widest item activity label sets the label column.
const (
	sessionTitleWidth  = 44
	activityLabelWidth = 8
)

func runToday(ctx *cli.Ctx) (TodayResult, error) {
	requested, passed := ctx.Flags.String("date")
	date, err := readDate(requested, passed, ctx.Now)
	if err != nil {
		return TodayResult{}, err
	}
	start, end := render.DayBounds(date)

	// Only the session is carried forward: the cache's stat fields are an implementation detail of
	// syncing, not part of a day's report.
	daily := []sessions.Session{}
	for _, session := range ctx.Sessions {
		if session.Prompts > 0 && overlaps(session.Session, start, end) {
			daily = append(daily, session.Session)
		}
	}
	slices.SortStableFunc(daily, byStart)

	return TodayResult{
		Date:     date,
		Sessions: daily,
		Projects: rollUpDay(daily),
		Items:    itemActivity(ctx, start, end),
	}, nil
}

// readDate reads --date when given, otherwise the day the command is being run on.
func readDate(input string, passed bool, now time.Time) (string, error) {
	if !passed {
		return render.LocalYmd(now), nil
	}

	value := strings.TrimSpace(input)
	// A syntactically valid date can still be impossible (`2026-02-31`); the round trip catches it.
	start, _ := render.DayBounds(value)
	if !ymdPattern.MatchString(value) || render.LocalYmd(start) != value {
		return "", errs.Userf("invalid --date: %s (expected YYYY-MM-DD)", input)
	}
	return value, nil
}

// overlaps reports whether any part of a session's span falls inside the half-open window. A
// session with only one of the two timestamps is treated as an instant at that time.
func overlaps(session sessions.Session, start, end time.Time) bool {
	started, ok := startedAt(session)
	ended, endOk := endedAt(session)
	if !ok || !endOk {
		return false
	}
	return started.Before(end) && !ended.Before(start)
}

func startedAt(session sessions.Session) (time.Time, bool) {
	return instant(session.Started, session.Ended)
}

func endedAt(session sessions.Session) (time.Time, bool) {
	return instant(session.Ended, session.Started)
}

// instant parses the first of the two timestamps that is present, since a session missing one end
// stands in for it with the other.
func instant(preferred, fallback *string) (time.Time, bool) {
	for _, candidate := range []*string{preferred, fallback} {
		if candidate == nil {
			continue
		}
		return render.ParseTime(*candidate)
	}
	return time.Time{}, false
}

// byStart orders sessions oldest first, treating an unparseable timestamp as equal to everything so
// an unsortable session keeps its place rather than moving one.
func byStart(a, b sessions.Session) int {
	left, leftOk := startedAt(a)
	right, rightOk := startedAt(b)
	if !leftOk || !rightOk {
		return 0
	}
	return left.Compare(right)
}

// projectKey folds a nullable project path into a map key, keeping the project-less group distinct
// from a project whose path is empty.
type projectKey struct {
	path    string
	present bool
}

func keyOf(path *string) projectKey {
	if path == nil {
		return projectKey{}
	}
	return projectKey{path: *path, present: true}
}

// rollUpDay puts the busiest project first, so the day reads as "where the time went"; ties fall
// back to name.
func rollUpDay(daily []sessions.Session) []DayProject {
	order := []projectKey{}
	groups := map[projectKey]DayProject{}

	for _, session := range daily {
		key := keyOf(session.Project)
		group, seen := groups[key]
		if !seen {
			group = DayProject{Project: session.Project}
			order = append(order, key)
		}
		group.Sessions++
		group.Prompts += session.Prompts
		groups[key] = group
	}

	rolled := make([]DayProject, 0, len(order))
	for _, key := range order {
		rolled = append(rolled, groups[key])
	}
	slices.SortStableFunc(rolled, func(a, b DayProject) int {
		if a.Prompts != b.Prompts {
			return b.Prompts - a.Prompts
		}
		return project.CompareNames(a.Project, b.Project)
	})
	return rolled
}

// itemActivity collects every item the day's events touched, read straight from the log so the day
// reflects when work happened rather than an item's current state. An item touched twice is still
// listed once.
func itemActivity(ctx *cli.Ctx, start, end time.Time) DayItems {
	state := events.Load(ctx.Cfg.EventsPath)
	activity := DayItems{Added: []events.Item{}, Closed: []events.Item{}, Noted: []events.Item{}}

	for _, record := range events.ReadRecords(ctx.Cfg.EventsPath) {
		id, hasId := record.Value["id"].(string)
		ts, hasTs := record.Value["ts"].(string)
		if !hasId || !hasTs {
			continue
		}

		at, parsed := render.ParseTime(ts)
		if !parsed || at.Before(start) || !at.Before(end) {
			continue
		}

		item, found := state.Find(id)
		bucket := bucketFor(&activity, record.Value["ev"])
		if !found || bucket == nil || slices.ContainsFunc(*bucket, sameItem(item)) {
			continue
		}
		*bucket = append(*bucket, item)
	}

	return activity
}

func bucketFor(activity *DayItems, ev any) *[]events.Item {
	switch ev {
	case "add":
		return &activity.Added
	case "close":
		return &activity.Closed
	case "note":
		return &activity.Noted
	}
	return nil
}

func sameItem(item events.Item) func(events.Item) bool {
	return func(candidate events.Item) bool { return candidate.Id == item.Id }
}

func renderToday(data TodayResult, ctx *cli.Ctx) string {
	type activityRow struct {
		label string
		item  events.Item
	}

	activity := []activityRow{}
	for _, item := range data.Items.Added {
		activity = append(activity, activityRow{"added", item})
	}
	for _, item := range data.Items.Closed {
		activity = append(activity, activityRow{"closed", item})
	}
	for _, item := range data.Items.Noted {
		activity = append(activity, activityRow{"noted", item})
	}

	if len(data.Sessions) == 0 && len(activity) == 0 {
		return "nothing recorded for " + data.Date
	}

	lines := []string{"TODAY  " + data.Date}

	for _, group := range data.Projects {
		lines = append(lines, "", "  "+projectHeading(group.Project)+"  "+summarizeDay(group))
		for _, session := range data.Sessions {
			if !sameProject(session.Project, group.Project) {
				continue
			}
			lines = append(lines, "    "+render.Pad(session.Title, sessionTitleWidth)+"  "+plural(session.Prompts, "prompt"))
		}
	}

	if len(activity) > 0 {
		lines = append(lines, "", "  ITEMS")
		for _, row := range activity {
			lines = append(lines, "    "+render.Pad(row.label, activityLabelWidth)+render.Pad(row.item.Id, idWidth)+"  "+row.item.Title)
		}
	}

	return strings.Join(lines, "\n")
}

func summarizeDay(group DayProject) string {
	return plural(group.Sessions, "session") + ", " + plural(group.Prompts, "prompt")
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// projectHeading names a group, standing in for the project-less one.
func projectHeading(path *string) string {
	if path == nil {
		return "(no project)"
	}
	return *path
}

func sameProject(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
