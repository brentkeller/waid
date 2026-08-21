package command_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/gh"
)

// loopsRecord is the --json payload loops prints.
type loopsRecord struct {
	Detected       []signalRecord `json:"detected"`
	DismissedCount int            `json:"dismissedCount"`
	Notes          []string       `json:"notes"`
	Items          []struct {
		Id        string  `json:"id"`
		Title     string  `json:"title"`
		Status    string  `json:"status"`
		Origin    *string `json:"origin"`
		Parent    *string `json:"parent"`
		WaitingOn *string `json:"waitingOn"`
	} `json:"items"`
}

// itemIds are every declared item, in the order loops listed them.
func (r loopsRecord) itemIds() []string {
	ids := []string{}
	for _, item := range r.Items {
		ids = append(ids, item.Id)
	}
	return ids
}

// quiet is a detection pair that finds nothing, so a test stays about declared items only.
var quiet = cli.Seams{Git: fakeGit{}, Gh: fakeGh{}}

// runLoops runs loops with the detection seams supplied; --no-sync keeps the real ~/.claude out of
// the test.
func runLoops(t *testing.T, home string, seams cli.Seams, args ...string) result {
	t.Helper()

	return waidWith(t, home, seams, append([]string{"loops"}, append(args, "--no-sync")...)...)
}

func loopsJson(t *testing.T, home string, seams cli.Seams, args ...string) (result, loopsRecord) {
	t.Helper()

	run := runLoops(t, home, seams, append(args, "--json")...)
	var record loopsRecord
	run.decode(t, &record)
	return run, record
}

// loopsHome is a home with no scan roots, so detection has no repo to look at.
func loopsHome(t *testing.T) string {
	t.Helper()

	return detectHome(t, filepath.Join(t.TempDir(), "nowhere"))
}

// seedLoops writes two projects plus an orphan and one closed item, spread across five days. The log
// is written directly so the updated timestamps — and therefore the ordering — are deterministic.
func seedLoops(t *testing.T, home string) {
	t.Helper()

	seedLog(t, home,
		map[string]any{"ts": "2026-08-08T09:00:00.000Z", "ev": "add", "id": "q8xt", "title": "Loose end with no project"},
		map[string]any{
			"ts": "2026-08-09T09:00:00.000Z", "ev": "add", "id": "k3f9",
			"title": "Chart legend overflows at 4+ series", "project": drProject, "tags": []string{"bug"},
		},
		map[string]any{"ts": "2026-08-10T09:00:00.000Z", "ev": "add", "id": "zz11", "title": "Ship the thing", "project": waidProject},
		map[string]any{"ts": "2026-08-11T09:00:00.000Z", "ev": "close", "id": "zz11"},
		map[string]any{
			"ts": "2026-08-12T09:00:00.000Z", "ev": "add", "id": "m7qz", "title": "Approve report-template copy",
			"project": drProject, "status": "waiting", "waitingOn": "Dan",
		},
		map[string]any{
			"ts": "2026-08-13T09:00:00.000Z", "ev": "add", "id": "p2vn",
			"title": "Decide whether events.jsonl gets its own repo", "project": waidProject,
		},
	)
}

func TestLoopsExcludesDoneItems(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet)
	for _, id := range record.itemIds() {
		if id == "zz11" {
			t.Fatalf("a closed item appeared in loops: %v", record.itemIds())
		}
	}
	assertKeys(t, record.itemIds(), []string{"q8xt", "k3f9", "m7qz", "p2vn"})
}

func TestLoopsKeepsWaitingItemsAlongsideOpenOnes(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet)
	for _, item := range record.Items {
		if item.Id != "m7qz" {
			continue
		}
		if item.Status != "waiting" {
			t.Errorf("status is %q, want waiting", item.Status)
		}
		if item.WaitingOn == nil || *item.WaitingOn != "Dan" {
			t.Errorf("waitingOn is %v, want Dan", item.WaitingOn)
		}
		return
	}
	t.Fatal("the waiting item is missing")
}

