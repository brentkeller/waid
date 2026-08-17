// Package config resolves the data directory, seeds it on first run, and loads config.json.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/errs"
)

// DefaultHome is the data directory used when neither an override nor WAID_HOME is given.
const DefaultHome = `C:\data\waid`

const gitignoreEntry = "cache/"

// ghDetectTimeout bounds the login lookup so a hung gh cannot stall first run.
const ghDetectTimeout = 5 * time.Second

// Settings are the keys persisted in config.json. Fields are declared in the order Node writes
// them, which is the order the seeded file holds.
type Settings struct {
	// ScanRoots are the directories walked when looking for git repos.
	ScanRoots []string `json:"scanRoots"`
	// ClaudeDir is the root of the Claude Code data directory holding the transcript files.
	ClaudeDir string `json:"claudeDir"`
	// ActiveWindowDays is the recency gate, in days, for repo activity and detection.
	ActiveWindowDays int `json:"activeWindowDays"`
	// GhUser is the GitHub login used for the pull request signals; nil disables them.
	GhUser *string `json:"ghUser"`
	// GhCacheTtlMinutes is the lifetime of cache/gh.json before gh is queried again.
	GhCacheTtlMinutes int `json:"ghCacheTtlMinutes"`
	// ScanMaxDepth is the repo discovery depth under each scan root.
	ScanMaxDepth int `json:"scanMaxDepth"`
}

// Config is the stored settings plus the paths derived from the home directory.
type Config struct {
	Settings

	Home              string `json:"home"`
	ConfigPath        string `json:"configPath"`
	EventsPath        string `json:"eventsPath"`
	CacheDir          string `json:"cacheDir"`
	SessionsCachePath string `json:"sessionsCachePath"`
	GhCachePath       string `json:"ghCachePath"`
}

// Defaults returns the settings used for keys config.json does not carry. Each call returns its own
// slice, so a caller mutating one config cannot reach the next.
func Defaults() Settings {
	return Settings{
		ScanRoots:         []string{`C:\dev`},
		ClaudeDir:         filepath.Join(userHome(), ".claude"),
		ActiveWindowDays:  30,
		GhUser:            nil,
		GhCacheTtlMinutes: 15,
		ScanMaxDepth:      4,
	}
}

// KnownKeys are the keys config.json is allowed to carry, in the order Settings declares them.
// Read from the struct tags rather than restated, so a new setting cannot be reported as a typo.
func KnownKeys() []string {
	fields := reflect.VisibleFields(reflect.TypeOf(Settings{}))
	keys := make([]string, 0, len(fields))
	for _, field := range fields {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// ResolveHome resolves the data directory: explicit override, then WAID_HOME, then DefaultHome.
func ResolveHome(override string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(os.Getenv("WAID_HOME")); trimmed != "" {
		return trimmed
	}
	return DefaultHome
}

// DetectGhUser reads the GitHub login from gh, returning nil when gh is missing, unauthenticated or
// slow. Skipped entirely when WAID_SKIP_GH_DETECT is set, so tests never shell out.
func DetectGhUser() *string {
	if os.Getenv("WAID_SKIP_GH_DETECT") != "" {
		return nil
	}

	command := exec.Command("gh", "api", "user", "-q", ".login")
	command.WaitDelay = ghDetectTimeout
	var out bytes.Buffer
	command.Stdout = &out
	if err := runWithTimeout(command, ghDetectTimeout); err != nil {
		return nil
	}

	login := strings.TrimSpace(out.String())
	if login == "" {
		return nil
	}
	return &login
}

// runWithTimeout runs command, killing it once timeout has passed.
func runWithTimeout(command *exec.Cmd, timeout time.Duration) error {
	if err := command.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() { done <- command.Wait() }()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		if command.Process != nil {
			command.Process.Kill()
		}
		<-done
		return fmt.Errorf("gh timed out after %s", timeout)
	}
}

// EnsureHome performs first-run init, idempotent against an already-populated home: it creates only
// what is missing and never rewrites config.json, events.jsonl or existing .gitignore entries. A nil
// detectUser falls back to DetectGhUser.
func EnsureHome(home string, detectUser func() *string) error {
	if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", home, err)
	}

	eventsPath := filepath.Join(home, "events.jsonl")
	if !exists(eventsPath) {
		if err := os.WriteFile(eventsPath, nil, 0o644); err != nil {
			return fmt.Errorf("creating %s: %w", eventsPath, err)
		}
	}

	if err := ensureGitignoreEntry(filepath.Join(home, ".gitignore")); err != nil {
		return err
	}

	configPath := filepath.Join(home, "config.json")
	if exists(configPath) {
		return nil
	}

	if detectUser == nil {
		detectUser = DetectGhUser
	}
	seeded := Defaults()
	seeded.GhUser = detectUser()

	encoded, err := encodeSettings(seeded)
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, encoded, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", configPath, err)
	}
	return nil
}

