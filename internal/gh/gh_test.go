package gh

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
)

var now = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

// testConfig is a config over a fresh home with the cache directory already in place.
func testConfig(t *testing.T, apply func(*config.Config)) config.Config {
	t.Helper()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
		t.Fatalf("creating the cache directory: %v", err)
	}
	cfg, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading the config: %v", err)
	}

	user := "octocat"
	cfg.GhUser = &user
	cfg.GhCacheTtlMinutes = 15
	if apply != nil {
		apply(&cfg)
	}
	return cfg
}

func pr(number int, apply func(*Pr)) Pr {
	item := Pr{
		Number:     number,
		Repository: "octo/widgets",
		Title:      "pull request " + strconv.Itoa(number),
		Author:     "octocat",
		IsDraft:    false,
		State:      "open",
		CreatedAt:  "2026-08-10T09:00:00Z",
		Url:        "https://github.com/octo/widgets/pull/" + strconv.Itoa(number),
		Branch:     "branch-" + strconv.Itoa(number),
	}
	if apply != nil {
		apply(&item)
	}
	return item
}

// countingClient records how often each query ran, so a cache hit is observable.
type countingClient struct {
	reviewRequested []Pr
	authored        []Pr
	reviewCalls     int
	authoredCalls   int
}

func (c *countingClient) ReviewRequested() ([]Pr, error) {
	c.reviewCalls++
	return c.reviewRequested, nil
}

func (c *countingClient) Authored() ([]Pr, error) {
	c.authoredCalls++
	return c.authored, nil
}

// failingClient fails both queries with the same error.
type failingClient struct{ err error }

func (c failingClient) ReviewRequested() ([]Pr, error) { return nil, c.err }
func (c failingClient) Authored() ([]Pr, error)        { return nil, c.err }

func writeCacheFile(t *testing.T, cfg config.Config, minutesOld int, reviewRequested, authored []Pr) {
	t.Helper()

	cache := Cache{
		Version:         CacheVersion,
		CachedAt:        events.FormatTs(now.Add(-time.Duration(minutesOld) * time.Minute)),
		ReviewRequested: reviewRequested,
		Authored:        authored,
	}
	encoded, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		t.Fatalf("encoding the cache: %v", err)
	}
	writeRaw(t, cfg, string(encoded)+"\n")
}

func writeRaw(t *testing.T, cfg config.Config, contents string) {
	t.Helper()

	if err := os.WriteFile(cfg.GhCachePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing the cache: %v", err)
	}
}