// The recorded path is provenance now, not a grouping key: it rides along on each item rather than
// gathering them into sections.
func TestLoopsCarriesTheRecordedOriginOnTheItem(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet)
	origins := map[string]string{}
	for _, item := range record.Items {
		if item.Origin != nil {
			origins[item.Id] = *item.Origin
		}
	}
	if origins["k3f9"] != drProject || origins["p2vn"] != waidProject {
		t.Errorf("origins are %v", origins)
	}
	if _, recorded := origins["q8xt"]; recorded {
		t.Errorf("the loose end reported an origin: %v", origins)
	}
}

func TestLoopsFiltersByRecordedOrigin(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet, "--origin", "waid")
	assertKeys(t, record.itemIds(), []string{"p2vn"})
}

func TestLoopsReservesTheDetectionFieldsWithNothingDetected(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet)
	if len(record.Detected) != 0 {
		t.Errorf("detected is %v, want empty", signalKeys(record.Detected))
	}
	if record.DismissedCount != 0 {
		t.Errorf("dismissed count is %d, want 0", record.DismissedCount)
	}
	if len(record.Notes) != 0 {
		t.Errorf("notes are %v, want none", record.Notes)
	}
}

func TestLoopsRendersNoOpenLoopsForAnEmptyState(t *testing.T) {
	run := runLoops(t, loopsHome(t), quiet)
	if run.code != cli.ExitOK {
		t.Fatalf("loops exited %d: %s", run.code, run.err)
	}
	assertMatches(t, run.out, `(?i)no open loops`)
}

func TestLoopsRendersNoOpenLoopsWhenEverythingIsClosed(t *testing.T) {
	home := loopsHome(t)
	seedLog(t, home,
		map[string]any{"ts": "2026-08-10T09:00:00.000Z", "ev": "add", "id": "zz11", "title": "Ship the thing"},
		map[string]any{"ts": "2026-08-11T09:00:00.000Z", "ev": "close", "id": "zz11"},
	)

	run := runLoops(t, home, quiet)
	if run.code != cli.ExitOK {
		t.Fatalf("loops exited %d: %s", run.code, run.err)
	}
	assertMatches(t, run.out, `(?i)no open loops`)
}

func TestHumanLoopsMatchesTheSpecLayout(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	run := runLoops(t, home, quiet)
	if run.code != cli.ExitOK {
		t.Fatalf("loops exited %d: %s", run.code, run.err)
	}

	lines := strings.Split(run.out, "\n")
	if lines[0] != "OPEN LOOPS" {
		t.Errorf("header is %q", lines[0])
	}
	if lines[1] != "" {
		t.Errorf("line 2 is %q, want blank", lines[1])
	}

	chart := lineContaining(t, lines, "k3f9")
	assertMatches(t, chart, `^ {2}k3f9 {2}open {6}Chart legend overflows at 4\+ series`)
	assertMatches(t, chart, `\[bug]`)
	assertMatches(t, chart, `\d+[mhdwy]$`)

	approval := lineContaining(t, lines, "m7qz")
	assertMatches(t, approval, `waiting`)
	assertMatches(t, approval, `← Dan`)
}

