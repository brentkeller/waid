// Package gh queries GitHub through the gh CLI and caches what it returns.
package gh

// Pr is a pull request reduced to the fields waid renders. gh's JSON carries far more and is
// narrowed into this shape rather than trusted, so a schema change upstream cannot reach the
// renderers.
type Pr struct {
	Number int `json:"number"`
	// Repository is `owner/repo` — the form the signal key embeds.
	Repository string `json:"repository"`
	Title      string `json:"title"`
	// Author is the login of the PR author.
	Author string `json:"author"`
	// IsDraft stands in for a review state: the search exposes no review decision, and a richer
	// state would cost a per-PR API call.
	IsDraft   bool   `json:"isDraft"`
	State     string `json:"state"`
	CreatedAt string `json:"createdAt"`
	Url       string `json:"url"`
	// Branch is the head branch the PR merges from; empty when the search did not report one.
	Branch string `json:"branch"`
}

// Client is the pair of account-wide queries detection needs, declared as an interface so tests
// inject a fake. Unlike git.Client these may fail, and the caller turns a failure into a note.
type Client interface {
	// ReviewRequested lists open PRs waiting on my review.
	ReviewRequested() ([]Pr, error)
	// Authored lists open PRs I authored.
	Authored() ([]Pr, error)
}