func numbers(items []Pr) []int {
	out := []int{}
	for _, item := range items {
		out = append(out, item.Number)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func TestCacheYoungerThanTtlSkipsGh(t *testing.T) {
	cfg := testConfig(t, nil)
	writeCacheFile(t, cfg, 5, []Pr{pr(1, nil)}, []Pr{pr(2, nil)})
	client := &countingClient{reviewRequested: []Pr{pr(9, nil)}, authored: []Pr{pr(9, nil)}}

	result := Fetch(cfg, Deps{Client: client, Now: now})

	if !result.Available {
		t.Fatalf("Available = false, want true (reason %q)", result.Reason)
	}
	if client.reviewCalls != 0 || client.authoredCalls != 0 {
		t.Errorf("calls = %d/%d, want 0/0", client.reviewCalls, client.authoredCalls)
	}
	if got := numbers(result.ReviewRequested); !equalInts(got, []int{1}) {
		t.Errorf("reviewRequested = %v, want [1]", got)
	}
	if got := numbers(result.Authored); !equalInts(got, []int{2}) {
		t.Errorf("authored = %v, want [2]", got)
	}
	want := "2026-08-14T11:55:00.000Z"
	if result.CachedAt == nil || *result.CachedAt != want {
		t.Errorf("cachedAt = %v, want %q", result.CachedAt, want)
	}
}

func TestExpiredCacheFetchesOncePerQueryAndRewrites(t *testing.T) {
	cfg := testConfig(t, nil)
	writeCacheFile(t, cfg, 60, []Pr{pr(1, nil)}, []Pr{pr(2, nil)})
	client := &countingClient{reviewRequested: []Pr{pr(1, nil)}, authored: []Pr{pr(2, nil), pr(3, nil)}}

	result := Fetch(cfg, Deps{Client: client, Now: now})

	if !result.Available {
		t.Fatalf("Available = false, want true")
	}
	if client.reviewCalls != 1 || client.authoredCalls != 1 {
		t.Errorf("calls = %d/%d, want 1/1", client.reviewCalls, client.authoredCalls)
	}
	if result.CachedAt == nil || *result.CachedAt != "2026-08-14T12:00:00.000Z" {
		t.Errorf("cachedAt = %v, want the pinned now", result.CachedAt)
	}

	raw, err := os.ReadFile(cfg.GhCachePath)
	if err != nil {
		t.Fatalf("reading the cache: %v", err)
	}
	var written Cache
	if err := json.Unmarshal(raw, &written); err != nil {
		t.Fatalf("decoding the cache: %v", err)
	}
	if written.Version != CacheVersion {
		t.Errorf("version = %d, want %d", written.Version, CacheVersion)
	}
	if written.CachedAt != "2026-08-14T12:00:00.000Z" {
		t.Errorf("cachedAt = %q, want the pinned now", written.CachedAt)
	}
	if got := numbers(written.ReviewRequested); !equalInts(got, []int{1}) {
		t.Errorf("written reviewRequested = %v, want [1]", got)
	}
	if got := numbers(written.Authored); !equalInts(got, []int{2, 3}) {
		t.Errorf("written authored = %v, want [2 3]", got)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Error("the cache file does not end in a newline")
	}
}

func TestTheCacheIsWrittenTheWayNodeWritesIt(t *testing.T) {
	cfg := testConfig(t, nil)
	titled := pr(1, func(item *Pr) { item.Title = "drop <script> & tighten" })

	Fetch(cfg, Deps{Client: &countingClient{reviewRequested: []Pr{titled}}, Now: now})

	raw, err := os.ReadFile(cfg.GhCachePath)
	if err != nil {
		t.Fatalf("reading the cache: %v", err)
	}
	if !strings.Contains(string(raw), `"title": "drop <script> & tighten"`) {
		t.Errorf("the cache escapes characters JSON.stringify leaves raw:\n%s", raw)
	}
	if !strings.Contains(string(raw), "\n  \"version\": 2,") {
		t.Errorf("the cache is not indented two spaces:\n%s", raw)
	}
}

func TestMissingCacheFetchesAndWritesOne(t *testing.T) {
	cfg := testConfig(t, nil)
	client := &countingClient{reviewRequested: []Pr{pr(1, nil)}}

	result := Fetch(cfg, Deps{Client: client, Now: now})

	if !result.Available {
		t.Fatalf("Available = false, want true")
	}
	if client.reviewCalls != 1 {
		t.Errorf("reviewRequested calls = %d, want 1", client.reviewCalls)
	}
	if _, err := os.Stat(cfg.GhCachePath); err != nil {
		t.Errorf("no cache written: %v", err)
	}
}

func TestFailingClientReportsUnavailableAndLeavesTheCache(t *testing.T) {
	cfg := testConfig(t, nil)
	writeCacheFile(t, cfg, 60, []Pr{pr(1, nil)}, []Pr{pr(2, nil)})
	before, err := os.ReadFile(cfg.GhCachePath)
	if err != nil {
		t.Fatalf("reading the cache: %v", err)
	}

	result := Fetch(cfg, Deps{Client: failingClient{err: errors.New("gh: not logged in")}, Now: now})

	if result.Available {
		t.Fatal("Available = true, want false")
	}
	if !strings.Contains(result.Reason, "not logged in") {
		t.Errorf("reason = %q, want it to mention the login failure", result.Reason)
	}
	if len(result.ReviewRequested) != 0 || len(result.Authored) != 0 {
		t.Errorf("lists = %v/%v, want both empty", result.ReviewRequested, result.Authored)
	}
	if result.CachedAt != nil {
		t.Errorf("cachedAt = %v, want nil", *result.CachedAt)
	}

	after, err := os.ReadFile(cfg.GhCachePath)
	if err != nil {
		t.Fatalf("re-reading the cache: %v", err)
	}
	if string(after) != string(before) {
		t.Error("the cache was rewritten by a failed fetch")
	}
}

func TestFailureReasonPrefersStderrOverTheEchoedCommand(t *testing.T) {
	cfg := testConfig(t, nil)
	failure := &Failure{
		Message: "Command failed: gh api graphql -f query=...",
		Stderr:  "\ngh: API rate limit exceeded\n",
	}

	result := Fetch(cfg, Deps{Client: failingClient{err: failure}, Now: now})

	if result.Available {
		t.Fatal("Available = true, want false")
	}
	if result.Reason != "gh: API rate limit exceeded" {
		t.Errorf("reason = %q, want the stderr line", result.Reason)
	}
}

func TestALongFailureReasonIsTruncated(t *testing.T) {
	cfg := testConfig(t, nil)
	long := strings.Repeat("x", 400)

	result := Fetch(cfg, Deps{Client: failingClient{err: errors.New(long)}, Now: now})

	if runes := []rune(result.Reason); len(runes) != 200 || runes[199] != '…' {
		t.Errorf("reason has %d runes ending %q, want 200 ending in an ellipsis", len(runes), string(runes[len(runes)-1]))
	}
}

func TestCorruptCacheIsAMissNotAnError(t *testing.T) {
	cfg := testConfig(t, nil)
	writeRaw(t, cfg, "{not json at all")
	client := &countingClient{reviewRequested: []Pr{pr(4, nil)}}

	result := Fetch(cfg, Deps{Client: client, Now: now})

	if !result.Available {
		t.Fatalf("Available = false, want true")
	}
	if client.reviewCalls != 1 {
		t.Errorf("reviewRequested calls = %d, want 1", client.reviewCalls)
	}
	if got := numbers(result.ReviewRequested); !equalInts(got, []int{4}) {
		t.Errorf("reviewRequested = %v, want [4]", got)
	}
}

func TestCacheFromAnotherVersionIsAMiss(t *testing.T) {
	cfg := testConfig(t, nil)
	writeRaw(t, cfg, `{"version":`+strconv.Itoa(CacheVersion+1)+`,"cachedAt":"2026-08-14T12:00:00.000Z","reviewRequested":[],"authored":[]}`)
	client := &countingClient{reviewRequested: []Pr{pr(5, nil)}}

	result := Fetch(cfg, Deps{Client: client, Now: now})

	if client.reviewCalls != 1 {
		t.Errorf("reviewRequested calls = %d, want 1", client.reviewCalls)
	}
	if got := numbers(result.ReviewRequested); !equalInts(got, []int{5}) {
		t.Errorf("reviewRequested = %v, want [5]", got)
	}
}

func TestNoGhUserShortCircuits(t *testing.T) {
	cfg := testConfig(t, func(cfg *config.Config) { cfg.GhUser = nil })
	client := &countingClient{reviewRequested: []Pr{pr(1, nil)}, authored: []Pr{pr(2, nil)}}

	result := Fetch(cfg, Deps{Client: client, Now: now})

	if result.Available {
		t.Fatal("Available = true, want false")
	}
	if !strings.Contains(result.Reason, "ghUser") {
		t.Errorf("reason = %q, want it to name ghUser", result.Reason)
	}
	if client.reviewCalls != 0 || client.authoredCalls != 0 {
		t.Errorf("calls = %d/%d, want 0/0", client.reviewCalls, client.authoredCalls)
	}
	if result.CachedAt != nil {
		t.Errorf("cachedAt = %v, want nil", *result.CachedAt)
	}
	if _, err := os.Stat(cfg.GhCachePath); err == nil {
		t.Error("a cache was written despite gh never being queried")
	}
}

func TestBranchComesFromEitherGraphqlOrTheCache(t *testing.T) {
	cfg := testConfig(t, nil)
	writeRaw(t, cfg, `{
  "version": 2,
  "cachedAt": "2026-08-14T12:00:00.000Z",
  "reviewRequested": [
    {"number": 1, "title": "one", "repository": "octo/widgets", "headRefName": "from-graphql"},
    {"number": 2, "title": "two", "repository": "octo/widgets", "branch": "from-cache"},
    {"number": 3, "title": "three", "repository": "octo/widgets"}
  ],
  "authored": []
}`)

	result := Fetch(cfg, Deps{Client: &countingClient{}, Now: now})

	want := []string{"from-graphql", "from-cache", ""}
	for index, branch := range want {
		if index >= len(result.ReviewRequested) {
			t.Fatalf("reviewRequested has %d entries, want %d", len(result.ReviewRequested), len(want))
		}
		if got := result.ReviewRequested[index].Branch; got != branch {
			t.Errorf("branch %d = %q, want %q", index, got, branch)
		}
	}
}

func TestCachedEntriesThatAreNotPullRequestsAreDropped(t *testing.T) {
	cfg := testConfig(t, nil)
	writeRaw(t, cfg, `{
  "version": 2,
  "cachedAt": "2026-08-14T12:00:00.000Z",
  "reviewRequested": [
    {"number": 1, "title": "one", "repository": "octo/widgets"},
    {"number": "seven", "title": "seven", "repository": "octo/widgets"},
    {"number": 8, "repository": "octo/widgets"},
    {"number": 9, "title": "nine"},
    null
  ],
  "authored": "not an array"
}`)

	result := Fetch(cfg, Deps{Client: &countingClient{}, Now: now})

	if got := numbers(result.ReviewRequested); !equalInts(got, []int{1}) {
		t.Errorf("reviewRequested = %v, want [1]", got)
	}
	if len(result.Authored) != 0 {
		t.Errorf("authored = %v, want empty", result.Authored)
	}
}

func TestCacheAgeMinutes(t *testing.T) {
	cfg := testConfig(t, nil)
	if age := CacheAgeMinutes(cfg, now); age != nil {
		t.Errorf("age without a cache = %d, want nil", *age)
	}

	writeCacheFile(t, cfg, 90, nil, nil)
	age := CacheAgeMinutes(cfg, now)
	if age == nil || *age != 90 {
		t.Errorf("age = %v, want 90", age)
	}

	writeRaw(t, cfg, `{"version": 2, "cachedAt": "not a date", "reviewRequested": [], "authored": []}`)
	if age := CacheAgeMinutes(cfg, now); age != nil {
		t.Errorf("age of an unparseable cachedAt = %d, want nil", *age)
	}
}

func TestNarrowingARecordedSearchResponse(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "search-response.json"))
	if err != nil {
		t.Fatalf("reading the recorded response: %v", err)
	}

	prs := narrowSearch(raw)

	if len(prs) != 2 {
		t.Fatalf("narrowed %d pull requests, want 2 (the third node is not a PR)", len(prs))
	}
	want := Pr{
		Number:     41,
		Repository: "octo/widgets",
		Title:      "Tighten the retry budget",
		Author:     "octocat",
		IsDraft:    false,
		State:      "OPEN",
		CreatedAt:  "2026-08-11T16:04:12Z",
		Url:        "https://github.com/octo/widgets/pull/41",
		Branch:     "retry-budget",
	}
	if prs[0] != want {
		t.Errorf("first pull request =\n  %+v\nwant\n  %+v", prs[0], want)
	}
	if !prs[1].IsDraft {
		t.Error("the second pull request should be a draft")
	}
	if prs[1].Branch != "" {
		t.Errorf("branch = %q, want empty when the response omits it", prs[1].Branch)
	}
}

