package command_test

import (
	"fmt"
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
		Status *string  `json:"status"`
		Origin *string  `json:"origin"`
		Tag    []string `json:"tag"`
		All    bool     `json:"all"`
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

// outline reduces the rendered tree to one "<depth> <label>" per row — an id for a real item, the
// title for the synthetic bucket — so a test states the nesting it expects rather than matching
// against columns whose widths are not what is under test.
func outline(t *testing.T, home string, args ...string) []string {
	t.Helper()

	run := waid(t, home, append([]string{"list"}, args...)...)
	if run.code != cli.ExitOK {
		t.Fatalf("list exited %d: %s", run.code, run.err)
	}

	rows := []string{}
	for _, line := range strings.Split(run.out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		depth := (len(line)-len(strings.TrimLeft(line, " ")))/2 - 1
		rows = append(rows, fmt.Sprintf("%d %s", depth, strings.Fields(trimmed)[0]))
	}
	return rows
}

func assertOutline(t *testing.T, got []string, want ...string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Errorf("outline =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestListNestsItemsUnderTheirParents(t *testing.T) {
	home := makeHome(t)
	area := addItem(t, home, "DevResults")
	proj := addItem(t, home, "Search rewrite", "-p", "DevResults")
	task := addItem(t, home, "Reindex script", "-p", "Search rewrite")

	assertOutline(t, outline(t, home), "0 "+area, "1 "+proj, "2 "+task)
}

// The bucket is a rendering artifact of a level that mixes headings with loose rows. A level of
// nothing but leaves keeps them where they are, at every depth including the top.
func TestListBucketsLeavesOnlyWhereALevelMixes(t *testing.T) {
	home := makeHome(t)
	area := addItem(t, home, "DevResults")
	proj := addItem(t, home, "Search rewrite", "-p", "DevResults")
	task := addItem(t, home, "Reindex script", "-p", "Search rewrite")
	loose := addItem(t, home, "Renew SSL cert", "-p", "DevResults")
	flights := addItem(t, home, "Book flights")

	assertOutline(t, outline(t, home),
		"0 "+area,
		"1 "+proj,
		"2 "+task,
		"1 (unassigned)",
		"2 "+loose,
		"0 (unassigned)",
		"1 "+flights,
	)
}

func TestListOrdersParentsByTitleThenLeavesByRecency(t *testing.T) {
	home := makeHome(t)
	at := func(minute string) { t.Setenv(cli.EnvNow, "2026-08-14T09:"+minute+":00Z") }

	at("01")
	zebra := addItem(t, home, "Zebra area")
	at("02")
	alpha := addItem(t, home, "Alpha area")
	at("03")
	zebraKid := addItem(t, home, "Zebra child", "-p", "Zebra area")
	at("04")
	alphaKid := addItem(t, home, "Alpha child", "-p", "Alpha area")
	at("05")
	older := addItem(t, home, "Older loose end")
	at("06")
	newer := addItem(t, home, "Newer loose end")

	assertOutline(t, outline(t, home),
		"0 "+alpha,
		"1 "+alphaKid,
		"0 "+zebra,
		"1 "+zebraKid,
		"0 (unassigned)",
		"1 "+newer,
		"1 "+older,
	)
}

func TestListKeepsTheAncestorsOfAFilteredMatch(t *testing.T) {
	home := makeHome(t)
	area := addItem(t, home, "DevResults")
	proj := addItem(t, home, "Search rewrite", "-p", "DevResults")
	task := addItem(t, home, "Reindex script", "-p", "Search rewrite", "--tag", "bug")
	addItem(t, home, "Unrelated loose end")

	assertOutline(t, outline(t, home, "--tag", "bug"), "0 "+area, "1 "+proj, "2 "+task)
}

// The status gate is asked of every row in its own right, so a closed row does not ride into the
// listing on an open parent — and a parent it strips bare renders as a leaf, into the bucket.
func TestListDropsDoneRowsBeneathAnOpenParent(t *testing.T) {
	home := makeHome(t)
	at := func(minute string) { t.Setenv(cli.EnvNow, "2026-08-14T09:"+minute+":00Z") }

	at("01")
	area := addItem(t, home, "DevResults")
	at("02")
	proj := addItem(t, home, "Search rewrite", "-p", "DevResults")
	at("03")
	shipped := addItem(t, home, "Reindex script", "-p", "Search rewrite")
	at("04")
	loose := addItem(t, home, "Renew SSL cert", "-p", "DevResults")
	at("05")
	imports := addItem(t, home, "Bulk imports", "-p", "DevResults")
	at("06")
	backoff := addItem(t, home, "Add retry backoff", "-p", "Bulk imports")
	at("07")
	waid(t, home, "done", shipped)

	assertOutline(t, outline(t, home),
		"0 "+area,
		"1 "+imports,
		"2 "+backoff,
		"1 (unassigned)",
		"2 "+loose,
		"2 "+proj,
	)
}

// Item rows keep the columns they had before the tree: the tree changed where a row sits, not what
// it says.
func TestListRowsCarryTheirColumns(t *testing.T) {
	home := makeHome(t)
	ids := seed(t, home)

	run := waid(t, home, "list")
	if run.code != cli.ExitOK {
		t.Fatalf("list exited %d: %s", run.code, run.err)
	}
	line := lineHolding(run.out, ids["chart"])
	if line == "" {
		t.Fatalf("no line for the chart item:\n%s", run.out)
	}
	for _, part := range []string{"open", "Chart legend overflows", "[bug]"} {
		if !strings.Contains(line, part) {
			t.Errorf("item line %q does not carry %q", line, part)
		}
	}
}
