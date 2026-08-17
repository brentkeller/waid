package gh

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
)

// CacheVersion is bumped whenever a cache written by an older waid can no longer be reused.
const CacheVersion = 2

// reasonLimit caps a failure reason so it stays on one dim note line.
const reasonLimit = 200

// Cache is cache/gh.json as written. Disposable: deleting it costs two API calls, nothing more.
type Cache struct {
	Version         int    `json:"version"`
	CachedAt        string `json:"cachedAt"`
	ReviewRequested []Pr   `json:"reviewRequested"`
	Authored        []Pr   `json:"authored"`
}

// Result is what detection sees: the two pull request lists, or an explanation of why GitHub could
// not answer.
type Result struct {
	Available bool `json:"available"`
	// Reason is set only when Available is false, and is short enough for a single dim note line.
	Reason          string `json:"reason,omitempty"`
	ReviewRequested []Pr   `json:"reviewRequested"`
	Authored        []Pr   `json:"authored"`
	// CachedAt is when the returned lists were fetched; nil when nothing was fetched.
	CachedAt *string `json:"cachedAt"`
}

// Deps are the collaborators Fetch runs against. A nil Client reaches for Real.
type Deps struct {
	Client Client
	Now    time.Time
}

// Failure is a gh failure carrying the command's own stderr, which names the cause far better than
// the exit status or the echoed command line does.
type Failure struct {
	Message string
	Stderr  string
}

func (f *Failure) Error() string { return f.Message }

// Fetch returns the two pull request lists, served from cache/gh.json while it is younger than
// GhCacheTtlMinutes and refetched otherwise. A missing, unauthenticated, timed-out or erroring gh
// yields an unavailable result with a reason and leaves any existing cache untouched; nothing here
// fails loudly.
func Fetch(cfg config.Config, deps Deps) Result {
	if cfg.GhUser == nil {
		return unavailable("ghUser is not set in config.json, so GitHub signals are off")
	}

	if cached := readCache(cfg); cached != nil && ageMinutes(cached.CachedAt, deps.Now) < float64(cfg.GhCacheTtlMinutes) {
		cachedAt := cached.CachedAt
		return Result{
			Available:       true,
			ReviewRequested: cached.ReviewRequested,
			Authored:        cached.Authored,
			CachedAt:        &cachedAt,
		}
	}

	client := deps.Client
	if client == nil {
		client = Real()
	}

	reviewRequested, err := client.ReviewRequested()
	if err != nil {
		return unavailable(describe(err))
	}
	authored, err := client.Authored()
	if err != nil {
		return unavailable(describe(err))
	}

	cache := Cache{
		Version:         CacheVersion,
		CachedAt:        events.FormatTs(deps.Now),
		ReviewRequested: prsOrEmpty(reviewRequested),
		Authored:        prsOrEmpty(authored),
	}
	writeCache(cfg, cache)

	return Result{
		Available:       true,
		ReviewRequested: cache.ReviewRequested,
		Authored:        cache.Authored,
		CachedAt:        &cache.CachedAt,
	}
}

// CacheAgeMinutes is the whole minutes since the cache was written, or nil when there is no readable
// cache or its timestamp cannot be parsed.
func CacheAgeMinutes(cfg config.Config, now time.Time) *int {
	cached := readCache(cfg)
	if cached == nil {
		return nil
	}
	age := ageMinutes(cached.CachedAt, now)
	if math.IsInf(age, 0) {
		return nil
	}
	whole := int(math.Floor(age))
	return &whole
}

// ageMinutes is the minutes since cachedAt, or +Inf when it is unparseable, which forces a refetch.
func ageMinutes(cachedAt string, now time.Time) float64 {
	at, err := time.Parse(time.RFC3339, cachedAt)
	if err != nil {
		return math.Inf(1)
	}
	return math.Max(0, now.Sub(at).Minutes())
}

func unavailable(reason string) Result {
	return Result{Available: false, Reason: reason, ReviewRequested: []Pr{}, Authored: []Pr{}}
}

// readCache reads cache/gh.json. A missing, corrupt or foreign-version cache reads as absent — a
// miss, never an error.
func readCache(cfg config.Config) *Cache {
	raw, err := os.ReadFile(cfg.GhCachePath)
	if err != nil {
		return nil
	}

	parsed, ok := decodeRecord(raw)
	if !ok || !isNumber(parsed["version"], CacheVersion) {
		return nil
	}
	cachedAt, ok := parsed["cachedAt"].(string)
	if !ok {
		return nil
	}

	return &Cache{
		Version:         CacheVersion,
		CachedAt:        cachedAt,
		ReviewRequested: narrowPrs(parsed["reviewRequested"]),
		Authored:        narrowPrs(parsed["authored"]),
	}
}

// writeCache stores the fetched lists. An unwritable cache costs freshness, not correctness: the
// fetched lists still return, so the failure is swallowed.
func writeCache(cfg config.Config, cache Cache) {
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return
	}

	// Encoded the way JSON.stringify(cache, null, 2) writes it, down to `<`, `>` and `&` staying raw
	// and the trailing newline, so either build reads back what the other wrote.
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cache); err != nil {
		return
	}
	os.WriteFile(cfg.GhCachePath, buf.Bytes(), 0o644)
}

// describe is the first line of a failure. gh's own stderr is preferred over the message, which
// only echoes the command that failed.
func describe(err error) string {
	stderr := ""
	var failure *Failure
	if errors.As(err, &failure) {
		stderr = failure.Stderr
	}

	text := firstLine(stderr)
	if text == "" {
		text = firstLine(err.Error())
	}
	if text == "" {
		return "gh failed"
	}
	if runes := []rune(text); len(runes) > reasonLimit {
		return string(runes[:reasonLimit-1]) + "…"
	}
	return text
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func prsOrEmpty(prs []Pr) []Pr {
	if prs == nil {
		return []Pr{}
	}
	return prs
}