// loopsOutline reduces the declared half of the rendering to one "<depth> <label>" per row — an id
// for a real item, the title for the synthetic bucket — so a test states the nesting it expects
// rather than matching against column widths that are not what is under test.
func loopsOutline(t *testing.T, home string, seams cli.Seams, args ...string) []string {
	t.Helper()

	run := runLoops(t, home, seams, args...)
	if run.code != cli.ExitOK {
		t.Fatalf("loops exited %d: %s", run.code, run.err)
	}

	rows := []string{}
	for _, line := range strings.Split(run.out, "\n") {
		if strings.HasPrefix(line, "DETECTED") {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "OPEN LOOPS" {
			continue
		}
		depth := (len(line)-len(strings.TrimLeft(line, " ")))/2 - 1
		rows = append(rows, fmt.Sprintf("%d %s", depth, strings.Fields(trimmed)[0]))
	}
	return rows
}

func TestLoopsNestsItemsUnderTheirParents(t *testing.T) {
	home := loopsHome(t)
	area := addItem(t, home, "DevResults")
	proj := addItem(t, home, "Search rewrite", "-p", "DevResults")
	task := addItem(t, home, "Reindex script", "-p", "Search rewrite")
	loose := addItem(t, home, "Renew SSL cert", "-p", "DevResults")
	flights := addItem(t, home, "Book flights")

	assertOutline(t, loopsOutline(t, home, quiet),
		"0 "+area,
		"1 "+proj,
		"2 "+task,
		"1 (unassigned)",
		"2 "+loose,
		"0 (unassigned)",
		"1 "+flights,
	)
}

// -p names a heading and gets what it holds: the ancestors above it and the branches beside it are
// not the question that was asked.
func TestLoopsNarrowsToTheSubtreeUnderAParent(t *testing.T) {
	home := loopsHome(t)
	addItem(t, home, "DevResults")
	proj := addItem(t, home, "Search rewrite", "-p", "DevResults")
	task := addItem(t, home, "Reindex script", "-p", "Search rewrite")
	addItem(t, home, "Bulk imports", "-p", "DevResults")
	addItem(t, home, "Book flights")

	assertOutline(t, loopsOutline(t, home, quiet, "-p", "Search rewrite"), "0 "+proj, "1 "+task)
}

// -p still reads a path as provenance, as it does everywhere else, so the `waid loops -p .` form
// agents learned keeps answering the question --origin now spells out.
func TestLoopsReadsAPathGivenToPAsAnOrigin(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet, "-p", waidProject)
	assertKeys(t, record.itemIds(), []string{"p2vn"})
}

func TestLoopsRejectsAParentFragmentMatchingNoItem(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	run := runLoops(t, home, quiet, "-p", "nothing-by-that-name")
	if run.code != cli.ExitUser {
		t.Fatalf("loops exited %d, want %d: %s", run.code, cli.ExitUser, run.err)
	}
	assertMatches(t, run.err, `no item matches`)
}

func TestLoopsJsonCarriesDeclaredItemsAndDetectedSignalsSeparately(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "inl-prt")
	home := detectHome(t, root)
	seedLog(t, home,
		map[string]any{"ts": "2026-08-09T09:00:00.000Z", "ev": "add", "id": "k3f9", "title": "Chart legend", "project": repo},
		map[string]any{"ts": "2026-08-10T09:00:00.000Z", "ev": "dismiss", "key": "pr:acme/web#456"},
	)

	run, record := loopsJson(t, home, cli.Seams{
		Git: fakeGit{repo: {branch: ptr("inl-prt-fixes"), dirty: 14}},
		Gh:  fakeGh{review: []gh.Pr{pr("acme/web", 123, nil)}, authored: []gh.Pr{pr("acme/web", 456, nil)}},
	})

	if run.code != cli.ExitOK {
		t.Fatalf("loops exited %d: %s", run.code, run.err)
	}
	assertKeys(t, record.itemIds(), []string{"k3f9"})
	assertKeys(t, signalKeys(record.Detected), []string{"review:acme/web#123", "dirty:" + repo})
	if record.DismissedCount != 1 {
		t.Errorf("dismissed count is %d, want 1", record.DismissedCount)
	}
	if len(record.Notes) != 0 {
		t.Errorf("notes are %v, want none", record.Notes)
	}
}

