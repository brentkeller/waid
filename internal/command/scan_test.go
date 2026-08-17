package command_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/gh"
)

// signalRecord is one entry of the signals array scan prints.
type signalRecord struct {
	Key     string  `json:"key"`
	Kind    string  `json:"kind"`
	Title   string  `json:"title"`
	Subject string  `json:"subject"`
	Detail  string  `json:"detail"`
	Branch  *string `json:"branch"`
	Project *string `json:"project"`
	Age     string  `json:"age"`
}

// scanRecord is the --json payload scan prints.
type scanRecord struct {
	Signals        []signalRecord `json:"signals"`
	DismissedCount int            `json:"dismissedCount"`
	Notes          []string       `json:"notes"`
}

func signalKeys(signals []signalRecord) []string {
	keys := make([]string, 0, len(signals))
	for _, signal := range signals {
		keys = append(keys, signal.Key)
	}
	return keys
}

// repoState is what fakeGit reports for one repo. A zero value reads as a quiet, clean repo on
// `main` whose HEAD landed yesterday.
type repoState struct {
	head *time.Time
	// branch names the checked-out branch; nil means `main` unless detached is set.
	branch   *string
	detached bool
	ahead    *int
	dirty    int
}

// fakeGit answers from a per-repo table; anything unlisted reads as a quiet, clean repo.
type fakeGit map[string]repoState

func (f fakeGit) HeadCommitDate(repo string) *time.Time {
	if head := f[repo].head; head != nil {
		return head
	}
	head := daysAgo(1)
	return &head
}

func (f fakeGit) CurrentBranch(repo string) *string {
	state := f[repo]
	if state.detached {
		return nil
	}
	if state.branch != nil {
		return state.branch
	}
	branch := "main"
	return &branch
}

func (f fakeGit) AheadCount(repo string) *int { return f[repo].ahead }

func (f fakeGit) DirtyFileCount(repo string) int { return f[repo].dirty }

func (fakeGit) IsRepo(string) bool { return true }

// explodingGit survives the activity gate and then blows up probing one repo.
type explodingGit struct {
	fakeGit
	at string
}

func (e explodingGit) CurrentBranch(repo string) *string {
	if repo == e.at {
		panic(errors.New("git exploded"))
	}
	return e.fakeGit.CurrentBranch(repo)
}

// explodingGh blows up the account-wide query, which is not per-repo and so fails the whole pass.
type explodingGh struct{}

func (explodingGh) ReviewRequested() ([]gh.Pr, error) { panic(errors.New("gh exploded")) }
func (explodingGh) Authored() ([]gh.Pr, error)        { panic(errors.New("gh exploded")) }

// fakeGh serves two fixed lists, or fails both queries when err is set.
type fakeGh struct {
	review   []gh.Pr
	authored []gh.Pr
	err      error
}

func (f fakeGh) ReviewRequested() ([]gh.Pr, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.review, nil
}

func (f fakeGh) Authored() ([]gh.Pr, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.authored, nil
}

var noGh = fakeGh{err: errors.New("gh: command not found")}

func daysAgo(days int) time.Time {
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour)
}

func pr(repository string, number int, apply func(*gh.Pr)) gh.Pr {
	item := gh.Pr{
		Number:     number,
		Repository: repository,
		Title:      "PR " + strconv.Itoa(number),
		Author:     "tmoore",
		State:      "open",
		CreatedAt:  daysAgo(3).Format(time.RFC3339),
		Url:        "https://github.com/" + repository + "/pull/" + strconv.Itoa(number),
		Branch:     "pr-" + strconv.Itoa(number) + "-branch",
	}
	if apply != nil {
		apply(&item)
	}
	return item
}

// makeRepo creates <root>/<name> and marks it a repo by giving it a .git directory.
func makeRepo(t *testing.T, root, name string) string {
	t.Helper()

	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	return dir
}

// detectHome is a home configured to scan root with GitHub signals on. config.json is written before
// the CLI runs, since first-run init never overwrites an existing one.
func detectHome(t *testing.T, root string) string {
	t.Helper()

	home := makeHome(t)
	settings := map[string]any{
		"scanRoots":        []string{root},
		"scanMaxDepth":     3,
		"activeWindowDays": 30,
		"ghUser":           "me",
		"claudeDir":        filepath.Join(home, "claude"),
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("encoding the config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.json"), raw, 0o644); err != nil {
		t.Fatalf("writing the config: %v", err)
	}
	return home
}

// runScan runs scan with the detection seams supplied; --no-sync keeps the real ~/.claude out of the
// test.
func runScan(t *testing.T, home string, seams cli.Seams, args ...string) result {
	t.Helper()

	return waidWith(t, home, seams, append([]string{"scan"}, append(args, "--no-sync")...)...)
}

