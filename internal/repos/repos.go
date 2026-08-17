// Package repos discovers the git repos under the configured scan roots and gates them on recent
// activity, so detection probes only the repos worth probing.
package repos

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/git"
	"github.com/brentkeller/waid/internal/sessions"
)

// skipDirs are directory names never walked: build output, vendored trees, and git's own internals.
var skipDirs = map[string]struct{}{
	"node_modules": {},
	"bin":          {},
	"obj":          {},
	".git":         {},
}

// ActiveOptions carries the seams Active runs against.
type ActiveOptions struct {
	// Now is the instant the activity window is measured back from.
	Now time.Time
	// Git probes HEAD dates; nil reaches for the real client.
	Git git.Client
	// Repos short-circuits discovery; nil discovers under the config's scan roots.
	Repos []string
}

// Discover walks each scan root to ScanMaxDepth looking for a .git entry, in sorted order. A
// directory that is a repo is reported and not descended into, so a vendored repo inside a repo is
// never double-reported. Unreadable or missing roots are skipped rather than failing the scan.
func Discover(cfg config.Config) []string {
	var found []string
	seen := map[string]struct{}{}

	var visit func(dir string, depth int)
	visit = func(dir string, depth int) {
		if isRepoRoot(dir) {
			key := compareKey(dir)
			if _, already := seen[key]; !already {
				seen[key] = struct{}{}
				found = append(found, dir)
			}
			return
		}
		if depth >= cfg.ScanMaxDepth {
			return
		}
		for _, name := range childDirectories(dir) {
			if _, skip := skipDirs[strings.ToLower(name)]; skip {
				continue
			}
			visit(filepath.Join(dir, name), depth+1)
		}
	}

	for _, root := range cfg.ScanRoots {
		visit(root, 0)
	}
	return found
}

// Active returns the discovered repos worth scanning: those a harvested session ran inside within
// ActiveWindowDays, or whose HEAD commit falls in the same window. Order follows discovery.
func Active(cfg config.Config, harvested []sessions.CachedSession, opts ActiveOptions) []string {
	discovered := opts.Repos
	if discovered == nil {
		discovered = Discover(cfg)
	}
	if len(discovered) == 0 {
		return nil
	}

	client := opts.Git
	if client == nil {
		client = git.Real()
	}
	cutoff := opts.Now.Add(-time.Duration(cfg.ActiveWindowDays) * 24 * time.Hour)

	recentlyUsed := map[string]struct{}{}
	for _, session := range harvested {
		if session.Project == nil {
			continue
		}
		at, ok := sessionTime(session)
		if !ok || at.Before(cutoff) {
			continue
		}
		if repo := ForPath(*session.Project, discovered); repo != "" {
			recentlyUsed[repo] = struct{}{}
		}
	}

	var active []string
	for _, repo := range discovered {
		if _, used := recentlyUsed[repo]; used {
			active = append(active, repo)
			continue
		}
		if head := client.HeadCommitDate(repo); head != nil && !head.Before(cutoff) {
			active = append(active, repo)
		}
	}
	return active
}

// ForPath returns the repo p lies inside, preferring the longest match so a repo vendored inside
// another wins, and an empty string when p lies inside none. Matching is separator- and (on
// Windows) case-insensitive, and only at a path boundary.
func ForPath(p string, repos []string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	target := compareKey(p)

	best := ""
	bestLength := -1
	for _, repo := range repos {
		key := compareKey(repo)
		if key == "" {
			continue
		}
		if target != key && !strings.HasPrefix(target, key+"/") {
			continue
		}
		if len(key) > bestLength {
			best = repo
			bestLength = len(key)
		}
	}
	return best
}

// sessionTime is when a session last showed activity, preferring its end; ok is false when neither
// timestamp parses.
func sessionTime(session sessions.CachedSession) (at time.Time, ok bool) {
	stamp := session.Ended
	if stamp == nil {
		stamp = session.Started
	}
	if stamp == nil {
		return time.Time{}, false
	}

	parsed, err := time.Parse(time.RFC3339, *stamp)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// isRepoRoot reports whether dir holds a .git entry — a directory in a normal clone, a file in a
// worktree.
func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// childDirectories are the sorted subdirectory names of dir, or nothing when it is missing or
// unreadable.
func childDirectories(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names
}

// compareKey reduces a path to a comparable form: forward separators, no trailing separator, and on
// Windows case-folded.
func compareKey(p string) string {
	unified := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(p), `\`, "/"), "/")
	if runtime.GOOS == "windows" {
		return strings.ToLower(unified)
	}
	return unified
}
