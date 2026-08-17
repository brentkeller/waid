package detect

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/gh"
)

var now = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

func daysAgo(days int) time.Time {
	return now.Add(-time.Duration(days) * 24 * time.Hour)
}

// makeRepo creates <root>/<relative> and marks it a repo by giving it a .git directory.
func makeRepo(t *testing.T, root, relative string) string {
	t.Helper()

	dir := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	return dir
}

// testConfig scans root from a throwaway home, so the gh cache one test writes cannot leak into
// another.
func testConfig(t *testing.T, root string, apply func(*config.Config)) config.Config {
	t.Helper()

	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatalf("loading the config: %v", err)
	}

	user := "me"
	cfg.ScanRoots = []string{root}
	cfg.ScanMaxDepth = 3
	cfg.ActiveWindowDays = 30
	cfg.GhUser = &user
	if apply != nil {
		apply(&cfg)
	}
	return cfg
}

// repoState is what fakeGit reports for one repo. A zero value reads as a quiet, clean repo on
// `main` whose HEAD landed yesterday.
type repoState struct {
	head *time.Time
	// branch names the checked-out branch; nil means `main` unless detached is set.
	branch   *string
	detached bool
	ahead    *int
	dirty    *int
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

func (f fakeGit) DirtyFileCount(repo string) int {
	if dirty := f[repo].dirty; dirty != nil {
		return *dirty
	}
	return 0
}

func (fakeGit) IsRepo(string) bool { return true }

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

func pr(repository string, number int, apply func(*gh.Pr)) gh.Pr {
	item := gh.Pr{
		Number:     number,
		Repository: repository,
		Title:      "PR " + strconv.Itoa(number),
		Author:     "tmoore",
		IsDraft:    false,
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

func state(dismissed ...string) events.State {
	return events.State{Items: []events.Item{}, Dismissed: dismissed, Problems: []events.Problem{}}
}

func keys(signals []Signal) []string {
	found := make([]string, 0, len(signals))
	for _, signal := range signals {
		found = append(found, signal.Key)
	}
	return found
}

func kinds(signals []Signal) []string {
	found := make([]string, 0, len(signals))
	for _, signal := range signals {
		found = append(found, string(signal.Kind))
	}
	return found
}

func assertEqualStrings(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d entries %v, want %d %v", len(got), got, len(want), want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("entry %d is %q, want %q\ngot:  %v\nwant: %v", index, got[index], want[index], got, want)
		}
	}
}

func assertEqual(t *testing.T, label, got, want string) {
	t.Helper()

	if got != want {
		t.Errorf("%s is %q, want %q", label, got, want)
	}
}

// assertNilString fails when p is set, naming its value.
func assertNilString(t *testing.T, label string, p *string) {
	t.Helper()

	if p != nil {
		t.Errorf("%s is %q, want nil", label, *p)
	}
}

func str(text string) *string { return &text }
func num(value int) *int      { return &value }

func TestSignalsProducesAllFourKindsWithTheDocumentedKeyFormats(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{repo: {branch: str("feature"), ahead: num(2), dirty: num(5)}},
		Gh:    fakeGh{review: []gh.Pr{pr("acme/web", 123, nil)}, authored: []gh.Pr{pr("acme/web", 456, nil)}},
	})

	assertEqualStrings(t, keys(result.Signals), []string{
		"review:acme/web#123",
		"pr:acme/web#456",
		"ahead:" + repo + ":feature",
		"dirty:" + repo,
	})
	assertEqualStrings(t, result.Notes, []string{})
	if result.DismissedCount != 0 {
		t.Errorf("dismissed count is %d, want 0", result.DismissedCount)
	}
	assertEqualStrings(t, kinds(result.Signals), []string{"review", "pr", "ahead", "dirty"})
}

