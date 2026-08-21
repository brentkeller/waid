package command_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
)

// listRecord is the --json payload list echoes back.
type listRecord struct {
	Items []struct {
		Id string `json:"id"`
	} `json:"items"`
	Filters struct {
		Status  *string  `json:"status"`
		Origin  *string  `json:"origin"`
		Tag     []string `json:"tag"`
		All     bool     `json:"all"`
	} `json:"filters"`
}

func (r listRecord) ids() []string {
	ids := make([]string, 0, len(r.Items))
	for _, item := range r.Items {
		ids = append(ids, item.Id)
	}
	return ids
}

func list(t *testing.T, home string, args ...string) listRecord {
	t.Helper()

	run := waid(t, home, append(append([]string{"list"}, args...), "--json")...)
	if run.code != cli.ExitOK {
		t.Fatalf("list exited %d: %s", run.code, run.err)
	}
	var record listRecord
	run.decode(t, &record)
	return record
}

// seed builds two projects and four items, one of them closed and one waiting on someone.
func seed(t *testing.T, home string) map[string]string {
	t.Helper()

	ids := map[string]string{
		"chart":   addItem(t, home, "Chart legend overflows", "-p", `C:\dev\dr\devresults`, "--tag", "bug"),
		"copy":    addItem(t, home, "Approve report copy", "-p", `C:\dev\dr\devresults`, "--waiting-on", "Dan"),
		"repo":    addItem(t, home, "Decide on the events repo", "-p", `C:\dev\waid`, "--tag", "design", "--tag", "bug"),
		"shipped": addItem(t, home, "Ship the thing", "-p", `C:\dev\waid`),
	}
	waid(t, home, "done", ids["shipped"])
	return ids
}

func assertIds(t *testing.T, got []string, want ...string) {
	t.Helper()

	sorted := slices.Clone(got)
	slices.Sort(sorted)
	slices.Sort(want)
	if !slices.Equal(sorted, want) {
		t.Errorf("ids = %v, want %v", sorted, want)
	}
}

func TestListHidesDoneItemsUntilAll(t *testing.T) {
	home := makeHome(t)
	ids := seed(t, home)

	visible := list(t, home)
	assertIds(t, visible.ids(), ids["chart"], ids["copy"], ids["repo"])
	if visible.Filters.All {
		t.Error("filters.all = true, want false")
	}

	all := list(t, home, "--all")
	if !slices.Contains(all.ids(), ids["shipped"]) {
		t.Errorf("--all ids = %v, missing the closed item", all.ids())
	}
	if !all.Filters.All {
		t.Error("filters.all = false, want true")
	}
}

func TestListStatusFiltersAndImpliesAll(t *testing.T) {
	home := makeHome(t)
	ids := seed(t, home)

	waiting := list(t, home, "--status", "waiting")
	assertIds(t, waiting.ids(), ids["copy"])
	if waiting.Filters.Status == nil || *waiting.Filters.Status != "waiting" {
		t.Errorf("filters.status = %v, want waiting", waiting.Filters.Status)
	}

	done := list(t, home, "--status", "done")
	assertIds(t, done.ids(), ids["shipped"])
}

func TestListRejectsAnUnknownStatus(t *testing.T) {
	home := makeHome(t)
	seed(t, home)

	run := waid(t, home, "list", "--status", "nope")
	if run.code != cli.ExitUser {
		t.Fatalf("code = %d, want %d", run.code, cli.ExitUser)
	}
	if !strings.Contains(run.err, "status") {
		t.Errorf("stderr = %q, want it to name the status", run.err)
	}
}

func TestListTagFiltersConjunctively(t *testing.T) {
	home := makeHome(t)
	ids := seed(t, home)

	bugs := list(t, home, "--tag", "bug")
	assertIds(t, bugs.ids(), ids["chart"], ids["repo"])
	if !slices.Equal(bugs.Filters.Tag, []string{"bug"}) {
		t.Errorf("filters.tag = %v, want [bug]", bugs.Filters.Tag)
	}

	both := list(t, home, "--tag", "bug", "--tag", "design")
	assertIds(t, both.ids(), ids["repo"])
}

