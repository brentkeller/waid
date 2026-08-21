// Package project resolves the -p value against the project paths waid has already seen.
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

// CompareNames orders two project paths the way Node's localeCompare does for the paths waid
// stores: case is ignored first, so `C:\Dev` and `C:\dev` sit together rather than a drive letter's
// case splitting a listing, and only an otherwise exact tie falls back to a byte comparison. The
// project-less group sorts as the empty name, which is where Node's null coalescing puts it.
func CompareNames(left, right *string) int {
	a, b := derefOr(left, ""), derefOr(right, "")
	if folded := strings.Compare(strings.ToLower(a), strings.ToLower(b)); folded != 0 {
		return folded
	}
	return strings.Compare(a, b)
}

func derefOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
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
		if item.Origin != nil {
			add(*item.Origin)
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

func ptr(value string) *string { return &value }

// IsAbsolute reports whether a value is shaped like a path rather than a name: a Windows
// drive-letter or a POSIX absolute path.
func IsAbsolute(value string) bool { return absolute.MatchString(strings.TrimSpace(value)) }