// encodeSettings renders the seeded config the way Node writes it: two-space indent, `<`, `>` and
// `&` left raw, one trailing newline.
func encodeSettings(settings Settings) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(settings); err != nil {
		return nil, fmt.Errorf("encoding config.json: %w", err)
	}
	return buf.Bytes(), nil
}

// ensureGitignoreEntry adds cache/ to the home's .gitignore, leaving a file that already ignores the
// cache byte-identical and matching the line endings already in use.
func ensureGitignoreEntry(gitignorePath string) error {
	raw, err := os.ReadFile(gitignorePath)
	if err != nil {
		if err := os.WriteFile(gitignorePath, []byte(gitignoreEntry+"\n"), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", gitignorePath, err)
		}
		return nil
	}

	current := string(raw)
	lines := strings.Split(strings.ReplaceAll(current, "\r\n", "\n"), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	if slices.Contains(lines, gitignoreEntry) || slices.Contains(lines, "cache") {
		return nil
	}

	eol := "\n"
	if strings.Contains(current, "\r\n") {
		eol = "\r\n"
	}
	separator := ""
	if current != "" && !strings.HasSuffix(current, "\n") {
		separator = eol
	}

	file, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", gitignorePath, err)
	}
	defer file.Close()

	if _, err := file.WriteString(separator + gitignoreEntry + eol); err != nil {
		return fmt.Errorf("appending to %s: %w", gitignorePath, err)
	}
	return file.Close()
}

// Load reads config.json, filling missing keys from Defaults and deriving every path. A config file
// that cannot be read at all reads as an empty one; a malformed one is a user error.
func Load(home string) (Config, error) {
	configPath := filepath.Join(home, "config.json")
	settings, err := readSettings(configPath)
	if err != nil {
		return Config{}, err
	}

	cacheDir := filepath.Join(home, "cache")
	return Config{
		Settings:          settings,
		Home:              home,
		ConfigPath:        configPath,
		EventsPath:        filepath.Join(home, "events.jsonl"),
		CacheDir:          cacheDir,
		SessionsCachePath: filepath.Join(cacheDir, "sessions.json"),
		GhCachePath:       filepath.Join(cacheDir, "gh.json"),
	}, nil
}

func readSettings(configPath string) (Settings, error) {
	settings := Defaults()

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return settings, nil
	}

	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return Settings{}, errs.Userf("invalid JSON in config.json (%s): %s", configPath, err)
	}
	if _, isObject := probe.(map[string]any); !isObject {
		return Settings{}, errs.Userf("config.json must contain a JSON object (%s)", configPath)
	}

	// Keys the file omits keep the defaults already in settings.
	if err := json.Unmarshal(raw, &settings); err != nil {
		return Settings{}, errs.Userf("invalid value in config.json (%s): %s", configPath, describeValueError(err))
	}
	return settings, nil
}

// describeValueError names the offending key when the decoder knows it.
func describeValueError(err error) string {
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) && typeError.Field != "" {
		return fmt.Sprintf("%s must be %s, got %s", typeError.Field, typeError.Type, typeError.Value)
	}
	return err.Error()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func userHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
