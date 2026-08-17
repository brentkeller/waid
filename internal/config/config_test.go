package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/errs"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(raw)
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// storedConfig reads the seeded config.json back as a loosely typed map, the way the Node tests do.
func storedConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(home, "config.json"))), &parsed); err != nil {
		t.Fatalf("parsing config.json: %v", err)
	}
	return parsed
}

func ensureHome(t *testing.T, home string, detect func() *string) {
	t.Helper()
	if err := EnsureHome(home, detect); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
}

func user(login string) func() *string {
	return func() *string { return &login }
}

func noUser() *string { return nil }

// ignoreLines splits a .gitignore the way git reads it, trimming each entry.
func ignoreLines(contents string) []string {
	lines := strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	return lines
}

func TestResolveHomePrefersTheExplicitOverride(t *testing.T) {
	t.Setenv("WAID_HOME", `C:\from\env`)
	if got := ResolveHome(`C:\from\override`); got != `C:\from\override` {
		t.Fatalf("ResolveHome = %q", got)
	}
}

func TestResolveHomeFallsBackToEnv(t *testing.T) {
	t.Setenv("WAID_HOME", `C:\from\env`)
	if got := ResolveHome(""); got != `C:\from\env` {
		t.Fatalf("ResolveHome(\"\") = %q", got)
	}
	if got := ResolveHome("   "); got != `C:\from\env` {
		t.Fatalf("ResolveHome(blank) = %q", got)
	}
}

func TestResolveHomeFallsBackToTheDefaultDataDirectory(t *testing.T) {
	t.Setenv("WAID_HOME", "")
	os.Unsetenv("WAID_HOME")
	if got := ResolveHome(""); got != DefaultHome {
		t.Fatalf("ResolveHome = %q, want %q", got, DefaultHome)
	}
	if DefaultHome != `C:\data\waid` {
		t.Fatalf("DefaultHome = %q", DefaultHome)
	}
}

func TestEnsureHomeInitialisesAnEmptyHome(t *testing.T) {
	home := t.TempDir()
	ensureHome(t, home, user("octocat"))

	info, err := os.Stat(filepath.Join(home, "cache"))
	if err != nil || !info.IsDir() {
		t.Fatalf("cache/ not a directory: %v", err)
	}
	if events := readFile(t, filepath.Join(home, "events.jsonl")); events != "" {
		t.Fatalf("events.jsonl = %q, want empty", events)
	}

	ignore := readFile(t, filepath.Join(home, ".gitignore"))
	if !slices.Contains(ignoreLines(ignore), "cache/") {
		t.Fatalf("expected cache/ in .gitignore, got %q", ignore)
	}

	defaults := Defaults()
	stored := storedConfig(t, home)
	if stored["ghUser"] != "octocat" {
		t.Fatalf("ghUser = %v", stored["ghUser"])
	}
	if !reflect.DeepEqual(stored["scanRoots"], []any{defaults.ScanRoots[0]}) {
		t.Fatalf("scanRoots = %v", stored["scanRoots"])
	}
	if stored["claudeDir"] != defaults.ClaudeDir {
		t.Fatalf("claudeDir = %v", stored["claudeDir"])
	}
	if stored["activeWindowDays"] != float64(defaults.ActiveWindowDays) {
		t.Fatalf("activeWindowDays = %v", stored["activeWindowDays"])
	}
	if stored["ghCacheTtlMinutes"] != float64(defaults.GhCacheTtlMinutes) {
		t.Fatalf("ghCacheTtlMinutes = %v", stored["ghCacheTtlMinutes"])
	}
	if stored["scanMaxDepth"] != float64(defaults.ScanMaxDepth) {
		t.Fatalf("scanMaxDepth = %v", stored["scanMaxDepth"])
	}
}

// The seeded file has to stay readable by the Node build until cutover: two-space indent, keys in
// the order Node emits them, one trailing newline.
func TestEnsureHomeSeedsConfigInNodesShape(t *testing.T) {
	home := t.TempDir()
	ensureHome(t, home, user("octocat"))

	raw := readFile(t, filepath.Join(home, "config.json"))
	if !strings.HasSuffix(raw, "}\n") {
		t.Fatalf("config.json does not end in a single newline: %q", raw)
	}
	if !strings.Contains(raw, "\n  \"scanRoots\": [") {
		t.Fatalf("config.json is not indented by two spaces: %q", raw)
	}

	order := []string{"scanRoots", "claudeDir", "activeWindowDays", "ghUser", "ghCacheTtlMinutes", "scanMaxDepth"}
	at := -1
	for _, key := range order {
		index := strings.Index(raw, `"`+key+`":`)
		if index <= at {
			t.Fatalf("key %s out of order in %q", key, raw)
		}
		at = index
	}
}