// scanJson runs scan --json and decodes what it printed.
func scanJson(t *testing.T, home string, seams cli.Seams, args ...string) (result, scanRecord) {
	t.Helper()

	run := runScan(t, home, seams, append(args, "--json")...)
	var record scanRecord
	run.decode(t, &record)
	return run, record
}

func assertKeys(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d keys %v, want %d %v", len(got), got, len(want), want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("key %d is %q, want %q\ngot:  %v\nwant: %v", index, got[index], want[index], got, want)
		}
	}
}

func TestScanJsonReturnsEverySignalInRankOrder(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	home := detectHome(t, root)

	run, record := scanJson(t, home, cli.Seams{
		Git: fakeGit{repo: {branch: ptr("topic"), ahead: ptr(2), dirty: 5}},
		Gh:  fakeGh{review: []gh.Pr{pr("acme/web", 123, nil)}, authored: []gh.Pr{pr("acme/web", 456, nil)}},
	})

	if run.code != cli.ExitOK {
		t.Fatalf("scan exited %d: %s", run.code, run.err)
	}
	assertKeys(t, signalKeys(record.Signals), []string{
		"review:acme/web#123",
		"pr:acme/web#456",
		"ahead:" + repo + ":topic",
		"dirty:" + repo,
	})
	if record.DismissedCount != 0 {
		t.Errorf("dismissed count is %d, want 0", record.DismissedCount)
	}
	if len(record.Notes) != 0 {
		t.Errorf("notes are %v, want none", record.Notes)
	}
}

func TestScanNarrowsToTheResolvedProject(t *testing.T) {
	root := t.TempDir()
	alpha := makeRepo(t, root, "alpha")
	beta := makeRepo(t, root, "beta")
	home := detectHome(t, root)

	seams := cli.Seams{
		Git: fakeGit{
			alpha: {branch: ptr("main"), ahead: ptr(1), dirty: 1},
			beta:  {branch: ptr("main"), ahead: ptr(1), dirty: 1},
		},
		Gh: fakeGh{},
	}

	_, absolute := scanJson(t, home, seams, "-p", alpha)
	assertKeys(t, signalKeys(absolute.Signals), []string{"ahead:" + alpha + ":main", "dirty:" + alpha})

	// A partial resolves because the detected repo paths join the known projects.
	_, partial := scanJson(t, home, seams, "-p", "beta")
	assertKeys(t, signalKeys(partial.Signals), []string{"ahead:" + beta + ":main", "dirty:" + beta})
}

func TestScanDropsPrSignalsWithNoLocalCheckout(t *testing.T) {
	root := t.TempDir()
	alpha := makeRepo(t, root, "alpha")
	home := detectHome(t, root)

	_, record := scanJson(t, home, cli.Seams{
		Git: fakeGit{alpha: {branch: ptr("main"), dirty: 2}},
		Gh:  fakeGh{review: []gh.Pr{pr("acme/nowhere", 7, nil)}},
	}, "-p", alpha)

	assertKeys(t, signalKeys(record.Signals), []string{"dirty:" + alpha})
}

func TestScanOnACleanMachineSaysSoAndExitsZero(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "clean")
	home := detectHome(t, root)

	seams := cli.Seams{Git: fakeGit{}, Gh: fakeGh{}}
	run, record := scanJson(t, home, seams)
	if run.code != cli.ExitOK {
		t.Fatalf("scan exited %d: %s", run.code, run.err)
	}
	if len(record.Signals) != 0 {
		t.Errorf("signals are %v, want none", signalKeys(record.Signals))
	}

	human := runScan(t, home, seams)
	if human.code != cli.ExitOK {
		t.Fatalf("scan exited %d: %s", human.code, human.err)
	}
	if !strings.Contains(human.out, "nothing detected") {
		t.Errorf("output %q does not say nothing was detected", human.out)
	}
}

