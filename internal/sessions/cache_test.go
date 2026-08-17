package sessions

import (
	"encoding/json"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/config"
)

// layoutEntry is one transcript to plant: which project directory holds it, what it is called, and
// which fixture supplies its content.
type layoutEntry struct {
	dir     string
	id      string
	fixture string
}

// layout is the transcript tree every test starts from: two project directories, four sessions.
var layout = []layoutEntry{
	{dir: "C--dev-waid", id: "a1b2c3d4", fixture: "normal.jsonl"},
	{dir: "C--dev-waid", id: "b2c3d4e5", fixture: "no-ai-title.jsonl"},
	{dir: "C--dev-decoded", id: "c3d4e5f6", fixture: "sidechain-only.jsonl"},
	{dir: "C--dev-decoded", id: "empty-session", fixture: "empty.jsonl"},
}

// setup returns a config against a fresh home whose claudeDir points at a fresh transcript tree.
func setup(t *testing.T) config.Config {
	t.Helper()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
		t.Fatalf("creating the cache directory: %v", err)
	}

	cfg, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	cfg.ClaudeDir = makeClaudeDir(t)
	return cfg
}

func makeClaudeDir(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, entry := range layout {
		dir := filepath.Join(root, "projects", entry.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
		raw, err := os.ReadFile(filepath.Join(fixtures, entry.fixture))
		if err != nil {
			t.Fatalf("reading fixture %s: %v", entry.fixture, err)
		}
		if err := os.WriteFile(filepath.Join(dir, entry.id+".jsonl"), raw, 0o644); err != nil {
			t.Fatalf("planting %s: %v", entry.id, err)
		}
	}
	return root
}

func transcriptPath(cfg config.Config, dir, id string) string {
	return filepath.Join(cfg.ClaudeDir, "projects", dir, id+".jsonl")
}

// countingReader wraps the real line reader so a test can assert which transcripts were opened.
func countingReader(opened *[]string) LineReader {
	return func(filePath string) iter.Seq2[string, error] {
		*opened = append(*opened, filePath)
		return ReadLines(filePath)
	}
}

func sync(t *testing.T, cfg config.Config, opts SyncOptions) SyncResult {
	t.Helper()

	result, err := Sync(cfg, opts)
	if err != nil {
		t.Fatalf("syncing: %v", err)
	}
	return result
}

func byId(sessions []CachedSession) map[string]CachedSession {
	index := make(map[string]CachedSession, len(sessions))
	for _, session := range sessions {
		index[session.Id] = session
	}
	return index
}

func assertStats(t *testing.T, got SyncStats, want SyncStats) {
	t.Helper()

	if got != want {
		t.Errorf("stats mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func writeCacheFile(t *testing.T, cfg config.Config, contents string) {
	t.Helper()

	if err := os.WriteFile(cfg.SessionsCachePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing the cache: %v", err)
	}
}

func TestDiscoverTranscriptsFindsEveryTranscriptWithItsStatsAndDirectorySlug(t *testing.T) {
	cfg := setup(t)

	found := DiscoverTranscripts(cfg)

	if len(found) != 4 {
		t.Fatalf("found %d transcripts, want 4", len(found))
	}
	index := slices.IndexFunc(found, func(file TranscriptFile) bool { return file.Id == "a1b2c3d4" })
	if index < 0 {
		t.Fatalf("no transcript a1b2c3d4 in %+v", found)
	}

	normal := found[index]
	if want := transcriptPath(cfg, "C--dev-waid", "a1b2c3d4"); normal.Path != want {
		t.Errorf("path = %q, want %q", normal.Path, want)
	}
	if normal.Dir != "C--dev-waid" {
		t.Errorf("dir = %q, want C--dev-waid", normal.Dir)
	}
	if normal.Size <= 0 {
		t.Errorf("size = %d, want a positive size", normal.Size)
	}
	if normal.MtimeNs <= 0 {
		t.Errorf("mtimeNs = %d, want a positive time", normal.MtimeNs)
	}
}

func TestFirstSyncParsesEveryTranscriptAndWritesTheCache(t *testing.T) {
	cfg := setup(t)
	var opened []string

	result := sync(t, cfg, SyncOptions{ReadLines: countingReader(&opened)})

	assertStats(t, result.Stats, SyncStats{Scanned: 4, Parsed: 4, Reused: 0, Removed: 0})
	if len(opened) != 4 {
		t.Errorf("opened %d transcripts, want 4", len(opened))
	}
	if result.Version != CacheVersion {
		t.Errorf("version = %d, want %d", result.Version, CacheVersion)
	}

	sessions := byId(result.Sessions)
	normal, found := sessions["a1b2c3d4"]
	if !found {
		t.Fatalf("no session a1b2c3d4 in %v", sessions)
	}
	if normal.Title != "Fix budget chart legend overflow" {
		t.Errorf("title = %q", normal.Title)
	}
	if derefOr(normal.Project, "") != `C:\dev\waid` {
		t.Errorf("project = %v", normal.Project)
	}
	if derefOr(normal.Branch, "") != "fix/legend" {
		t.Errorf("branch = %v", normal.Branch)
	}
	if normal.Prompts != 2 {
		t.Errorf("prompts = %d, want 2", normal.Prompts)
	}
	if want := transcriptPath(cfg, "C--dev-waid", "a1b2c3d4"); normal.File.Path != want {
		t.Errorf("_file.path = %q, want %q", normal.File.Path, want)
	}

	// No cwd anywhere in a sidechain-only transcript, so the directory slug supplies the project.
	if got := derefOr(sessions["c3d4e5f6"].Project, ""); got != `C:\dev\decoded` {
		t.Errorf("sidechain-only project = %q, want C:\\dev\\decoded", got)
	}

	// An empty transcript carries no sessionId, so the filename is the only id available.
	empty, found := sessions["empty-session"]
	if !found {
		t.Fatalf("no session empty-session in %v", sessions)
	}
	if empty.Title != "(untitled)" {
		t.Errorf("empty title = %q, want (untitled)", empty.Title)
	}
	if empty.Prompts != 0 {
		t.Errorf("empty prompts = %d, want 0", empty.Prompts)
	}

	if _, err := os.Stat(cfg.SessionsCachePath); err != nil {
		t.Fatalf("the cache was not written: %v", err)
	}
	if got := len(Load(cfg).Sessions); got != 4 {
		t.Errorf("reloaded %d sessions, want 4", got)
	}
}

func TestSecondSyncWithUntouchedFilesReusesEveryRecordAndReReadsNothing(t *testing.T) {
	cfg := setup(t)
	sync(t, cfg, SyncOptions{})

	var opened []string
	result := sync(t, cfg, SyncOptions{ReadLines: countingReader(&opened)})

	assertStats(t, result.Stats, SyncStats{Scanned: 4, Parsed: 0, Reused: 4, Removed: 0})
	if len(opened) != 0 {
		t.Errorf("re-read %v, want nothing", opened)
	}
	if len(result.Sessions) != 4 {
		t.Errorf("returned %d sessions, want 4", len(result.Sessions))
	}
}

func TestTouchingATranscriptForcesItAndOnlyItToBeReParsed(t *testing.T) {
	cfg := setup(t)
	sync(t, cfg, SyncOptions{})

	touched := transcriptPath(cfg, "C--dev-waid", "a1b2c3d4")
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(touched, future, future); err != nil {
		t.Fatalf("touching %s: %v", touched, err)
	}

	var opened []string
	result := sync(t, cfg, SyncOptions{ReadLines: countingReader(&opened)})

	assertStats(t, result.Stats, SyncStats{Scanned: 4, Parsed: 1, Reused: 3, Removed: 0})
	if !reflect.DeepEqual(opened, []string{touched}) {
		t.Errorf("opened %v, want just %s", opened, touched)
	}
}

func TestFullReParsesEveryTranscriptEvenWhenNothingChanged(t *testing.T) {
	cfg := setup(t)
	sync(t, cfg, SyncOptions{})

	var opened []string
	result := sync(t, cfg, SyncOptions{Full: true, ReadLines: countingReader(&opened)})

	assertStats(t, result.Stats, SyncStats{Scanned: 4, Parsed: 4, Reused: 0, Removed: 0})
	if len(opened) != 4 {
		t.Errorf("opened %d transcripts, want 4", len(opened))
	}
}

func TestADeletedTranscriptDropsOutOfTheCacheOnTheNextSync(t *testing.T) {
	cfg := setup(t)
	sync(t, cfg, SyncOptions{})

	if err := os.Remove(transcriptPath(cfg, "C--dev-decoded", "c3d4e5f6")); err != nil {
		t.Fatalf("removing a transcript: %v", err)
	}
	result := sync(t, cfg, SyncOptions{})

	assertStats(t, result.Stats, SyncStats{Scanned: 3, Parsed: 0, Reused: 3, Removed: 1})
	if _, found := byId(result.Sessions)["c3d4e5f6"]; found {
		t.Error("the deleted transcript's session survived the sync")
	}
	if got := len(Load(cfg).Sessions); got != 3 {
		t.Errorf("reloaded %d sessions, want 3", got)
	}
}

func TestDeletingTheCacheDirectoryIsSafeAndTheNextSyncRebuildsIt(t *testing.T) {
	cfg := setup(t)
	sync(t, cfg, SyncOptions{})

	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatalf("removing the cache directory: %v", err)
	}
	assertEmptyCache(t, Load(cfg))

	result := sync(t, cfg, SyncOptions{})

	assertStats(t, result.Stats, SyncStats{Scanned: 4, Parsed: 4, Reused: 0, Removed: 0})
	if got := len(Load(cfg).Sessions); got != 4 {
		t.Errorf("reloaded %d sessions, want 4", got)
	}
}

func TestACorruptCacheIsAMissNotAnError(t *testing.T) {
	cfg := setup(t)
	writeCacheFile(t, cfg, "{not json")

	assertEmptyCache(t, Load(cfg))
	if age := CacheAgeMinutes(cfg, time.Now()); age != nil {
		t.Errorf("age = %d, want none", *age)
	}
	assertStats(t, sync(t, cfg, SyncOptions{}).Stats, SyncStats{Scanned: 4, Parsed: 4, Reused: 0, Removed: 0})
}

func TestACacheWrittenByAFutureVersionIsIgnoredRatherThanTrusted(t *testing.T) {
	cfg := setup(t)
	writeCacheFile(t, cfg, `{"version":99,"syncedAt":"2026-08-14T00:00:00.000Z","sessions":[]}`)

	assertEmptyCache(t, Load(cfg))
}

func TestAStaleNodeWrittenCacheIsDiscardedAndRebuilt(t *testing.T) {
	cfg := setup(t)
	// Version 1 is the shape the Node build wrote, down to its millisecond mtimes.
	writeCacheFile(t, cfg, `{
  "version": 1,
  "syncedAt": "2026-08-14T00:00:00.000Z",
  "sessions": [
    {
      "id": "a1b2c3d4",
      "title": "Fix budget chart legend overflow",
      "project": "C:\\dev\\waid",
      "branch": "fix/legend",
      "started": "2026-08-14T09:00:00.000Z",
      "ended": "2026-08-14T09:06:30.000Z",
      "prompts": 2,
      "_file": { "path": "`+jsonPath(transcriptPath(cfg, "C--dev-waid", "a1b2c3d4"))+`", "mtimeMs": 1, "size": 1 }
    }
  ]
}`)

	assertEmptyCache(t, Load(cfg))

	result := sync(t, cfg, SyncOptions{})

	assertStats(t, result.Stats, SyncStats{Scanned: 4, Parsed: 4, Reused: 0, Removed: 0})
	if got := Load(cfg).Sessions; len(got) != 4 {
		t.Errorf("reloaded %d sessions, want 4", len(got))
	}
}

func TestAMissingClaudeDirYieldsZeroSessionsAndNoError(t *testing.T) {
	cfg := setup(t)
	cfg.ClaudeDir = filepath.Join(cfg.ClaudeDir, "does-not-exist")

	if found := DiscoverTranscripts(cfg); len(found) != 0 {
		t.Errorf("discovered %v, want nothing", found)
	}

	result := sync(t, cfg, SyncOptions{})

	assertStats(t, result.Stats, SyncStats{})
	if len(result.Sessions) != 0 {
		t.Errorf("returned %d sessions, want none", len(result.Sessions))
	}
}

func TestCacheAgeMinutesMeasuresTheRecordedSyncTime(t *testing.T) {
	cfg := setup(t)
	syncedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	sync(t, cfg, SyncOptions{Now: syncedAt})

	if age := CacheAgeMinutes(cfg, syncedAt); age == nil || *age != 0 {
		t.Errorf("age at the sync time = %v, want 0", age)
	}
	later := syncedAt.Add(30 * time.Minute)
	if age := CacheAgeMinutes(cfg, later); age == nil || *age != 30 {
		t.Errorf("age 30 minutes on = %v, want 30", age)
	}
}

func TestAHandEditedSessionRecordIsSkippedRatherThanCrashingARead(t *testing.T) {
	cfg := setup(t)
	sync(t, cfg, SyncOptions{})

	raw, err := os.ReadFile(cfg.SessionsCachePath)
	if err != nil {
		t.Fatalf("reading the cache: %v", err)
	}
	var cache map[string]any
	if err := json.Unmarshal(raw, &cache); err != nil {
		t.Fatalf("decoding the cache: %v", err)
	}
	sessions, _ := cache["sessions"].([]any)
	if len(sessions) != 4 {
		t.Fatalf("cache holds %d sessions, want 4", len(sessions))
	}
	// A record whose id has been replaced by a number carries no usable identity.
	sessions[0].(map[string]any)["id"] = 7
	edited, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("re-encoding the cache: %v", err)
	}
	writeCacheFile(t, cfg, string(edited))

	if got := len(Load(cfg).Sessions); got != 3 {
		t.Errorf("loaded %d sessions, want the 3 that survived narrowing", got)
	}
}

func TestReadLinesSplitsATranscriptOnNewlinesWithoutTheSeparators(t *testing.T) {
	cfg := setup(t)

	var lines []string
	for line, err := range ReadLines(transcriptPath(cfg, "C--dev-waid", "a1b2c3d4")) {
		if err != nil {
			t.Fatalf("reading lines: %v", err)
		}
		lines = append(lines, line)
	}

	if len(lines) <= 1 {
		t.Fatalf("read %d lines, want several", len(lines))
	}
	for _, line := range lines {
		if strings.Contains(line, "\n") {
			t.Errorf("line %q still carries a newline", line)
		}
	}
}

func TestReadLinesReportsAMissingFileRatherThanYieldingNothing(t *testing.T) {
	cfg := setup(t)

	var failed bool
	for _, err := range ReadLines(filepath.Join(cfg.ClaudeDir, "nope.jsonl")) {
		if err != nil {
			failed = true
		}
	}

	if !failed {
		t.Error("a missing transcript yielded no error")
	}
}

func assertEmptyCache(t *testing.T, loaded LoadedCache) {
	t.Helper()

	if loaded.SyncedAt != "" || len(loaded.Sessions) != 0 {
		t.Errorf("cache = %+v, want an empty one", loaded)
	}
}

// jsonPath escapes a Windows path for embedding in a JSON string literal.
func jsonPath(path string) string {
	encoded, err := json.Marshal(path)
	if err != nil {
		return path
	}
	return string(encoded[1 : len(encoded)-1])
}
