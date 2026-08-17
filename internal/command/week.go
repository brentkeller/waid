package command

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/sessions"
)

// WeekProject is one project's share of a week, carrying its session titles so the rollup reads as
// a work log.
type WeekProject struct {
	Project  *string `json:"project"`
	Sessions int     `json:"sessions"`
	Prompts  int     `json:"prompts"`
	// Titles are the distinct session titles, oldest first; a resumed session is not listed twice.
	Titles      []string `json:"titles"`
	ItemsClosed int      `json:"itemsClosed"`
}

// WeekTotals sums the week's groups.
type WeekTotals struct {
	Sessions int `json:"sessions"`
	Prompts  int `json:"prompts"`
	Closed   int `json:"closed"`
}

// WeekResult is one Monday-start week rolled up by project.
type WeekResult struct {
	// Start is the window's Monday, as `YYYY-MM-DD`.
	Start string `json:"start"`
	// End is the window's Sunday — the last day covered, not the exclusive bound.
	End string `json:"end"`
	// Projects touched by the week, busiest first.
	Projects []WeekProject `json:"projects"`
	Totals   WeekTotals    `json:"totals"`
}

func runWeek(ctx *cli.Ctx) (WeekResult, error) {
	offset := 0
	if ctx.Flags.Bool("last") {
		offset = -1
	}
	start, end := render.WeekBounds(ctx.Now, offset)

	weekly := []sessions.Session{}
	for _, session := range ctx.Sessions {
		if session.Prompts > 0 && overlaps(session.Session, start, end) {
			weekly = append(weekly, session.Session)
		}
	}
	slices.SortStableFunc(weekly, byStart)

	projects := rollUpWeek(weekly, closuresByProject(ctx, start, end))

	totals := WeekTotals{}
	for _, group := range projects {
		totals.Sessions += group.Sessions
		totals.Prompts += group.Prompts
		totals.Closed += group.ItemsClosed
	}

	return WeekResult{
		Start:    render.LocalYmd(start),
		End:      render.LocalYmd(end.Add(-time.Millisecond)),
		Projects: projects,
		Totals:   totals,
	}, nil
}

// closure is one project's closures inside the window, kept in first-seen order so a project that
// only closed an item still lands in a predictable place.
type closure struct {
	key     projectKey
	project *string
	count   int
}

// rollUpWeek puts the busiest project first; ties fall back to name. Projects that only closed an
// item still earn a group, so a week of finishing work does not read as an empty one.
func rollUpWeek(weekly []sessions.Session, closures []closure) []WeekProject {
	order := []projectKey{}
	groups := map[projectKey]WeekProject{}

	group := func(key projectKey, path *string) WeekProject {
		existing, seen := groups[key]
		if !seen {
			existing = WeekProject{Project: path, Titles: []string{}}
			order = append(order, key)
		}
		return existing
	}

	for _, session := range weekly {
		key := keyOf(session.Project)
		entry := group(key, session.Project)
		entry.Sessions++
		entry.Prompts += session.Prompts
		if !slices.Contains(entry.Titles, session.Title) {
			entry.Titles = append(entry.Titles, session.Title)
		}
		groups[key] = entry
	}

	for _, closed := range closures {
		entry := group(closed.key, closed.project)
		entry.ItemsClosed = closed.count
		groups[closed.key] = entry
	}

	rolled := make([]WeekProject, 0, len(order))
	for _, key := range order {
		rolled = append(rolled, groups[key])
	}
	slices.SortStableFunc(rolled, func(a, b WeekProject) int {
		if a.Prompts != b.Prompts {
			return b.Prompts - a.Prompts
		}
		return project.CompareNames(a.Project, b.Project)
	})
	return rolled
}

// closuresByProject counts how many items each project closed inside the window, read from the log
// so the count reflects when the work finished rather than an item's current state.
func closuresByProject(ctx *cli.Ctx, start, end time.Time) []closure {
	state := events.Load(ctx.Cfg.EventsPath)
	order := []projectKey{}
	counts := map[projectKey]closure{}
	counted := map[string]bool{}

	for _, record := range events.ReadRecords(ctx.Cfg.EventsPath) {
		id, hasId := record.Value["id"].(string)
		ts, hasTs := record.Value["ts"].(string)
		if record.Value["ev"] != "close" || !hasId || !hasTs {
			continue
		}

		at, parsed := render.ParseTime(ts)
		if !parsed || at.Before(start) || !at.Before(end) {
			continue
		}

		item, found := state.Find(id)
		// An item closed, reopened and closed again in one week is still one closure.
		if !found || counted[item.Id] {
			continue
		}
		counted[item.Id] = true

		key := keyOf(item.Project)
		entry, seen := counts[key]
		if !seen {
			entry = closure{key: key, project: item.Project}
			order = append(order, key)
		}
		entry.count++
		counts[key] = entry
	}

	closures := make([]closure, 0, len(order))
	for _, key := range order {
		closures = append(closures, counts[key])
	}
	return closures
}

func renderWeek(data WeekResult, ctx *cli.Ctx) string {
	if len(data.Projects) == 0 {
		return "nothing recorded for the week of " + data.Start
	}

	lines := []string{"WEEK  " + data.Start + " → " + data.End}

	for _, group := range data.Projects {
		heading := summarizeWeek(group.Sessions, group.Prompts, group.ItemsClosed)
		lines = append(lines, "", "  "+projectHeading(group.Project)+"  "+heading)
		for _, title := range group.Titles {
			lines = append(lines, "    "+title)
		}
	}

	totals := summarizeWeek(data.Totals.Sessions, data.Totals.Prompts, data.Totals.Closed)
	lines = append(lines, "", "  TOTAL  "+totals)
	return strings.Join(lines, "\n")
}

func summarizeWeek(sessionCount, prompts, closed int) string {
	return plural(sessionCount, "session") + ", " + plural(prompts, "prompt") +
		", " + strconv.Itoa(closed) + " closed"
}