// The keys doctor measures a config against are the keys a seeded config carries, in the same
// order, so a new setting can never be reported to its author as a typo.
func TestKnownKeysAreTheKeysASeededConfigCarries(t *testing.T) {
	home := t.TempDir()
	ensureHome(t, home, user("octocat"))
	raw := readFile(t, filepath.Join(home, "config.json"))

	at := -1
	for _, key := range KnownKeys() {
		index := strings.Index(raw, `"`+key+`":`)
		if index <= at {
			t.Fatalf("key %s is missing or out of order in %q", key, raw)
		}
		at = index
	}
	stored := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("decoding the seeded config: %v", err)
	}
	if len(KnownKeys()) != len(stored) {
		t.Errorf("KnownKeys has %d entries, but the seeded config carries %d", len(KnownKeys()), len(stored))
	}
}

func TestEnsureHomeCreatesTheHomeDirectory(t *testing.T) {
	home := filepath.Join(t.TempDir(), "nested", "waid")
	ensureHome(t, home, noUser)

	info, err := os.Stat(filepath.Join(home, "cache"))
	if err != nil || !info.IsDir() {
		t.Fatalf("cache/ not a directory: %v", err)
	}
}

func TestEnsureHomeLeavesAnExistingConfigUntouched(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	original := `{"ghUser":"mine","scanMaxDepth":9}`
	writeFile(t, configPath, original)

	ensureHome(t, home, user("octocat"))

	if got := readFile(t, configPath); got != original {
		t.Fatalf("config.json = %q, want %q", got, original)
	}
}

func TestEnsureHomeRecordsANullGhUserWhenDetectionFindsNothing(t *testing.T) {
	home := t.TempDir()
	ensureHome(t, home, noUser)

	if stored := storedConfig(t, home); stored["ghUser"] != nil {
		t.Fatalf("ghUser = %v, want null", stored["ghUser"])
	}
}

func TestEnsureHomeSkipsGhDetectionUnderTheSkipEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WAID_SKIP_GH_DETECT", "1")

	ensureHome(t, home, nil)

	if stored := storedConfig(t, home); stored["ghUser"] != nil {
		t.Fatalf("ghUser = %v, want null", stored["ghUser"])
	}
}

func TestEnsureHomeMutatesNothingInAPopulatedHome(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	eventsPath := filepath.Join(home, "events.jsonl")
	ignorePath := filepath.Join(home, ".gitignore")
	gitDir := filepath.Join(home, ".git")

	writeFile(t, configPath, "{\"ghUser\":\"brentkeller\"}\n")
	writeFile(t, eventsPath, "{\"ev\":\"add\",\"id\":\"abcd\",\"title\":\"existing\"}\n")
	writeFile(t, ignorePath, "*.tmp\r\nscratch/\r\n")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatalf("creating .git: %v", err)
	}
	writeFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")

	configBefore := readFile(t, configPath)
	eventsBefore := readFile(t, eventsPath)
	headBefore := readFile(t, filepath.Join(gitDir, "HEAD"))

	ensureHome(t, home, user("octocat"))

	if got := readFile(t, configPath); got != configBefore {
		t.Fatalf("config.json rewritten: %q", got)
	}
	if got := readFile(t, eventsPath); got != eventsBefore {
		t.Fatalf("events.jsonl rewritten: %q", got)
	}
	if got := readFile(t, filepath.Join(gitDir, "HEAD")); got != headBefore {
		t.Fatalf(".git/HEAD rewritten: %q", got)
	}

	ignore := readFile(t, ignorePath)
	lines := ignoreLines(ignore)
	for _, want := range []string{"*.tmp", "scratch/", "cache/"} {
		if !slices.Contains(lines, want) {
			t.Fatalf("expected %q in .gitignore, got %q", want, ignore)
		}
	}
	if !strings.HasSuffix(ignore, "cache/\r\n") {
		t.Fatalf("appended entry did not follow the file's CRLF endings: %q", ignore)
	}

	// A second pass must not append a duplicate entry.
	ensureHome(t, home, user("octocat"))
	if got := readFile(t, ignorePath); got != ignore {
		t.Fatalf(".gitignore changed on the second pass: %q", got)
	}
}

func TestEnsureHomeLeavesAGitignoreThatAlreadyIgnoresCacheByteIdentical(t *testing.T) {
	for _, entry := range []string{"cache/\n", "cache\n", "# notes\ncache/\n"} {
		home := t.TempDir()
		ignorePath := filepath.Join(home, ".gitignore")
		writeFile(t, ignorePath, entry)

		ensureHome(t, home, noUser)

		if got := readFile(t, ignorePath); got != entry {
			t.Fatalf(".gitignore = %q, want %q", got, entry)
		}
	}
}