func TestSignalsFillsTitleSubjectDetailAndProjectForEachKind(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "web")

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{repo: {branch: str("inl-prt-fixes"), ahead: num(3), dirty: num(14)}},
		Gh: fakeGh{review: []gh.Pr{pr("acme/web", 123, func(item *gh.Pr) {
			item.Title = "Fix the chart legend"
			item.Branch = "chart-legend"
		})}},
	})

	if len(result.Signals) != 3 {
		t.Fatalf("got %d signals %v, want 3", len(result.Signals), keys(result.Signals))
	}
	review, ahead, dirty := result.Signals[0], result.Signals[1], result.Signals[2]

	assertEqual(t, "review title", review.Title, "Fix the chart legend")
	assertEqual(t, "review subject", review.Subject, "Fix the chart legend")
	assertEqual(t, "review detail", review.Detail, "@tmoore · chart-legend · 3d")
	if review.Branch == nil || *review.Branch != "chart-legend" {
		t.Errorf("review branch is %v, want chart-legend", review.Branch)
	}
	// The PR's repo name matches exactly one local repo, so `-p` can narrow to it.
	if review.Project == nil || *review.Project != repo {
		t.Errorf("review project is %v, want %s", review.Project, repo)
	}
	assertEqual(t, "review age", review.Age, "3d")

	if ahead.Project == nil || *ahead.Project != repo {
		t.Errorf("ahead project is %v, want %s", ahead.Project, repo)
	}
	assertEqual(t, "ahead subject", ahead.Subject, "3 commits ahead")
	assertEqual(t, "ahead detail", ahead.Detail, "inl-prt-fixes · 1d")
	if ahead.Branch == nil || *ahead.Branch != "inl-prt-fixes" {
		t.Errorf("ahead branch is %v, want inl-prt-fixes", ahead.Branch)
	}

	if dirty.Project == nil || *dirty.Project != repo {
		t.Errorf("dirty project is %v, want %s", dirty.Project, repo)
	}
	assertEqual(t, "dirty subject", dirty.Subject, "14 uncommitted files")
	assertEqual(t, "dirty detail", dirty.Detail, "inl-prt-fixes · 1d")
	if dirty.Branch == nil || *dirty.Branch != "inl-prt-fixes" {
		t.Errorf("dirty branch is %v, want inl-prt-fixes", dirty.Branch)
	}
	if !strings.Contains(dirty.Title, "web") {
		t.Errorf("dirty title %q does not name the repo", dirty.Title)
	}
}

func TestSignalsReportsTheBranchDraftAndOpenStateOnAuthoredPrs(t *testing.T) {
	root := t.TempDir()

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{},
		Gh: fakeGh{authored: []gh.Pr{
			pr("acme/web", 1, func(item *gh.Pr) { item.IsDraft = true }),
			pr("acme/web", 2, nil),
		}},
	})

	if len(result.Signals) != 2 {
		t.Fatalf("got %d signals, want 2", len(result.Signals))
	}
	assertEqual(t, "draft detail", result.Signals[0].Detail, "pr-1-branch · draft · 3d")
	assertEqual(t, "open detail", result.Signals[1].Detail, "pr-2-branch · open · 3d")
}

func TestSignalsOmitsABranchTheSearchCouldNotReport(t *testing.T) {
	root := t.TempDir()

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{},
		Gh: fakeGh{
			review:   []gh.Pr{pr("acme/web", 1, func(item *gh.Pr) { item.Branch = "" })},
			authored: []gh.Pr{pr("acme/web", 2, func(item *gh.Pr) { item.Branch = "" })},
		},
	})

	if len(result.Signals) != 2 {
		t.Fatalf("got %d signals, want 2", len(result.Signals))
	}
	assertEqual(t, "review detail", result.Signals[0].Detail, "@tmoore · 3d")
	assertNilString(t, "review branch", result.Signals[0].Branch)
	assertEqual(t, "pr detail", result.Signals[1].Detail, "open · 3d")
	assertNilString(t, "pr branch", result.Signals[1].Branch)
}

func TestSignalsLeavesTheBranchOffADetachedHead(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "detached")

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{repo: {detached: true, dirty: num(2)}},
		Gh:    fakeGh{},
	})

	if len(result.Signals) != 1 {
		t.Fatalf("got %d signals, want 1", len(result.Signals))
	}
	assertEqual(t, "subject", result.Signals[0].Subject, "2 uncommitted files")
	assertEqual(t, "detail", result.Signals[0].Detail, "1d")
	assertNilString(t, "branch", result.Signals[0].Branch)
}

func TestSignalsRanksByKindBreakingTiesByAgeDescending(t *testing.T) {
	root := t.TempDir()
	fresh := makeRepo(t, root, "fresh")
	stale := makeRepo(t, root, "stale")

	freshHead, staleHead := daysAgo(1), daysAgo(9)
	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git: fakeGit{
			fresh: {head: &freshHead, branch: str("main"), ahead: num(1), dirty: num(2)},
			stale: {head: &staleHead, branch: str("main"), ahead: num(1), dirty: num(2)},
		},
		Gh: fakeGh{
			review: []gh.Pr{
				pr("acme/web", 10, func(item *gh.Pr) { item.CreatedAt = daysAgo(1).Format(time.RFC3339) }),
				pr("acme/web", 11, func(item *gh.Pr) { item.CreatedAt = daysAgo(8).Format(time.RFC3339) }),
			},
			authored: []gh.Pr{pr("acme/web", 12, nil)},
		},
	})

	assertEqualStrings(t, keys(result.Signals), []string{
		"review:acme/web#11",
		"review:acme/web#10",
		"pr:acme/web#12",
		"ahead:" + stale + ":main",
		"ahead:" + fresh + ":main",
		"dirty:" + stale,
		"dirty:" + fresh,
	})
}

