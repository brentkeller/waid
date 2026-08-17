// Package project resolves the -p value against the project paths waid has already seen, and
// groups items by project for display.
package project

import (
	"regexp"
	"slices"
	"strings"

	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// bareRoot matches a bare filesystem root — `/`, `C:` or `C:\` — where the trailing separator
// carries meaning.
var bareRoot = regexp.MustCompile(`^(?:[\\/]+|[A-Za-z]:[\\/]*)$`)

// absolute matches a Windows drive-letter or POSIX absolute path.
var absolute = regexp.MustCompile(`^(?:[\\/]|[A-Za-z]:[\\/])`)

// trailingSeparators matches the separators at the end of a path.
var trailingSeparators = regexp.MustCompile(`[\\/]+$`)

// Group is the items belonging to one project, or to no project when Project is nil.
type Group struct {
	Project *string       `json:"project"`
	Items   []events.Item `json:"items"`
}

// NormalizePath trims a project path and drops trailing separators, leaving bare roots intact.
func NormalizePath(p string) string {
	trimmed := strings.TrimSpace(p)
	if bareRoot.MatchString(trimmed) {
		return trimmed
	}
	return trailingSeparators.ReplaceAllString(trimmed, "")
}

// KnownProjects returns every project path waid has seen, in first-seen order: items first, then
// sessions. Empty paths are dropped.
func KnownProjects(items []events.Item, sessionProjects []string) []string {
	known := []string{}
	add := func(project string) {
		if project == "" || slices.Contains(known, project) {
			return
		}
		known = append(known, project)
	}

	for _, item := range items {
		if item.Project != nil {
			add(*item.Project)
		}
	}
	for _, project := range sessionProjects {
		add(project)
	}
	return known
}

// Resolve resolves a -p value: nothing → no project, `.` → the current directory, an absolute path
// taken as given, anything else a case-insensitive substring of a known project path. A partial
// matching no known project, or more than one, is a errs.UserError.
func Resolve(input string, known []string, cwd string) (*string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return nil, nil
	}
	if value == "." {
		return ptr(NormalizePath(cwd)), nil
	}
	if absolute.MatchString(value) {
		return ptr(NormalizePath(value)), nil
	}

	needle := strings.ToLower(value)
	matches := []string{}
	for _, project := range known {
		if strings.Contains(strings.ToLower(project), needle) {
			matches = append(matches, project)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, errs.Userf("no known project matches: %s", value)
	default:
		return nil, errs.Ambiguous("ambiguous project: "+value, matches)
	}
}

// GroupByProject groups items by project in first-seen order, with the project-less group last.
// Item order inside each group is the order given.
func GroupByProject(items []events.Item) []Group {
	order := []string{}
	byProject := map[string][]events.Item{}
	orphans := []events.Item{}

	for _, item := range items {
		if item.Project == nil {
			orphans = append(orphans, item)
			continue
		}
		if _, seen := byProject[*item.Project]; !seen {
			order = append(order, *item.Project)
		}
		byProject[*item.Project] = append(byProject[*item.Project], item)
	}

	groups := make([]Group, 0, len(order)+1)
	for _, project := range order {
		groups = append(groups, Group{Project: ptr(project), Items: byProject[project]})
	}
	if len(orphans) > 0 {
		groups = append(groups, Group{Project: nil, Items: orphans})
	}
	return groups
}

func ptr(value string) *string { return &value }
