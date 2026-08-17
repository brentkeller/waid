// Package git wraps the git subprocesses detection runs.
package git

import "time"

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
