package repos

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/sessions"
)

var now = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

// makeRepo creates <root>/<relative> and marks it a repo by giving it a .git directory.
func makeRepo(t *testing.T, root, relative string) string {
	t.Helper()

	dir := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	return dir
}

func cfg(roots []string, maxDepth, windowDays int) config.Config {
	return config.Config{Settings: config.Settings{
		ScanRoots:        roots,
		ScanMaxDepth:     maxDepth,
		ActiveWindowDays: windowDays,
	}}
}

// fakeGit answers only the HEAD date the activity gate asks for.
type fakeGit struct{ heads map[string]time.Time }

func (f fakeGit) HeadCommitDate(repo string) *time.Time {
	head, found := f.heads[repo]
	if !found {
		return nil
	}
	return &head
}

func (fakeGit) CurrentBranch(string) *string { branch := "main"; return &branch }
func (fakeGit) AheadCount(string) *int       { return nil }
func (fakeGit) DirtyFileCount(string) int    { return 0 }
func (fakeGit) IsRepo(string) bool           { return true }

func daysAgo(days int) time.Time {
	return now.Add(-time.Duration(days) * 24 * time.Hour)
}

func session(project string, endedDaysAgo int) sessions.CachedSession {
	ended := daysAgo(endedDaysAgo).Format(time.RFC3339)
	return sessions.CachedSession{
		Session: sessions.Session{
			Id:      "s-" + project,
			Title:   "a session",
			Project: &project,
			Started: &ended,
			Ended:   &ended,
			Prompts: 3,
		},
		File: sessions.FileStat{Path: project + ".jsonl"},
	}
}

func assertSameRepos(t *testing.T, got, want []string) {
	t.Helper()

	sorted := slices.Clone(got)
	slices.Sort(sorted)
	expected := slices.Clone(want)
	slices.Sort(expected)

	if !slices.Equal(sorted, expected) {
		t.Fatalf("repos = %v, want %v", got, want)
	}
}

func TestDiscoverFindsReposAtDepthOneAndAtScanMaxDepth(t *testing.T) {
	root := t.TempDir()
	shallow := makeRepo(t, root, "alpha")
	deep := makeRepo(t, root, filepath.Join("a", "b", "gamma"))

	assertSameRepos(t, Discover(cfg([]string{root}, 3, 30)), []string{shallow, deep})
}

func TestDiscoverIgnoresARepoDeeperThanScanMaxDepth(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, filepath.Join("a", "b", "c", "too-deep"))

	assertSameRepos(t, Discover(cfg([]string{root}, 3, 30)), nil)
}

func TestDiscoverTreatsAScanRootThatIsItselfARepoAsTheOnlyResult(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("creating %s: %v", root, err)
	}
	makeRepo(t, root, "child")

	assertSameRepos(t, Discover(cfg([]string{root}, 4, 30)), []string{root})
}

func TestDiscoverSkipsNodeModulesBinAndObj(t *testing.T) {
	root := t.TempDir()
	real := makeRepo(t, root, "alpha")
	makeRepo(t, root, filepath.Join("node_modules", "vendored"))
	makeRepo(t, root, filepath.Join("bin", "built"))
	makeRepo(t, root, filepath.Join("obj", "generated"))

	assertSameRepos(t, Discover(cfg([]string{root}, 4, 30)), []string{real})
}

func TestDiscoverDoesNotDescendIntoGitInternals(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	makeRepo(t, repo, filepath.Join(".git", "modules", "sub"))

	assertSameRepos(t, Discover(cfg([]string{root}, 4, 30)), []string{repo})
}

func TestDiscoverDoesNotReportARepoNestedInsideAnotherRepo(t *testing.T) {
	root := t.TempDir()
	outer := makeRepo(t, root, "alpha")
	makeRepo(t, root, filepath.Join("alpha", "vendor", "inner"))

	assertSameRepos(t, Discover(cfg([]string{root}, 4, 30)), []string{outer})
}

func TestDiscoverSkipsAMissingScanRootWithoutError(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	missing := filepath.Join(root, "not", "there")

	assertSameRepos(t, Discover(cfg([]string{missing, root}, 4, 30)), []string{repo})
}

func TestDiscoverReportsARepoFoundUnderTwoScanRootsOnce(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")

	assertSameRepos(t, Discover(cfg([]string{root, root}, 4, 30)), []string{repo})
}

func TestDiscoverReturnsSubdirectoriesInSortedOrder(t *testing.T) {
	root := t.TempDir()
	zulu := makeRepo(t, root, "zulu")
	alpha := makeRepo(t, root, "alpha")
	mike := makeRepo(t, root, "mike")

	got := Discover(cfg([]string{root}, 4, 30))
	if !slices.Equal(got, []string{alpha, mike, zulu}) {
		t.Fatalf("repos = %v, want discovery order %v", got, []string{alpha, mike, zulu})
	}
}

