package tree

import (
	"fmt"
	"strings"

	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
)

// Target is what a -p value named: the item to file under, or the path the work came from, never
// both. A Target holding neither is a -p that was not passed.
//
// The two are exclusive because a path is no longer a place in the tree. It is provenance — where
// the work came from — so an item given one sits at the top level with the path recorded beside it,
// which is where promotion has always put it.
type Target struct {
	Parent *string `json:"parent"`
	Origin *string `json:"origin"`
}

// Resolve reads a -p value: nothing yields neither field, `.` and an absolute path yield an origin,
// and anything else is a case-insensitive substring of an item's title yielding that item's id as a
// parent. A fragment matching no item, or more than one, is a errs.UserError; the ambiguous case
// carries the matches so the caller can print them.
//
// Every item the log holds is a candidate, closed ones included, since a fragment naming a heading
// should resolve the same way whichever filter the caller happens to be rendering under.
func Resolve(input string, state events.State, cwd string) (Target, error) {
	value := strings.TrimSpace(input)
	switch {
	case value == "":
		return Target{}, nil
	case value == ".":
		return origin(project.NormalizePath(cwd)), nil
	case project.IsAbsolute(value):
		return origin(project.NormalizePath(value)), nil
	}

	needle := strings.ToLower(value)
	matches := []events.Item{}
	for _, item := range state.Items {
		if strings.Contains(strings.ToLower(item.Title), needle) {
			matches = append(matches, item)
		}
	}

	switch len(matches) {
	case 1:
		return Target{Parent: &matches[0].Id}, nil
	case 0:
		return Target{}, errs.Userf("no item matches: %s", value)
	default:
		return Target{}, errs.Ambiguous("ambiguous parent: "+value, labels(matches))
	}
}

// ResolveId reads a --parent value: an item's id, taken literally and never matched against titles,
// so it names one item however many share a title. An id no item holds is a errs.UserError.
func ResolveId(input string, state events.State) (Target, error) {
	id := strings.TrimSpace(input)
	item, found := state.Find(id)
	if !found {
		return Target{}, errs.Userf("unknown item id: %s", id)
	}
	return Target{Parent: &item.Id}, nil
}

// labels names the ambiguous matches by id and title, since two items may share a title and the id
// is what tells them apart.
func labels(items []events.Item) []string {
	listed := make([]string, 0, len(items))
	for _, item := range items {
		listed = append(listed, fmt.Sprintf("%s  %s", item.Id, item.Title))
	}
	return listed
}

func origin(path string) Target { return Target{Origin: &path} }
