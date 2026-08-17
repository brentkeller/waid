package command_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/command"
)

// syncHome is a home whose transcripts live inside it, so a sync reads the test's own files rather
// than the developer's.
func syncHome(t *testing.T, ids ...string) string {
	t.Helper()

	home := makeHome(t)
	claudeDir := filepath.Join(home, "claude")
	seedConfig(t, home, `{ "scanRoots": [], "claudeDir": `+quote(t, claudeDir)+` }`)

	dir := filepath.Join(claudeDir, "projects", "C--dev-waid")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	for _, id := range ids {
		line := `{"type":"user","sessionId":"` + id + `","cwd":"C:\\dev\\waid",` +
			`"timestamp":"2026-08-14T09:00:00.000Z","message":{"role":"user","content":"Do the thing"}}`
		if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(line+"\n"), 0o644); err != nil {
			t.Fatalf("writing a transcript: %v", err)
		}
	}
	return home
}

// quote renders a path as a JSON string, which is the only safe way to put a Windows path in a
// hand-written config.
func quote(t *testing.T, value string) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("quoting %q: %v", value, err)
	}
	return string(raw)
}

func runSync(t *testing.T, home string, args ...string) command.SyncResult {
	t.Helper()

	run := waid(t, home, append([]string{"sync", "--json"}, args...)...)
	if run.code != cli.ExitOK {
		t.Fatalf("sync exited %d: %s", run.code, run.err)
	}

	var data command.SyncResult
	run.decode(t, &data)
	return data
}

func TestAFirstSyncParsesEveryTranscriptAndLeavesACache(t *testing.T) {
	home := syncHome(t, "aaaa1111-2222-3333-4444-555555555555", "bbbb2222-3333-4444-5555-666666666666")

	data := runSync(t, home)

	if data.Scanned != 2 || data.Parsed != 2 || data.Reused != 0 || data.Removed != 0 {
		t.Errorf("stats = %+v, want two parsed", data)
	}
	if data.Sessions != 2 {
		t.Errorf("sessions = %d, want 2", data.Sessions)
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "sessions.json")); err != nil {
		t.Errorf("the sync left no cache behind: %v", err)
	}
}

func TestASecondSyncReusesTheUnchangedTranscripts(t *testing.T) {
	home := syncHome(t, "aaaa1111-2222-3333-4444-555555555555")
	runSync(t, home)

	data := runSync(t, home)

	if data.Parsed != 0 || data.Reused != 1 {
		t.Errorf("stats = %+v, want the transcript reused", data)
	}
}

func TestFullReParsesEverything(t *testing.T) {
	home := syncHome(t, "aaaa1111-2222-3333-4444-555555555555")
	runSync(t, home)

	data := runSync(t, home, "--full")

	if data.Parsed != 1 || data.Reused != 0 {
		t.Errorf("stats = %+v, want the transcript re-parsed", data)
	}
}

func TestASyncDropsSessionsWhoseTranscriptIsGone(t *testing.T) {
	id := "aaaa1111-2222-3333-4444-555555555555"
	home := syncHome(t, id)
	runSync(t, home)

	transcript := filepath.Join(home, "claude", "projects", "C--dev-waid", id+".jsonl")
	if err := os.Remove(transcript); err != nil {
		t.Fatalf("removing the transcript: %v", err)
	}

	data := runSync(t, home)
	if data.Scanned != 0 || data.Removed != 1 || data.Sessions != 0 {
		t.Errorf("stats = %+v, want the session removed", data)
	}
}

func TestSyncStampsTheCacheWithThePinnedClock(t *testing.T) {
	home := syncHome(t, "aaaa1111-2222-3333-4444-555555555555")
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00.000Z")

	if got := runSync(t, home).SyncedAt; got != "2026-08-17T12:00:00.000Z" {
		t.Errorf("syncedAt = %q, want the pinned instant", got)
	}
}

func TestHumanSyncReportsWhatItMoved(t *testing.T) {
	home := syncHome(t, "aaaa1111-2222-3333-4444-555555555555")

	run := waid(t, home, "sync")
	if run.code != cli.ExitOK {
		t.Fatalf("sync exited %d: %s", run.code, run.err)
	}

	if run.out != "synced 1 session  parsed 1, reused 0, removed 0\n" {
		t.Errorf("out = %q", run.out)
	}
}

func TestHumanSyncPluralizesTheSessionCount(t *testing.T) {
	home := syncHome(t, "aaaa1111-2222-3333-4444-555555555555", "bbbb2222-3333-4444-5555-666666666666")

	run := waid(t, home, "sync")

	if run.out != "synced 2 sessions  parsed 2, reused 0, removed 0\n" {
		t.Errorf("out = %q", run.out)
	}
}
