package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/sessions"
)

// sessionProbe is a registry holding one command that reports the session ids dispatch handed it,
// which is the only observable difference between a run that synced and one that did not.
func sessionProbe(needsSessions bool) Registry {
	return Registry{"probe": Module[[]string]{
		Run: func(ctx *Ctx) ([]string, error) {
			ids := []string{}
			for _, session := range ctx.Sessions {
				ids = append(ids, session.Id)
			}
			return ids, nil
		},
		Render:        func(ids []string, ctx *Ctx) string { return strings.Join(ids, ",") },
		NeedsSessions: needsSessions,
	}}
}

// seedTranscript writes a one-prompt transcript under the home's Claude directory and returns the
// session id it carries.
func seedTranscript(t *testing.T, home, id string) string {
	t.Helper()

	dir := filepath.Join(home, "claude", "projects", "C--dev-waid")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}

	line := map[string]any{
		"type":      "user",
		"sessionId": id,
		"cwd":       `C:\dev\waid`,
		"timestamp": "2026-08-14T09:00:00.000Z",
		"message":   map[string]any{"role": "user", "content": "Do the thing"},
	}
	raw, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("encoding a transcript line: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	return id
}

// seedSessionCache writes cache/sessions.json holding one session, stamped the given age.
func seedSessionCache(t *testing.T, home, id string, age time.Duration) {
	t.Helper()

	cache := sessions.Cache{
		Version:  sessions.CacheVersion,
		SyncedAt: time.Now().Add(-age).UTC().Format("2006-01-02T15:04:05.000Z"),
		Sessions: []sessions.CachedSession{{
			Session: sessions.Session{Id: id, Title: "cached", Prompts: 3},
			File:    sessions.FileStat{Path: `C:\transcripts\cached.jsonl`, MtimeNs: 1, Size: 1},
		}},
	}
	raw, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("encoding the cache: %v", err)
	}

	dir := filepath.Join(home, "cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), raw, 0o644); err != nil {
		t.Fatalf("writing the cache: %v", err)
	}
}

// seedClaudeDir points config.json at the transcripts inside the home, so a sync reads the test's
// own files rather than the developer's.
func seedClaudeDir(t *testing.T, home string) {
	t.Helper()

	settings := map[string]any{
		"scanRoots":         []string{},
		"claudeDir":         filepath.Join(home, "claude"),
		"activeWindowDays":  30,
		"ghUser":            nil,
		"ghCacheTtlMinutes": 15,
		"scanMaxDepth":      4,
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("encoding config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.json"), raw, 0o644); err != nil {
		t.Fatalf("writing config.json: %v", err)
	}
}

func runProbe(t *testing.T, home string, needsSessions bool, args ...string) string {
	t.Helper()

	c := &capture{}
	code := Run(argv(home, append([]string{"probe"}, args...)...), c.io(), sessionProbe(needsSessions))
	if code != ExitOK {
		t.Fatalf("probe exited %d: %s", code, c.err.String())
	}
	return strings.TrimSpace(c.out.String())
}

func TestSessionsAreSyncedWhenTheCacheIsMissing(t *testing.T) {
	home := testHome(t)
	seedClaudeDir(t, home)
	id := seedTranscript(t, home, "aaaa1111-2222-3333-4444-555555555555")

	if got := runProbe(t, home, true); got != id {
		t.Fatalf("sessions = %q, want %q", got, id)
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "sessions.json")); err != nil {
		t.Fatalf("the sync left no cache behind: %v", err)
	}
}

func TestSessionsAreSyncedWhenTheCacheIsStale(t *testing.T) {
	home := testHome(t)
	seedClaudeDir(t, home)
	id := seedTranscript(t, home, "aaaa1111-2222-3333-4444-555555555555")
	seedSessionCache(t, home, "stale", (SyncStaleMinutes+1)*time.Minute)

	if got := runProbe(t, home, true); got != id {
		t.Fatalf("sessions = %q, want %q", got, id)
	}
}

func TestAFreshCacheIsUsedAsIs(t *testing.T) {
	home := testHome(t)
	seedClaudeDir(t, home)
	seedTranscript(t, home, "aaaa1111-2222-3333-4444-555555555555")
	seedSessionCache(t, home, "fresh", time.Minute)

	if got := runProbe(t, home, true); got != "fresh" {
		t.Fatalf("sessions = %q, want the cached one", got)
	}
}

func TestNoSyncKeepsAStaleCache(t *testing.T) {
	home := testHome(t)
	seedClaudeDir(t, home)
	seedTranscript(t, home, "aaaa1111-2222-3333-4444-555555555555")
	seedSessionCache(t, home, "stale", time.Hour)

	if got := runProbe(t, home, true, "--no-sync"); got != "stale" {
		t.Fatalf("sessions = %q, want the cached one", got)
	}
}

func TestACommandNotNeedingSessionsGetsNone(t *testing.T) {
	home := testHome(t)
	seedClaudeDir(t, home)
	seedTranscript(t, home, "aaaa1111-2222-3333-4444-555555555555")

	if got := runProbe(t, home, false); got != "" {
		t.Fatalf("sessions = %q, want none", got)
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "sessions.json")); !os.IsNotExist(err) {
		t.Fatalf("a command not needing sessions synced anyway")
	}
}