func TestActiveAdmitsARepoWithARecentSessionAndOneWithARecentHead(t *testing.T) {
	root := t.TempDir()
	bySession := makeRepo(t, root, "session-repo")
	byHead := makeRepo(t, root, "head-repo")
	stale := makeRepo(t, root, "stale-repo")

	git := fakeGit{heads: map[string]time.Time{
		bySession: daysAgo(200),
		byHead:    daysAgo(2),
		stale:     daysAgo(200),
	}}
	found := []sessions.CachedSession{
		session(filepath.Join(bySession, "src"), 1),
		session(filepath.Join(stale, "src"), 90),
	}

	active := Active(cfg([]string{root}, 4, 30), found, ActiveOptions{Now: now, Git: git})
	assertSameRepos(t, active, []string{bySession, byHead})
}

func TestActiveRejectsARepoWithNeitherARecentSessionNorARecentHead(t *testing.T) {
	root := t.TempDir()
	stale := makeRepo(t, root, "stale-repo")

	git := fakeGit{heads: map[string]time.Time{stale: daysAgo(31)}}
	found := []sessions.CachedSession{session(filepath.Join(stale, "src"), 31)}

	active := Active(cfg([]string{root}, 4, 30), found, ActiveOptions{Now: now, Git: git})
	assertSameRepos(t, active, nil)
}

func TestActiveToleratesARepoWithNoCommitsAndASessionWithNoTimestamps(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "empty-repo")

	undated := session(filepath.Join(repo, "src"), 0)
	undated.Started = nil
	undated.Ended = nil

	active := Active(cfg([]string{root}, 4, 30), []sessions.CachedSession{undated}, ActiveOptions{Now: now, Git: fakeGit{}})
	assertSameRepos(t, active, nil)
}

func TestActiveIgnoresSessionsOutsideEveryDiscoveredRepo(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")

	settings := cfg([]string{root}, 4, 30)
	found := []sessions.CachedSession{session(filepath.Join(root, "elsewhere"), 1)}

	assertSameRepos(t, Active(settings, found, ActiveOptions{Now: now, Git: fakeGit{}}), nil)

	git := fakeGit{heads: map[string]time.Time{repo: daysAgo(1)}}
	assertSameRepos(t, Active(settings, found, ActiveOptions{Now: now, Git: git}), []string{repo})
}

func TestActiveUsesTheSuppliedReposRatherThanDiscovering(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "discovered")
	supplied := filepath.Join(root, "supplied")

	git := fakeGit{heads: map[string]time.Time{supplied: daysAgo(1)}}
	active := Active(cfg([]string{root}, 4, 30), nil, ActiveOptions{Now: now, Git: git, Repos: []string{supplied}})
	assertSameRepos(t, active, []string{supplied})
}

func TestActiveKeepsDiscoveryOrder(t *testing.T) {
	root := t.TempDir()
	alpha := makeRepo(t, root, "alpha")
	mike := makeRepo(t, root, "mike")
	zulu := makeRepo(t, root, "zulu")

	git := fakeGit{heads: map[string]time.Time{alpha: daysAgo(1), mike: daysAgo(1), zulu: daysAgo(1)}}
	got := Active(cfg([]string{root}, 4, 30), nil, ActiveOptions{Now: now, Git: git})
	if !slices.Equal(got, []string{alpha, mike, zulu}) {
		t.Fatalf("active = %v, want %v", got, []string{alpha, mike, zulu})
	}
}

func TestForPathPrefersTheLongestMatchingRepoPrefix(t *testing.T) {
	repos := []string{
		filepath.Join("C:", "dev", "outer"),
		filepath.Join("C:", "dev", "outer", "inner"),
		filepath.Join("C:", "dev", "other"),
	}

	cases := map[string]string{
		filepath.Join("C:", "dev", "outer", "src"):          repos[0],
		filepath.Join("C:", "dev", "outer", "inner", "src"): repos[1],
		filepath.Join("C:", "dev", "outer"):                 repos[0],
	}
	for path, want := range cases {
		if got := ForPath(path, repos); got != want {
			t.Fatalf("ForPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestForPathReturnsEmptyWhenNothingMatches(t *testing.T) {
	repos := []string{filepath.Join("C:", "dev", "outer")}

	// A sibling whose name merely starts with the repo name is not inside it.
	for _, path := range []string{
		filepath.Join("C:", "dev", "elsewhere"),
		filepath.Join("C:", "dev", "outerly"),
		"",
		"   ",
	} {
		if got := ForPath(path, repos); got != "" {
			t.Fatalf("ForPath(%q) = %q, want no match", path, got)
		}
	}

	if got := ForPath(filepath.Join("C:", "dev", "outer"), nil); got != "" {
		t.Fatalf("ForPath against no repos = %q, want no match", got)
	}
}

func TestForPathToleratesTrailingAndMixedSeparators(t *testing.T) {
	repos := []string{`C:\dev\outer`}

	for _, path := range []string{`C:\dev\outer\`, "C:/dev/outer/src", `  C:\dev\outer\src  `} {
		if got := ForPath(path, repos); got != repos[0] {
			t.Fatalf("ForPath(%q) = %q, want %q", path, got, repos[0])
		}
	}
}