func TestListWithoutTagsReportsAnEmptyTagFilter(t *testing.T) {
	home := makeHome(t)
	seed(t, home)

	run := waid(t, home, "list", "--json")
	if !strings.Contains(run.out, `"tag": []`) {
		t.Errorf("stdout does not carry an empty tag filter:\n%s", run.out)
	}
}

func TestListResolvesAPartialOrigin(t *testing.T) {
	home := makeHome(t)
	ids := seed(t, home)

	result := list(t, home, "--origin", "waid")
	assertIds(t, result.ids(), ids["repo"])
	if result.Filters.Origin == nil || *result.Filters.Origin != `C:\dev\waid` {
		t.Errorf("filters.origin = %v, want C:\\dev\\waid", result.Filters.Origin)
	}
}

func TestListCombinesFiltersConjunctively(t *testing.T) {
	home := makeHome(t)
	ids := seed(t, home)

	result := list(t, home, "--origin", "devresults", "--tag", "bug")
	assertIds(t, result.ids(), ids["chart"])
}

func TestListGroupsByProject(t *testing.T) {
	home := makeHome(t)
	ids := seed(t, home)
	addItem(t, home, "Loose end with no project")

	run := waid(t, home, "list")
	if run.code != cli.ExitOK {
		t.Fatalf("list exited %d: %s", run.code, run.err)
	}

	headings := []string{}
	for _, line := range strings.Split(run.out, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			headings = append(headings, strings.TrimSpace(line))
		}
	}
	want := []string{`C:\dev\dr\devresults`, `C:\dev\waid`, "(no project)"}
	if !slices.Equal(headings, want) {
		t.Errorf("headings = %v, want %v", headings, want)
	}

	line := lineHolding(run.out, ids["chart"])
	if line == "" {
		t.Fatalf("no line for the chart item:\n%s", run.out)
	}
	if !strings.HasPrefix(line, "    ") {
		t.Errorf("item line is not indented under its heading: %q", line)
	}
	for _, part := range []string{"open", "Chart legend overflows", "[bug]"} {
		if !strings.Contains(line, part) {
			t.Errorf("item line %q does not carry %q", line, part)
		}
	}
}

func TestListRendersAnExplicitEmptyMessage(t *testing.T) {
	home := makeHome(t)

	run := waid(t, home, "list")
	if run.code != cli.ExitOK {
		t.Fatalf("list exited %d: %s", run.code, run.err)
	}
	if !regexp.MustCompile(`(?i)no items`).MatchString(run.out) {
		t.Errorf("stdout = %q, want it to say there are no items", run.out)
	}
}

// lineHolding returns the first line of text carrying needle, or the empty string.
func lineHolding(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if needle != "" && strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

// --project was the path filter before origins existed. It is refused rather than ignored, so an
// invocation written against the old flag does not quietly list everything.
func TestListRefusesTheRetiredProjectFlag(t *testing.T) {
	home := makeHome(t)
	seed(t, home)

	run := waid(t, home, "list", "--project", "waid")
	if run.code != cli.ExitUser {
		t.Fatalf("code = %d, want %d", run.code, cli.ExitUser)
	}
	if !strings.Contains(run.err, "--origin") {
		t.Errorf("stderr = %q, want it to name --origin", run.err)
	}
}

// The origin filter is a path filter, so it composes with --status and --tag exactly as the project
// filter did, and reads a full path as readily as a fragment.
func TestListOriginTakesAWholePathAndComposes(t *testing.T) {
	home := makeHome(t)
	ids := seed(t, home)

	whole := list(t, home, "--origin", `C:\dev\dr\devresults`)
	assertIds(t, whole.ids(), ids["chart"], ids["copy"])

	waiting := list(t, home, "--origin", "devresults", "--status", "waiting")
	assertIds(t, waiting.ids(), ids["copy"])
}
