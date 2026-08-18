// Package detect assembles the loops found in git and GitHub rather than declared, ranks them, and
// removes the ones already dismissed.
package detect

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/gh"
	"github.com/brentkeller/waid/internal/git"
	"github.com/brentkeller/waid/internal/pool"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/repos"
	"github.com/brentkeller/waid/internal/sessions"
)

// Kind is one of the four things detection can notice.
type Kind string

const (
	// KindReview is a pull request waiting on my review.
	KindReview Kind = "review"
	// KindPr is an open pull request I authored.
	KindPr Kind = "pr"
	// KindAhead is a branch with commits its upstream lacks.
	KindAhead Kind = "ahead"
	// KindDirty is a repo with uncommitted files.
	KindDirty Kind = "dirty"
)

// kindRank orders the kinds: work blocking someone else outranks my own uncommitted scratch.
var kindRank = map[Kind]int{KindReview: 0, KindPr: 1, KindAhead: 2, KindDirty: 3}

// Signal is a detected loop. Key is the stable identity dismiss and promote address it by —
// `review:owner/repo#123`, `dirty:<repo>` and so on. Fields are declared in the order Node emits
// them, which is the order --json renders.
type Signal struct {
	Key  string `json:"key"`
	Kind Kind   `json:"kind"`
	// Title is a human summary, good enough to become an item title when the signal is promoted.
	Title string `json:"title"`
	// Subject is the middle column: what the signal is about — a PR title, a count of files.
	Subject string `json:"subject"`
	// Detail is the right column: where and when — a branch, an author, a state, the age.
	Detail string `json:"detail"`
	// Branch the signal sits on; nil when there is none, as on a detached HEAD.
	Branch *string `json:"branch"`
	// Project is the absolute repo path the signal belongs to; nil when no local repo matched.
	Project *string `json:"project"`
	// Age is the compact relative age of whatever the signal is timed by — a PR's creation, a repo's
	// HEAD.
	Age string `json:"age"`
}

// Deps are the inputs and seams detection runs against.
type Deps struct {
	// Sessions are the harvested sessions, read by the activity gate.
	Sessions []sessions.CachedSession
	// State is the folded log, read for its dismissed keys.
	State events.State
	Now   time.Time
	// Git probes the local repos; nil reaches for the real client.
	Git git.Client
	// Gh answers the two account-wide queries; nil reaches for the real client.
	Gh gh.Client
}

// Result is everything one detection pass produced.
type Result struct {
	Signals []Signal `json:"signals"`
	// DismissedCount is how many signals were suppressed because their key had been dismissed.
	DismissedCount int `json:"dismissedCount"`
	// Notes are non-fatal degradations, today only an unavailable gh.
	Notes []string `json:"notes"`
	// Repos are the checkouts the pass discovered, quiet ones included. The walk is made anyway to
	// find the signals, so carrying its result is what lets the app name the projects on disk without
	// walking again. It stays out of --json: the commands report signals, and a repo list is a
	// different question they are not asked.
	Repos []string `json:"-"`
}

// candidate is a signal plus the instant it is timed by, which ranks it before the timestamp is
// dropped.
type candidate struct {
	Signal
	at time.Time
}

// Signals assembles every detected signal, ranks them, and removes the ones already dismissed.
// Nothing here fails loudly: an unavailable gh becomes a note and the git signals still return, so
// detection works offline. GitHub is queried account-wide (two calls) while git is queried per
// *active* repo only.
func Signals(cfg config.Config, deps Deps) Result {
	client := deps.Git
	if client == nil {
		client = git.Real()
	}
	// The two GitHub queries are account-wide and independent of the local repos, so they overlap the
	// git pool rather than blocking it.
	fetching := pool.Detached(func() gh.Result {
		return gh.Fetch(cfg, gh.Deps{Client: deps.Gh, Now: deps.Now})
	})

	discovered := repos.Discover(cfg)
	active := repos.Active(cfg, deps.Sessions, repos.ActiveOptions{Now: deps.Now, Git: client, Repos: discovered})
	// Probing is four git subprocesses per repo. They run bounded and concurrently, but the results
	// are collected by repo index rather than by completion, so ranking sees the same input every run.
	probed := pool.Map(pool.Limit, active, func(repo string) []candidate {
		return repoSignals(repo, client, deps.Now)
	})

	fetched := fetching()

	var candidates []candidate
	for _, pr := range fetched.ReviewRequested {
		candidates = append(candidates, reviewSignal(pr, discovered, deps.Now))
	}
	for _, pr := range fetched.Authored {
		candidates = append(candidates, prSignal(pr, discovered, deps.Now))
	}
	for _, found := range probed {
		candidates = append(candidates, found...)
	}
	slices.SortStableFunc(candidates, byRank)

	dismissed := map[string]struct{}{}
	for _, key := range deps.State.Dismissed {
		dismissed[key] = struct{}{}
	}

	signals := []Signal{}
	dismissedCount := 0
	for _, found := range candidates {
		if _, hidden := dismissed[found.Key]; hidden {
			dismissedCount++
			continue
		}
		signals = append(signals, found.Signal)
	}

	notes := []string{}
	if !fetched.Available {
		reason := fetched.Reason
		if reason == "" {
			reason = "gh failed"
		}
		notes = append(notes, fmt.Sprintf("GitHub signals unavailable: %s", reason))
	}
	return Result{Signals: signals, DismissedCount: dismissedCount, Notes: notes, Repos: discovered}
}

