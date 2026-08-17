package command_test

import (
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
	Groups         []struct {
		Project *string `json:"project"`
		Items   []struct {
			Id        string  `json:"id"`
			Title     string  `json:"title"`
			Status    string  `json:"status"`
			WaitingOn *string `json:"waitingOn"`
		} `json:"items"`
	} `json:"groups"`
}

// itemIds are every declared item across the groups, in group order.
func (r loopsRecord) itemIds() []string {
	ids := []string{}
	for _, group := range r.Groups {
		for _, item := range group.Items {
			ids = append(ids, item.Id)
		}
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
	assertKeys(t, record.itemIds(), []string{"k3f9", "m7qz", "p2vn", "q8xt"})
}

func TestLoopsKeepsWaitingItemsAlongsideOpenOnes(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet)
	for _, group := range record.Groups {
		for _, item := range group.Items {
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
	}
	t.Fatal("the waiting item is missing")
}

func TestLoopsGroupsByProjectWithNoProjectLastAndOldestFirst(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet)
	if len(record.Groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(record.Groups))
	}
	assertProject(t, record.Groups[0].Project, drProject)
	assertProject(t, record.Groups[1].Project, waidProject)
	if record.Groups[2].Project != nil {
		t.Errorf("the last group is %q, want the project-less one", *record.Groups[2].Project)
	}
	assertKeys(t, record.itemIds(), []string{"k3f9", "m7qz", "p2vn", "q8xt"})
}

func TestLoopsNarrowsToOneProject(t *testing.T) {
	home := loopsHome(t)
	seedLoops(t, home)

	_, record := loopsJson(t, home, quiet, "-p", "waid")
	if len(record.Groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(record.Groups))
	}
	assertProject(t, record.Groups[0].Project, waidProject)
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

	headings := []string{}
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			headings = append(headings, strings.TrimSpace(line))
		}
	}
	assertKeys(t, headings, []string{drProject, waidProject, "(no project)"})

	chart := lineContaining(t, lines, "k3f9")
	assertMatches(t, chart, `^ {4}k3f9 {2}open {6}Chart legend overflows at 4\+ series`)
	assertMatches(t, chart, `\[bug]`)
	assertMatches(t, chart, `\d+[mhdwy]$`)

	approval := lineContaining(t, lines, "m7qz")
	assertMatches(t, approval, `waiting`)
	assertMatches(t, approval, `← Dan`)
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
	}, "-p", "beta")

	assertKeys(t, record.itemIds(), []string{"m7qz"})
	assertKeys(t, signalKeys(record.Detected), []string{"dirty:" + beta})
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

func assertProject(t *testing.T, got *string, want string) {
	t.Helper()

	if got == nil {
		t.Errorf("project is null, want %s", want)
		return
	}
	if *got != want {
		t.Errorf("project is %q, want %q", *got, want)
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
