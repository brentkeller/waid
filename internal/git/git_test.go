package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// run issues a git command inside repo, failing the test when git itself fails — the fixtures have
// to be built correctly for the assertions about them to mean anything.
func run(t *testing.T, repo string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, repo, err, out)
	}
}

// tempDir is a plain empty directory, used both as a repo root and as the non-repo case.
func tempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// makeRepo is a repo with identity configured and one commit on main.
func makeRepo(t *testing.T) string {
	t.Helper()

	dir := tempDir(t)
	run(t, dir, "init", "-b", "main")
	configure(t, dir)
	commit(t, dir, "a.txt", "one\n", "initial")
	return dir
}

func configure(t *testing.T, repo string) {
	t.Helper()

	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "waid test")
	run(t, repo, "config", "commit.gpgsign", "false")
}

func commit(t *testing.T, repo, file, contents, message string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(repo, file), []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", file, err)
	}
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-m", message)
}

// makeClone clones origin locally, so the clone's branch has a real upstream to be ahead of.
func makeClone(t *testing.T, origin string) string {
	t.Helper()

	dir := filepath.Join(tempDir(t), "clone")
	cmd := exec.Command("git", "clone", origin, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cloning %s: %v\n%s", origin, err, out)
	}
	configure(t, dir)
	return dir
}

func TestHeadCommitDateReturnsTheDateOfHead(t *testing.T) {
	date := Real().HeadCommitDate(makeRepo(t))
	if date == nil {
		t.Fatal("expected a commit date, got nil")
	}
	if delta := time.Since(*date); delta < -time.Minute || delta > time.Minute {
		t.Fatalf("commit date %s is %s away from now", date, delta)
	}
}

func TestCurrentBranchReturnsTheCheckedOutBranch(t *testing.T) {
	repo := makeRepo(t)
	assertBranch(t, Real().CurrentBranch(repo), "main")

	run(t, repo, "checkout", "-b", "feature/detect")
	assertBranch(t, Real().CurrentBranch(repo), "feature/detect")
}

func assertBranch(t *testing.T, got *string, want string) {
	t.Helper()

	if got == nil {
		t.Fatalf("expected branch %q, got nil", want)
	}
	if *got != want {
		t.Fatalf("branch = %q, want %q", *got, want)
	}
}

func TestCurrentBranchIsNilOnADetachedHead(t *testing.T) {
	repo := makeRepo(t)
	run(t, repo, "checkout", "--detach", "HEAD")

	if branch := Real().CurrentBranch(repo); branch != nil {
		t.Fatalf("branch = %q, want nil on a detached HEAD", *branch)
	}
}

func TestAheadCountIsNilWithoutAnUpstream(t *testing.T) {
	if ahead := Real().AheadCount(makeRepo(t)); ahead != nil {
		t.Fatalf("ahead = %d, want nil without an upstream", *ahead)
	}
}

func TestAheadCountCountsCommitsSinceTheUpstream(t *testing.T) {
	clone := makeClone(t, makeRepo(t))
	git := Real()

	assertAhead(t, git.AheadCount(clone), 0)
	commit(t, clone, "b.txt", "two\n", "second")
	assertAhead(t, git.AheadCount(clone), 1)
	commit(t, clone, "c.txt", "three\n", "third")
	assertAhead(t, git.AheadCount(clone), 2)
}

func assertAhead(t *testing.T, got *int, want int) {
	t.Helper()

	if got == nil {
		t.Fatalf("expected ahead %d, got nil", want)
	}
	if *got != want {
		t.Fatalf("ahead = %d, want %d", *got, want)
	}
}

func TestDirtyFileCountCountsModifiedAndUntrackedFiles(t *testing.T) {
	repo := makeRepo(t)
	git := Real()

	if dirty := git.DirtyFileCount(repo); dirty != 0 {
		t.Fatalf("dirty = %d, want 0 in a clean repo", dirty)
	}

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("modifying a.txt: %v", err)
	}
	if dirty := git.DirtyFileCount(repo); dirty != 1 {
		t.Fatalf("dirty = %d, want 1 after a modification", dirty)
	}

	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("writing untracked.txt: %v", err)
	}
	if dirty := git.DirtyFileCount(repo); dirty != 2 {
		t.Fatalf("dirty = %d, want 2 with an untracked file as well", dirty)
	}
}

func TestIsRepoDistinguishesARepoFromAPlainDirectory(t *testing.T) {
	git := Real()

	if !git.IsRepo(makeRepo(t)) {
		t.Fatal("a repo did not read as a repo")
	}
	if git.IsRepo(tempDir(t)) {
		t.Fatal("a plain directory read as a repo")
	}
}

func TestEveryWrapperDegradesQuietly(t *testing.T) {
	missing := filepath.Join(tempDir(t), "does", "not", "exist")

	for name, dir := range map[string]string{"a plain directory": tempDir(t), "a missing path": missing} {
		t.Run(name, func(t *testing.T) {
			git := Real()

			if git.IsRepo(dir) {
				t.Error("IsRepo = true, want false")
			}
			if date := git.HeadCommitDate(dir); date != nil {
				t.Errorf("HeadCommitDate = %s, want nil", date)
			}
			if branch := git.CurrentBranch(dir); branch != nil {
				t.Errorf("CurrentBranch = %q, want nil", *branch)
			}
			if ahead := git.AheadCount(dir); ahead != nil {
				t.Errorf("AheadCount = %d, want nil", *ahead)
			}
			if dirty := git.DirtyFileCount(dir); dirty != 0 {
				t.Errorf("DirtyFileCount = %d, want 0", dirty)
			}
		})
	}
}

func TestARepoWithNoCommitsHasNoHeadDate(t *testing.T) {
	dir := tempDir(t)
	run(t, dir, "init", "-b", "main")
	git := Real()

	if !git.IsRepo(dir) {
		t.Error("a freshly initialised repo did not read as a repo")
	}
	if date := git.HeadCommitDate(dir); date != nil {
		t.Errorf("HeadCommitDate = %s, want nil before the first commit", date)
	}
	if ahead := git.AheadCount(dir); ahead != nil {
		t.Errorf("AheadCount = %d, want nil before the first commit", *ahead)
	}
}
