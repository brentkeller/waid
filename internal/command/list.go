package command

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
	"github.com/brentkeller/waid/internal/render"
)

// ListFilters are the filters that produced a list, echoed back so --json callers can see what was
// applied.
type ListFilters struct {
	Status  *events.Status `json:"status"`
	Project *string        `json:"project"`
	Tag     []string       `json:"tag"`
	All     bool           `json:"all"`
}

// ListResult is the filtered items alongside the filters that selected them.
type ListResult struct {
	Items   []events.Item `json:"items"`
	Filters ListFilters   `json:"filters"`
}

// Column widths for the shared item row; titles beyond the width are ellipsized, not wrapped.
const (
	idWidth     = 4
	statusWidth = 10
	titleWidth  = 44
	metaWidth   = 10
)

func runList(ctx *cli.Ctx) (ListResult, error) {
	state := events.Load(ctx.Cfg.EventsPath)

	status, err := readStatus(ctx.Flags.String("status"))
	if err != nil {
		return ListResult{}, err
	}
	requested, _ := ctx.Flags.String("project")
	path, err := project.Resolve(requested, project.KnownProjects(state.Items, nil), ctx.Cwd)
	if err != nil {
		return ListResult{}, err
	}

	tags := ctx.Flags.List("tag")
	if tags == nil {
		tags = []string{}
	}
	filters := ListFilters{Status: status, Project: path, Tag: tags, All: ctx.Flags.Bool("all")}

	items := []events.Item{}
	for _, item := range state.Items {
		if matches(item, filters) {
			items = append(items, item)
		}
	}
	slices.SortStableFunc(items, func(a, b events.Item) int {
		return strings.Compare(a.Updated, b.Updated)
	})

	return ListResult{Items: items, Filters: filters}, nil
}

// matches reports whether an item survives every filter. Filters are conjunctive; an explicit
// --status speaks for itself, so it overrides the done gate.
func matches(item events.Item, filters ListFilters) bool {
	if filters.Status != nil {
		if item.Status != *filters.Status {
			return false
		}
	} else if !filters.All && item.Status == events.StatusDone {
		return false
	}
	if filters.Project != nil && (item.Project == nil || *item.Project != *filters.Project) {
		return false
	}
	for _, tag := range filters.Tag {
		if !slices.Contains(item.Tags, tag) {
			return false
		}
	}
	return true
}

// readStatus narrows the --status value, which an unpassed flag leaves absent rather than empty.
func readStatus(value string, passed bool) (*events.Status, error) {
	if !passed {
		return nil, nil
	}
	if !events.IsStatus(value) {
		names := make([]string, 0, len(events.Statuses))
		for _, status := range events.Statuses {
			names = append(names, string(status))
		}
		return nil, errs.Userf("unknown status: %s (expected %s)", value, strings.Join(names, ", "))
	}
	status := events.Status(value)
	return &status, nil
}

func renderList(data ListResult, ctx *cli.Ctx) string {
	if len(data.Items) == 0 {
		return "no items"
	}

	lines := []string{}
	for _, group := range project.GroupByProject(data.Items) {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		heading := "(no project)"
		if group.Project != nil {
			heading = *group.Project
		}
		lines = append(lines, "  "+heading)
		for _, item := range group.Items {
			lines = append(lines, ItemLine(item, ctx.Now))
		}
	}
	return strings.Join(lines, "\n")
}

// ItemLine renders one item as an aligned row, indented under its project heading. Shared with
// waid loops.
func ItemLine(item events.Item, now time.Time) string {
	parts := []string{}
	if len(item.Tags) > 0 {
		parts = append(parts, "["+strings.Join(item.Tags, ",")+"]")
	}
	if item.WaitingOn != nil {
		parts = append(parts, "← "+*item.WaitingOn)
	}

	row := render.Pad(item.Id, idWidth) +
		"  " +
		render.Pad(string(item.Status), statusWidth) +
		render.Pad(item.Title, titleWidth) +
		"  " +
		render.Pad(strings.Join(parts, " "), metaWidth) +
		render.RelTime(item.Updated, now)

	return strings.TrimRightFunc("    "+row, unicode.IsSpace)
}