func TestSignalsFiltersDismissedKeysAndCountsThem(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")

	result := Signals(testConfig(t, root, nil), Deps{
		State: state("review:acme/web#123", "dirty:"+repo),
		Now:   now,
		Git:   fakeGit{repo: {branch: str("main"), ahead: num(4), dirty: num(1)}},
		Gh:    fakeGh{review: []gh.Pr{pr("acme/web", 123, nil)}},
	})

	assertEqualStrings(t, keys(result.Signals), []string{"ahead:" + repo + ":main"})
	if result.DismissedCount != 2 {
		t.Errorf("dismissed count is %d, want 2", result.DismissedCount)
	}
}

func TestSignalsScopesADismissalToTheExactPrItNames(t *testing.T) {
	root := t.TempDir()

	result := Signals(testConfig(t, root, nil), Deps{
		State: state("pr:o/r#123"),
		Now:   now,
		Git:   fakeGit{},
		Gh:    fakeGh{authored: []gh.Pr{pr("o/r", 123, nil), pr("o/r", 124, nil)}},
	})

	assertEqualStrings(t, keys(result.Signals), []string{"pr:o/r#124"})
	if result.DismissedCount != 1 {
		t.Errorf("dismissed count is %d, want 1", result.DismissedCount)
	}
}

func TestSignalsReturnsGitSignalsPlusANoteWhenGhIsUnavailable(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{repo: {branch: str("main"), ahead: num(2), dirty: num(3)}},
		Gh:    noGh,
	})

	assertEqualStrings(t, keys(result.Signals), []string{"ahead:" + repo + ":main", "dirty:" + repo})
	if len(result.Notes) != 1 {
		t.Fatalf("got %d notes %v, want 1", len(result.Notes), result.Notes)
	}
	if !strings.Contains(result.Notes[0], "gh: command not found") {
		t.Errorf("note %q does not carry gh's own failure", result.Notes[0])
	}
}

func TestSignalsProducesNoAheadSignalWithoutAnUpstreamOrABranch(t *testing.T) {
	root := t.TempDir()
	noUpstream := makeRepo(t, root, "no-upstream")
	detached := makeRepo(t, root, "detached")
	levelWith := makeRepo(t, root, "level")

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git: fakeGit{
			noUpstream: {ahead: nil, dirty: num(1)},
			detached:   {detached: true, ahead: num(3), dirty: num(1)},
			levelWith:  {ahead: num(0), dirty: num(1)},
		},
		Gh: fakeGh{},
	})

	assertEqualStrings(t, keys(result.Signals), []string{
		"dirty:" + detached,
		"dirty:" + levelWith,
		"dirty:" + noUpstream,
	})
}

func TestSignalsProducesNoDirtySignalForACleanRepo(t *testing.T) {
	root := t.TempDir()
	clean := makeRepo(t, root, "clean")

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{clean: {branch: str("main"), dirty: num(0)}},
		Gh:    fakeGh{},
	})

	assertEqualStrings(t, keys(result.Signals), []string{})
}

func TestSignalsReportsARepoThatIsBothDirtyAndAheadAsTwoSignals(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "busy")

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{repo: {branch: str("topic"), ahead: num(7), dirty: num(2)}},
		Gh:    fakeGh{},
	})

	assertEqualStrings(t, keys(result.Signals), []string{"ahead:" + repo + ":topic", "dirty:" + repo})
}

func TestSignalsIgnoresReposOutsideTheActivityWindow(t *testing.T) {
	root := t.TempDir()
	stale := makeRepo(t, root, "stale")

	head := daysAgo(90)
	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{stale: {head: &head, branch: str("main"), ahead: num(5), dirty: num(5)}},
		Gh:    fakeGh{},
	})

	assertEqualStrings(t, keys(result.Signals), []string{})
}

func TestSignalsLeavesAPrProjectNullWhenTheRepoNameIsNotLocalOrIsAmbiguous(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, filepath.Join("one", "web"))
	makeRepo(t, root, filepath.Join("two", "web"))

	result := Signals(testConfig(t, root, nil), Deps{
		State: state(),
		Now:   now,
		Git:   fakeGit{},
		Gh:    fakeGh{review: []gh.Pr{pr("acme/web", 1, nil), pr("acme/nowhere", 2, nil)}},
	})

	if len(result.Signals) != 2 {
		t.Fatalf("got %d signals %v, want 2", len(result.Signals), keys(result.Signals))
	}
	assertNilString(t, "ambiguous project", result.Signals[0].Project)
	assertNilString(t, "unmatched project", result.Signals[1].Project)
}