// byRank puts kind first, then oldest first — an ancient review request is the most urgent thing on
// the list.
func byRank(a, b candidate) int {
	if rank := kindRank[a.Kind] - kindRank[b.Kind]; rank != 0 {
		return rank
	}
	return a.at.Compare(b.at)
}

func reviewSignal(pr gh.Pr, discovered []string, now time.Time) candidate {
	age := render.RelTime(pr.CreatedAt, now)
	author := ""
	if pr.Author != "" {
		author = "@" + pr.Author
	}

	return candidate{
		Signal: Signal{
			Key:     fmt.Sprintf("review:%s#%d", pr.Repository, pr.Number),
			Kind:    KindReview,
			Title:   pr.Title,
			Subject: pr.Title,
			Detail:  meta(author, pr.Branch, age),
			Branch:  optional(pr.Branch),
			Project: repoForGhRepository(pr.Repository, discovered),
			Age:     age,
		},
		at: instant(pr.CreatedAt, now),
	}
}

func prSignal(pr gh.Pr, discovered []string, now time.Time) candidate {
	age := render.RelTime(pr.CreatedAt, now)
	// The search exposes no review decision, so draft state is the most it can say.
	state := strings.ToLower(pr.State)
	if state == "" {
		state = "open"
	}
	if pr.IsDraft {
		state = "draft"
	}

	return candidate{
		Signal: Signal{
			Key:     fmt.Sprintf("pr:%s#%d", pr.Repository, pr.Number),
			Kind:    KindPr,
			Title:   pr.Title,
			Subject: pr.Title,
			Detail:  meta(pr.Branch, state, age),
			Branch:  optional(pr.Branch),
			Project: repoForGhRepository(pr.Repository, discovered),
			Age:     age,
		},
		at: instant(pr.CreatedAt, now),
	}
}

// repoSignals are the ahead and dirty signals for one active repo, in rank order.
func repoSignals(repo string, client git.Client, now time.Time) []candidate {
	branch := client.CurrentBranch(repo)
	head := client.HeadCommitDate(repo)

	age := "?"
	at := now
	if head != nil {
		age = render.RelTime(events.FormatTs(*head), now)
		at = *head
	}
	name := filepath.Base(repo)
	project := repo

	var found []candidate

	// A nil count covers both "no upstream" and "not a repo"; neither is something to push.
	ahead := client.AheadCount(repo)
	if branch != nil && ahead != nil && *ahead > 0 {
		found = append(found, candidate{
			Signal: Signal{
				Key:     fmt.Sprintf("ahead:%s:%s", repo, *branch),
				Kind:    KindAhead,
				Title:   fmt.Sprintf("%s on %s in %s", plural(*ahead, "unpushed commit"), *branch, name),
				Subject: plural(*ahead, "commit") + " ahead",
				Detail:  meta(*branch, age),
				Branch:  branch,
				Project: &project,
				Age:     age,
			},
			at: at,
		})
	}

	if dirty := client.DirtyFileCount(repo); dirty > 0 {
		branchName := ""
		if branch != nil {
			branchName = *branch
		}
		found = append(found, candidate{
			Signal: Signal{
				Key:     fmt.Sprintf("dirty:%s", repo),
				Kind:    KindDirty,
				Title:   fmt.Sprintf("%s in %s", plural(dirty, "uncommitted file"), name),
				Subject: plural(dirty, "uncommitted file"),
				Detail:  meta(branchName, age),
				Branch:  branch,
				Project: &project,
				Age:     age,
			},
			at: at,
		})
	}

	return found
}

// meta is the right-hand column: the parts that are known, dot-separated, in the order given.
func meta(parts ...string) string {
	known := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			known = append(known, part)
		}
	}
	return strings.Join(known, " · ")
}

// repoForGhRepository is the local checkout a GitHub `owner/repo` refers to, matched on directory
// name. Only an unambiguous match counts — two clones of the same name give no answer rather than an
// arbitrary one.
func repoForGhRepository(repository string, discovered []string) *string {
	parts := strings.Split(repository, "/")
	name := strings.ToLower(parts[len(parts)-1])
	if name == "" {
		return nil
	}

	var match *string
	for index, repo := range discovered {
		if strings.ToLower(filepath.Base(repo)) != name {
			continue
		}
		if match != nil {
			return nil
		}
		match = &discovered[index]
	}
	return match
}

// instant is a timestamp for ranking; an unparseable one sorts as "now", i.e. last within its kind.
func instant(iso string, now time.Time) time.Time {
	at, ok := render.ParseTime(iso)
	if !ok {
		return now
	}
	return at
}

// optional turns an empty string into no value.
func optional(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