func TestScanRendersTheDetectedHeaderColumnsAndFooter(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "inl-prt")
	home := detectHome(t, root)
	seedLog(t, home, map[string]any{
		"ts": daysAgo(1).UTC().Format("2006-01-02T15:04:05.000Z"), "ev": "dismiss", "key": "pr:acme/web#456",
	})

	human := runScan(t, home, cli.Seams{
		Git: fakeGit{repo: {branch: ptr("inl-prt-fixes"), dirty: 14}},
		Gh: fakeGh{
			review: []gh.Pr{pr("acme/web", 123, func(item *gh.Pr) {
				item.Title = "Fix the chart legend"
				item.Branch = "chart-legend"
			})},
			authored: []gh.Pr{pr("acme/web", 456, nil)},
		},
	})

	if human.code != cli.ExitOK {
		t.Fatalf("scan exited %d: %s", human.code, human.err)
	}
	lines := strings.Split(human.out, "\n")
	if lines[0] != "DETECTED  (2 shown, 1 dismissed)" {
		t.Errorf("header is %q", lines[0])
	}
	if lines[1] != "" {
		t.Errorf("line 2 is %q, want blank", lines[1])
	}
	assertMatches(t, lines[2], `^ {2}review:acme/web#123 +Fix the chart legend +@tmoore · chart-legend · 3d$`)
	// The HEAD date is relative to when the fake was built, so the age is a shape, not a value.
	assertMatches(t, lines[3], `^ {2}dirty:`+regexp.QuoteMeta(repo)+` +14 uncommitted files +inl-prt-fixes · \d+[hd]$`)
	assertMatches(t, human.out, `waid promote <key>`)
	assertMatches(t, human.out, `waid dismiss <key>`)
}

func TestScanAlignsTheSubjectColumnAcrossKeysOfDifferentLength(t *testing.T) {
	home := detectHome(t, t.TempDir())

	human := runScan(t, home, cli.Seams{
		Git: fakeGit{},
		Gh: fakeGh{authored: []gh.Pr{
			pr("acme/web", 1, func(item *gh.Pr) { item.Title = "Short key" }),
			pr("acme/a-much-longer-repo", 22222, func(item *gh.Pr) { item.Title = "Long key" }),
		}},
	})

	if human.code != cli.ExitOK {
		t.Fatalf("scan exited %d: %s", human.code, human.err)
	}
	lines := strings.Split(human.out, "\n")
	if first, second := columnOf(lines[2], "Short key"), columnOf(lines[3], "Long key"); first != second {
		t.Errorf("subjects start at %d and %d:\n%s\n%s", first, second, lines[2], lines[3])
	}
}

func TestScanTruncatesASubjectTooWideForItsColumn(t *testing.T) {
	home := detectHome(t, t.TempDir())
	long := "Migrate the BudgetBreakdown chart from ASP.NET to AngularJS and D3 in one pass"

	human := runScan(t, home, cli.Seams{
		Git: fakeGit{},
		Gh: fakeGh{authored: []gh.Pr{
			pr("acme/web", 1, func(item *gh.Pr) { item.Title = long }),
			pr("acme/web", 2, func(item *gh.Pr) { item.Title = "Short" }),
		}},
	})

	lines := strings.Split(human.out, "\n")
	if strings.Contains(lines[2], long) {
		t.Errorf("an over-wide subject must be truncated: %q", lines[2])
	}
	if !strings.Contains(lines[2], "…") {
		t.Errorf("a truncated subject must be ellipsized: %q", lines[2])
	}
	if first, second := columnOf(lines[2], "pr-1-branch"), columnOf(lines[3], "pr-2-branch"); first != second {
		t.Errorf("meta starts at %d and %d:\n%s\n%s", first, second, lines[2], lines[3])
	}
}

func TestScanNotesAnUnavailableGhOnceAndStillReportsGitSignals(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	home := detectHome(t, root)

	seams := cli.Seams{Git: fakeGit{repo: {branch: ptr("main"), dirty: 3}}, Gh: noGh}
	_, record := scanJson(t, home, seams)
	assertKeys(t, signalKeys(record.Signals), []string{"dirty:" + repo})
	if len(record.Notes) != 1 {
		t.Fatalf("got %d notes %v, want 1", len(record.Notes), record.Notes)
	}
	assertMatches(t, record.Notes[0], `GitHub signals unavailable`)

	human := runScan(t, home, seams)
	noted := []string{}
	for _, line := range strings.Split(human.out, "\n") {
		if strings.Contains(line, "GitHub signals unavailable") {
			noted = append(noted, line)
		}
	}
	if len(noted) != 1 {
		t.Fatalf("got %d note lines %v, want 1", len(noted), noted)
	}
	assertMatches(t, noted[0], `^ {2}\S`)
}

// columnOf is where needle starts in text, counted in the UTF-16 units the columns are measured in
// rather than in bytes, so an ellipsis earlier in the line does not shift the answer.
func columnOf(text, needle string) int {
	index := strings.Index(text, needle)
	if index < 0 {
		return -1
	}
	return len(utf16.Encode([]rune(text[:index])))
}

func assertMatches(t *testing.T, text, pattern string) {
	t.Helper()

	matched, err := regexp.MatchString(pattern, text)
	if err != nil {
		t.Fatalf("bad pattern %q: %v", pattern, err)
	}
	if !matched {
		t.Errorf("%q does not match %s", text, pattern)
	}
}
