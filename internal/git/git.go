// Package git wraps the git subprocesses detection runs.
package git

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Timeout bounds any single git invocation, so an unreachable path cannot stall a command.
const Timeout = 5 * time.Second

// Client is the git surface detection needs, declared as an interface so tests inject a fake with a
// compiler-checked shape. No method fails loudly: a failed or absent git reads as null, zero or
// false, because a repo that cannot be probed is a repo with nothing to report, not an error.
type Client interface {
	// HeadCommitDate is the commit date of HEAD; nil when the repo has no commits or is not a repo.
	HeadCommitDate(repo string) *time.Time
	// CurrentBranch is the checked-out branch; nil on a detached HEAD or an unborn branch.
	CurrentBranch(repo string) *string
	// AheadCount is the commits on HEAD the upstream lacks; nil when the branch has no upstream.
	AheadCount(repo string) *int
	// DirtyFileCount is the files git status reports, tracked or not.
	DirtyFileCount(repo string) int
	// IsRepo reports whether dir sits inside a git work tree.
	IsRepo(dir string) bool
}

// Real is the production Client, backed by git subprocesses.
func Real() Client { return real{} }

type real struct{}

// output runs `git -C repo …` and returns trimmed stdout. ok is false when git exits non-zero,
// times out, is missing, or the directory does not exist; callers never see an error.
func (real) output(repo string, args ...string) (out string, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	stdout, err := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(stdout)), true
}

func (r real) HeadCommitDate(repo string) *time.Time {
	out, ok := r.output(repo, "log", "-1", "--format=%cI")
	if !ok || out == "" {
		return nil
	}
	date, err := time.Parse(time.RFC3339, out)
	if err != nil {
		return nil
	}
	return &date
}

func (r real) CurrentBranch(repo string) *string {
	// symbolic-ref fails on a detached HEAD, where `rev-parse --abbrev-ref` would say "HEAD".
	out, ok := r.output(repo, "symbolic-ref", "--quiet", "--short", "HEAD")
	if !ok || out == "" {
		return nil
	}
	return &out
}

func (r real) AheadCount(repo string) *int {
	out, ok := r.output(repo, "rev-list", "--count", "@{u}..HEAD")
	if !ok {
		return nil
	}
	count, err := strconv.Atoi(out)
	if err != nil {
		return nil
	}
	return &count
}

func (r real) DirtyFileCount(repo string) int {
	out, ok := r.output(repo, "status", "--porcelain")
	if !ok || out == "" {
		return 0
	}

	dirty := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			dirty++
		}
	}
	return dirty
}

func (r real) IsRepo(dir string) bool {
	out, ok := r.output(dir, "rev-parse", "--is-inside-work-tree")
	return ok && out == "true"
}