func TestNarrowingAMalformedSearchResponse(t *testing.T) {
	for _, raw := range []string{"", "not json", "{}", `{"data": {}}`, `{"data": {"search": {"nodes": null}}}`} {
		if prs := narrowSearch([]byte(raw)); len(prs) != 0 {
			t.Errorf("narrowSearch(%q) = %v, want empty", raw, prs)
		}
	}
}

func TestFixtureClientServesTheRecordedQueries(t *testing.T) {
	client := Fixture(filepath.Join("testdata", "fixture.json"))

	review, err := client.ReviewRequested()
	if err != nil {
		t.Fatalf("ReviewRequested: %v", err)
	}
	if got := numbers(review); !equalInts(got, []int{41, 42}) {
		t.Errorf("reviewRequested = %v, want [41 42]", got)
	}

	authored, err := client.Authored()
	if err != nil {
		t.Fatalf("Authored: %v", err)
	}
	if got := numbers(authored); !equalInts(got, []int{7}) {
		t.Errorf("authored = %v, want [7]", got)
	}
}

func TestFixtureClientCanRecordAFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte(`{"error": "gh: not logged in"}`), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	if _, err := Fixture(path).ReviewRequested(); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("error = %v, want the recorded failure", err)
	}
}

func TestFixtureClientReportsAMissingFile(t *testing.T) {
	if _, err := Fixture(filepath.Join(t.TempDir(), "absent.json")).Authored(); err == nil {
		t.Error("a missing fixture should be an error, not an empty result")
	}
}