func TestEnsureHomeAppendsToAGitignoreMissingItsFinalNewline(t *testing.T) {
	home := t.TempDir()
	ignorePath := filepath.Join(home, ".gitignore")
	writeFile(t, ignorePath, "*.tmp")

	ensureHome(t, home, noUser)

	if got := readFile(t, ignorePath); got != "*.tmp\ncache/\n" {
		t.Fatalf(".gitignore = %q", got)
	}
}

func TestLoadFillsMissingKeysFromDefaultsAndDerivesEveryPath(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config.json"), `{"ghUser":"brentkeller"}`)

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	defaults := Defaults()
	if cfg.GhUser == nil || *cfg.GhUser != "brentkeller" {
		t.Fatalf("ghUser = %v", cfg.GhUser)
	}
	if !slices.Equal(cfg.ScanRoots, defaults.ScanRoots) {
		t.Fatalf("scanRoots = %v", cfg.ScanRoots)
	}
	if cfg.ClaudeDir != defaults.ClaudeDir {
		t.Fatalf("claudeDir = %q", cfg.ClaudeDir)
	}
	if cfg.ActiveWindowDays != defaults.ActiveWindowDays {
		t.Fatalf("activeWindowDays = %d", cfg.ActiveWindowDays)
	}
	if cfg.GhCacheTtlMinutes != defaults.GhCacheTtlMinutes {
		t.Fatalf("ghCacheTtlMinutes = %d", cfg.GhCacheTtlMinutes)
	}
	if cfg.ScanMaxDepth != defaults.ScanMaxDepth {
		t.Fatalf("scanMaxDepth = %d", cfg.ScanMaxDepth)
	}

	paths := map[string]struct{ got, want string }{
		"home":              {cfg.Home, home},
		"configPath":        {cfg.ConfigPath, filepath.Join(home, "config.json")},
		"eventsPath":        {cfg.EventsPath, filepath.Join(home, "events.jsonl")},
		"cacheDir":          {cfg.CacheDir, filepath.Join(home, "cache")},
		"sessionsCachePath": {cfg.SessionsCachePath, filepath.Join(home, "cache", "sessions.json")},
		"ghCachePath":       {cfg.GhCachePath, filepath.Join(home, "cache", "gh.json")},
	}
	for name, pair := range paths {
		if pair.got != pair.want {
			t.Fatalf("%s = %q, want %q", name, pair.got, pair.want)
		}
	}
}

func TestLoadUsesDefaultsWhenConfigIsAbsent(t *testing.T) {
	home := t.TempDir()

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.GhUser != nil {
		t.Fatalf("ghUser = %v, want null", *cfg.GhUser)
	}
	if !slices.Equal(cfg.ScanRoots, Defaults().ScanRoots) {
		t.Fatalf("scanRoots = %v", cfg.ScanRoots)
	}
	if cfg.Home != home {
		t.Fatalf("home = %q", cfg.Home)
	}
}

// Overrides must not be shared between calls, so mutating one config cannot reach the next.
func TestLoadDoesNotShareDefaultSlices(t *testing.T) {
	home := t.TempDir()

	first, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	first.ScanRoots[0] = `C:\mutated`

	second, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(second.ScanRoots, Defaults().ScanRoots) {
		t.Fatalf("scanRoots = %v, want the untouched defaults", second.ScanRoots)
	}
}

func TestLoadReportsAUserErrorNamingConfigJson(t *testing.T) {
	for name, contents := range map[string]string{
		"invalid JSON": "{ not json",
		"an array":     "[1, 2]",
		"a scalar":     `"nope"`,
		"null":         "null",
	} {
		home := t.TempDir()
		configPath := filepath.Join(home, "config.json")
		writeFile(t, configPath, contents)

		_, err := Load(home)
		var user *errs.UserError
		if !errors.As(err, &user) {
			t.Fatalf("%s: Load error = %v, want a UserError", name, err)
		}
		if !strings.Contains(user.Message, "config.json") {
			t.Fatalf("%s: message = %q, want it to name config.json", name, user.Message)
		}
	}
}

func TestLoadReportsAUserErrorOnAMistypedValue(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config.json"), `{"scanMaxDepth":"deep"}`)

	_, err := Load(home)
	var user *errs.UserError
	if !errors.As(err, &user) {
		t.Fatalf("Load error = %v, want a UserError", err)
	}
	if !strings.Contains(user.Message, "scanMaxDepth") {
		t.Fatalf("message = %q, want it to name the offending key", user.Message)
	}
}

func TestDetectGhUserIsSkippedByEnv(t *testing.T) {
	t.Setenv("WAID_SKIP_GH_DETECT", "1")
	if got := DetectGhUser(); got != nil {
		t.Fatalf("DetectGhUser = %v, want nil", *got)
	}
}
