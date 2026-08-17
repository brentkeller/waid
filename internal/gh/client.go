package gh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// Timeout bounds any single gh invocation, so a hanging network call cannot stall a command.
const Timeout = 10 * time.Second

// SearchLimit caps each search, well above a plausible number of open pull requests for one person.
const SearchLimit = 100

// searchQuery runs through GraphQL rather than `gh search prs` for one reason: --json on the search
// command cannot report a head branch, and GraphQL can, at the same cost of one request per query.
const searchQuery = `query($q: String!, $limit: Int!) {
  search(query: $q, type: ISSUE, first: $limit) {
    nodes {
      ... on PullRequest {
        number
        title
        headRefName
        isDraft
        state
        createdAt
        url
        repository { nameWithOwner }
        author { login }
      }
    }
  }
}`

var whitespace = regexp.MustCompile(`\s+`)

// Real is the production Client, backed by `gh api graphql`. Both queries fail rather than degrade,
// because Fetch is what turns a failure into an unavailable result.
func Real() Client { return real{} }

type real struct{}

func (real) ReviewRequested() ([]Pr, error) { return search("is:pr is:open review-requested:@me") }

func (real) Authored() ([]Pr, error) { return search("is:pr is:open author:@me") }

// search runs one account-wide query. Account-wide rather than per-repo on purpose: detection costs
// two calls regardless of how many repos are active.
func search(query string) ([]Pr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	command := exec.CommandContext(ctx, "gh",
		"api", "graphql",
		// Sent on one line so a failure message, which echoes the command, stays one line too.
		"-f", "query="+whitespace.ReplaceAllString(searchQuery, " "),
		"-f", "q="+query,
		"-F", "limit="+strconv.Itoa(SearchLimit),
	)

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, &Failure{
			Message: fmt.Sprintf("Command failed: gh api graphql: %s", err),
			Stderr:  stderr.String(),
		}
	}
	return narrowSearch(stdout.Bytes()), nil
}

// Fixture is a Client served from a recorded response file rather than the network, so detection can
// be diffed against fixed data. The file holds a recorded `gh api graphql` response per query:
//
//	{"reviewRequested": {"data": …}, "authored": {"data": …}}
//
// An "error" key instead records a gh that could not answer, so the degraded path is reachable too.
func Fixture(path string) Client { return fixture{path: path} }

type fixture struct{ path string }

func (f fixture) ReviewRequested() ([]Pr, error) { return f.query("reviewRequested") }

func (f fixture) Authored() ([]Pr, error) { return f.query("authored") }

func (f fixture) query(name string) ([]Pr, error) {
	raw, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("reading the gh fixture: %w", err)
	}

	document, ok := decodeRecord(raw)
	if !ok {
		return nil, fmt.Errorf("the gh fixture at %s is not a JSON object", f.path)
	}
	if message, recorded := document["error"].(string); recorded {
		return nil, &Failure{Message: message, Stderr: message}
	}

	response, recorded := document[name]
	if !recorded {
		return nil, fmt.Errorf("the gh fixture at %s records no %s response", f.path, name)
	}
	return narrowSearchDocument(response), nil
}