func TestHumanLoopsRendersDetectedBeneathTheDeclaredItems(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "inl-prt")
	home := detectHome(t, root)
	seedLog(t, home,
		map[string]any{"ts": "2026-08-09T09:00:00.000Z", "ev": "add", "id": "k3f9", "title": "Chart legend", "project": repo},
		map[string]any{"ts": "2026-08-10T09:00:00.000Z", "ev": "dismiss", "key": "pr:acme/web#456"},
	)

	run := runLoops(t, home, cli.Seams{
		Git: fakeGit{repo: {branch: ptr("inl-prt-fixes"), dirty: 14}},
		Gh: fakeGh{
			review: []gh.Pr{pr("acme/web", 123, func(item *gh.Pr) {
				item.Title = "Fix the chart legend"
				item.Branch = "chart-legend"
			})},
			authored: []gh.Pr{pr("acme/web", 456, nil)},
		},
	})

	if run.code != cli.ExitOK {
		t.Fatalf("loops exited %d: %s", run.code, run.err)
	}
	lines := strings.Split(run.out, "\n")
	if lines[0] != "OPEN LOOPS" {
		t.Errorf("header is %q", lines[0])
	}

	header := indexOfPrefix(lines, "DETECTED")
	if header <= 0 {
		t.Fatalf("no DETECTED header in:\n%s", run.out)
	}
	if lines[header] != "DETECTED  (2 shown, 1 dismissed)" {
		t.Errorf("header is %q", lines[header])
	}
	if lines[header-1] != "" {
		t.Errorf("the DETECTED section is not separated from the items: %q", lines[header-1])
	}
	if lines[header+1] != "" {
		t.Errorf("line after the header is %q, want blank", lines[header+1])
	}

	// The declared item stays above the header; the signals stay below it.
	if item := indexOfContaining(lines, "k3f9"); item <= 0 || item >= header {
		t.Errorf("the declared item is at line %d, want above the header at %d", item, header)
	}
	assertMatches(t, lines[header+2], `^ {2}review:acme/web#123 +Fix the chart legend +@tmoore · chart-legend · 3d$`)
	// The HEAD date is relative to when the fake was built, so the age is a shape, not a value.
	assertMatches(t, lines[header+3], `^ {2}dirty:`+regexp.QuoteMeta(repo)+` +14 uncommitted files +inl-prt-fixes · \d+[hd]$`)
	assertMatches(t, run.out, `waid promote <key> to track · waid dismiss <key> to hide`)
}

// --origin is a question about a path, and both halves of the view answer it: the declared items by
// the origin they recorded, the signals by the repo they were found in. It resolves against the
// repos detection just found as well as the paths already logged, which is why a repo waid holds no
// item for is still a legitimate target.
func TestLoopsNarrowsDeclaredItemsAndDetectedSignalsTogether(t *testing.T) {
	root := t.TempDir()
	alpha := makeRepo(t, root, "alpha")
	beta := makeRepo(t, root, "beta")
	home := detectHome(t, root)
	seedLog(t, home,
		map[string]any{"ts": "2026-08-09T09:00:00.000Z", "ev": "add", "id": "k3f9", "title": "In alpha", "project": alpha},
		map[string]any{"ts": "2026-08-10T09:00:00.000Z", "ev": "add", "id": "m7qz", "title": "In beta", "project": beta},
	)

	_, record := loopsJson(t, home, cli.Seams{
		Git: fakeGit{alpha: {branch: ptr("main"), dirty: 2}, beta: {branch: ptr("main"), dirty: 3}},
		Gh:  fakeGh{},
	}, "--origin", "beta")

	assertKeys(t, record.itemIds(), []string{"m7qz"})
	assertKeys(t, signalKeys(record.Detected), []string{"dirty:" + beta})
}

// -p asks about the item tree, which detection knows nothing about, so the signals are left whole.
func TestLoopsLeavesDetectedSignalsAloneWhenNarrowingToASubtree(t *testing.T) {
	root := t.TempDir()
	alpha := makeRepo(t, root, "alpha")
	beta := makeRepo(t, root, "beta")
	home := detectHome(t, root)
	addItem(t, home, "Search rewrite")
	addItem(t, home, "Reindex script", "-p", "Search rewrite")

	_, record := loopsJson(t, home, cli.Seams{
		Git: fakeGit{alpha: {branch: ptr("main"), dirty: 2}, beta: {branch: ptr("main"), dirty: 3}},
		Gh:  fakeGh{},
	}, "-p", "Search rewrite")

	if len(record.Detected) != 2 {
		t.Errorf("detected is %v, want both repos", signalKeys(record.Detected))
	}
}

func TestLoopsSurfacesTheUnavailableGhNoteWithoutLosingGitSignals(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	home := detectHome(t, root)

	_, record := loopsJson(t, home, cli.Seams{
		Git: fakeGit{repo: {branch: ptr("main"), dirty: 3}},
		Gh:  noGh,
	})

	assertKeys(t, signalKeys(record.Detected), []string{"dirty:" + repo})
	if len(record.Notes) != 1 {
		t.Fatalf("got %d notes %v, want 1", len(record.Notes), record.Notes)
	}
	assertMatches(t, record.Notes[0], `GitHub signals unavailable`)
}

func TestLoopsKeepsTheOtherReposWhenOneRepoCannotBeProbed(t *testing.T) {
	root := t.TempDir()
	alpha := makeRepo(t, root, "alpha")
	beta := makeRepo(t, root, "beta")
	home := detectHome(t, root)

	_, record := loopsJson(t, home, cli.Seams{
		Git: explodingGit{
			fakeGit: fakeGit{alpha: {branch: ptr("main"), dirty: 2}, beta: {branch: ptr("main"), dirty: 3}},
			at:      alpha,
		},
		Gh: fakeGh{},
	})

	// A repo that blew up mid-probe reports nothing; it does not cost the scan its other results.
	assertKeys(t, signalKeys(record.Detected), []string{"dirty:" + beta})
	if len(record.Notes) != 0 {
		t.Errorf("got notes %v, want none", record.Notes)
	}
}

func TestLoopsDegradesToDeclaredItemsWhenDetectionFails(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "alpha")
	home := detectHome(t, root)
	seedLog(t, home, map[string]any{"ts": "2026-08-09T09:00:00.000Z", "ev": "add", "id": "k3f9", "title": "Chart legend"})

	seams := cli.Seams{Git: fakeGit{}, Gh: explodingGh{}}

	run, record := loopsJson(t, home, seams)
	if run.code != cli.ExitOK {
		t.Fatalf("loops exited %d: %s", run.code, run.err)
	}
	assertKeys(t, record.itemIds(), []string{"k3f9"})
	if len(record.Detected) != 0 {
		t.Errorf("detected is %v, want empty", signalKeys(record.Detected))
	}
	if record.DismissedCount != 0 {
		t.Errorf("dismissed count is %d, want 0", record.DismissedCount)
	}
	if len(record.Notes) != 1 {
		t.Fatalf("got %d notes %v, want 1", len(record.Notes), record.Notes)
	}
	assertMatches(t, record.Notes[0], `detection failed.*gh exploded`)

	human := runLoops(t, home, seams)
	if human.code != cli.ExitOK {
		t.Fatalf("loops exited %d: %s", human.code, human.err)
	}
	assertMatches(t, human.out, `OPEN LOOPS`)
	assertMatches(t, human.out, `Chart legend`)
	assertMatches(t, human.out, `detection failed`)
	if strings.Contains(human.out, "DETECTED") {
		t.Errorf("a failed detection must not render a DETECTED section:\n%s", human.out)
	}
}

func lineContaining(t *testing.T, lines []string, needle string) string {
	t.Helper()

	index := indexOfContaining(lines, needle)
	if index < 0 {
		t.Fatalf("no line contains %q", needle)
	}
	return lines[index]
}

func indexOfContaining(lines []string, needle string) int {
	for index, line := range lines {
		if strings.Contains(line, needle) {
			return index
		}
	}
	return -1
}

func indexOfPrefix(lines []string, prefix string) int {
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return index
		}
	}
	return -1
}
