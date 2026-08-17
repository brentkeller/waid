// Package harness runs the built waid binary as a real process against a data directory it
// materializes, so every command is exercised end to end over data no unit test constructs: the
// checked-in fixture home, which carries one of every event type and one of every problem, and a
// copy of the real data directory on this machine.
//
// Every run is pinned: the clock, the id generator and the GitHub response come from the
// environment seams the binary honours, and the binary is never pointed at a home it could damage —
// both sources are copied first, and the originals are only ever read.
//
// Two inputs live outside the copied home and move on their own: the transcript directory, which an
// agent session appends to while the suite runs, and the scan roots, whose working trees follow
// whatever is being edited. RealHomePinned replaces the first with a snapshot and empties the
// second; RealHomeLiveRepos restores the roots for a run that wants them.
//
// The package began as a differential harness that ran this binary and the Node build it replaced
// side by side and diffed both. That comparison was retired with the Node tree once the port was
// verified; what remains is the half that outlives it.
package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
)

// EnvRealHome overrides the data directory RealHome copies from.
const EnvRealHome = "WAID_REAL_HOME"

// EnvLiveRepos opts a run into the detection pass that probes the real working trees under the
// configured scan roots. See RealHomeLiveRepos for why it is not on by default.
const EnvLiveRepos = "WAID_HARNESS_LIVE_REPOS"

// DefaultNow is the instant a run is pinned to when the caller pins none.
var DefaultNow = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

// homeToken stands in for the materialized data directory, and is the placeholder the fixture
// config.json carries.
const homeToken = "{{home}}"

// Invocation is one command line, with everything non-deterministic about it pinned.
type Invocation struct {
	// Args are the command and its flags; --waid-home is supplied by the harness.
	Args []string
	// Now pins the clock. The zero value means DefaultNow.
	Now time.Time
	// Ids pins the ids new items draw, consumed in order.
	Ids []string
	// Cwd is the working directory the command runs in, defaulting to the repo root.
	Cwd string
	// GhFixture is the path to the recorded GitHub response served instead of querying gh, so
	// detection runs against fixed data rather than the network.
	GhFixture string
}

// Output is everything a run is judged on.
type Output struct {
	Stdout string
	Stderr string
	Code   int
}

// HomeSource materializes a data directory to run against. GoldenHome and RealHome are the two.
type HomeSource func(t testing.TB) string

// GoldenHome copies the checked-in fixture home into a fresh temp directory and returns it. Each
// call yields its own copy, so a run that writes cannot reach the fixture or another run's home.
func GoldenHome(t testing.TB) string {
	t.Helper()

	home := t.TempDir()
	copyTree(t, filepath.Join(RepoRoot(t), "testdata", "golden"), home, nil)
	expandHomeToken(t, filepath.Join(home, "config.json"), home)
	return home
}

// RealHomePath is the data directory RealHome copies from.
func RealHomePath() string {
	if override := strings.TrimSpace(os.Getenv(EnvRealHome)); override != "" {
		return override
	}
	return config.DefaultHome
}

// RealHome copies the real data directory into a fresh temp directory and returns it, because
// synthetic events do not reproduce a note carrying an ampersand or a project path with a space in
// it. The original is never handed out and never written to. The derived cache is left behind:
// it is disposable, and a copy of it would only mask a rebuild the port has to get right anyway.
func RealHome(t testing.TB) string {
	t.Helper()

	source := RealHomePath()
	if _, err := os.Stat(filepath.Join(source, "events.jsonl")); err != nil {
		t.Skipf("no real data directory at %s", source)
	}

	home := t.TempDir()
	copyTree(t, source, home, func(rel string) bool {
		root, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		return root == "cache" || root == ".git"
	})
	return home
}

// RealHomePinned is RealHome with the two inputs that live outside the copied directory pinned: the
// transcript directory is replaced by a snapshot taken once per test binary, and the scan roots are
// emptied. Both move on their own — an agent session appends to its transcript while the suite runs,
// and a working tree's dirty count follows whatever is being edited — so a run that read them would
// be reporting the filesystem rather than the binary. RealHomeLiveRepos restores the roots for a run
// that wants them.
func RealHomePinned(t testing.TB) string {
	t.Helper()
	return pinnedRealHome(t, false)
}

// RealHomeLiveRepos is RealHomePinned with the configured scan roots left in place, so detection
// discovers and probes the real working trees. It runs only when EnvLiveRepos is set: the run costs
// a git subprocess per active repo, and what it reads is whatever the machine's working trees happen
// to hold at the time.
func RealHomeLiveRepos(t testing.TB) string {
	t.Helper()

	if strings.TrimSpace(os.Getenv(EnvLiveRepos)) == "" {
		t.Skipf("set %s to run against the real working trees", EnvLiveRepos)
	}
	return pinnedRealHome(t, true)
}

func pinnedRealHome(t testing.TB, keepScanRoots bool) string {
	t.Helper()

	home := RealHome(t)
	cfg, err := config.Load(home)
	if err != nil {
		t.Fatalf("reading the copied config: %v", err)
	}

	PatchConfig(t, home, func(settings map[string]any) {
		settings["claudeDir"] = claudeSnapshot(t, cfg.ClaudeDir)
		if !keepScanRoots {
			settings["scanRoots"] = []string{}
		}
	})
	return home
}

// PatchConfig rewrites config.json in a materialized home. The keys are re-encoded rather than
// edited in place, so a caller that depends on their stored order patches the text itself.
func PatchConfig(t testing.TB, home string, mutate func(settings map[string]any)) {
	t.Helper()

	path := filepath.Join(home, "config.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	settings := map[string]any{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	mutate(settings)

	patched, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("encoding %s: %v", path, err)
	}
	if err := os.WriteFile(path, patched, 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// claudeSnapshot copies the transcripts under source once per test binary and returns a directory
// that can stand in for the Claude data directory. Only projects/ is copied, since that is the only
// subtree the harvester reads, and the copy is shared by every home the suite materializes: it is
// large enough that copying it per home would dominate the run.
var claudeSnapshot = func() func(t testing.TB, source string) string {
	var once sync.Once
	var root string
	var snapshotErr error

	return func(t testing.TB, source string) string {
		t.Helper()
		once.Do(func() {
			root, snapshotErr = os.MkdirTemp("", "waid-harness-claude-")
			if snapshotErr != nil {
				return
			}
			releasable(root)
			snapshotErr = copyDir(
				filepath.Join(source, "projects"),
				filepath.Join(root, "projects"),
				nil,
			)
		})
		if snapshotErr != nil {
			t.Fatalf("snapshotting %s: %v", source, snapshotErr)
		}
		return root
	}
}()

// Release removes what the harness materializes once per test binary — the built binary and the
// transcript snapshot. TestMain calls it once the suite is done; the directories are outside any
// t.TempDir precisely so they outlive the test that created them.
func Release() {
	releaseMu.Lock()
	defer releaseMu.Unlock()

	for _, path := range releasePaths {
		os.RemoveAll(path)
	}
	releasePaths = nil
}

var (
	releaseMu    sync.Mutex
	releasePaths []string
)

func releasable(path string) {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	releasePaths = append(releasePaths, path)
}

// GoBinary builds cmd/waid once per test binary and returns the path to it, so a test that spawns
// the process itself uses the same build every other run in this package does.
func GoBinary(t testing.TB) string {
	t.Helper()
	return goBinary(t)
}

// Run runs the built binary against home.
func Run(t testing.TB, home string, inv Invocation) Output {
	t.Helper()
	return run(t, goBinary(t), home, inv)
}

// RepoRoot is the directory holding go.mod, located from this file rather than the working
// directory, which a test may have moved.
func RepoRoot(t testing.TB) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the harness source file")
	}

	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", filepath.Dir(file))
		}
		dir = parent
	}
}

// run executes the binary and captures both streams and the exit code. A command that fails to
// start is a harness fault rather than a result, so it fails the test outright.
func run(t testing.TB, program string, home string, inv Invocation) Output {
	t.Helper()

	args := append([]string{"--waid-home", home}, inv.Args...)

	command := exec.Command(program, args...)
	command.Dir = inv.Cwd
	if command.Dir == "" {
		command.Dir = RepoRoot(t)
	}
	command.Env = environment(inv)

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	var exit *exec.ExitError
	if err := command.Run(); err != nil && !errors.As(err, &exit) {
		t.Fatalf("running %s: %v", program, err)
	}
	return Output{Stdout: stdout.String(), Stderr: stderr.String(), Code: command.ProcessState.ExitCode()}
}

// environment builds the child's environment: the seams pinned, and every waid variable the parent
// happens to carry cleared, so a developer's own WAID_HOME cannot reach the run.
func environment(inv Invocation) []string {
	pinned := map[string]string{
		"WAID_HOME":           "",
		"WAID_SKIP_GH_DETECT": "1",
		cli.EnvNow:            events.FormatTs(nowOr(inv.Now)),
		cli.EnvIds:            strings.Join(inv.Ids, ","),
		cli.EnvGhFixture:      inv.GhFixture,
	}

	environment := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, overridden := pinned[strings.ToUpper(name)]; !overridden {
			environment = append(environment, entry)
		}
	}
	for name, value := range pinned {
		if value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func nowOr(pinned time.Time) time.Time {
	if pinned.IsZero() {
		return DefaultNow
	}
	return pinned
}

// goBinary builds cmd/waid once per test binary and returns the path to it.
var goBinary = func() func(t testing.TB) string {
	var once sync.Once
	var path string
	var buildErr error

	return func(t testing.TB) string {
		t.Helper()
		once.Do(func() {
			dir, err := os.MkdirTemp("", "waid-harness-")
			if err != nil {
				buildErr = err
				return
			}
			releasable(dir)
			path = filepath.Join(dir, "waid"+exeSuffix())
			build := exec.Command("go", "build", "-o", path, "./cmd/waid")
			build.Dir = RepoRoot(t)
			if out, err := build.CombinedOutput(); err != nil {
				buildErr = fmt.Errorf("go build: %v\n%s", err, out)
			}
		})
		if buildErr != nil {
			t.Fatalf("building cmd/waid: %v", buildErr)
		}
		return path
	}
}()

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// copyTree copies src into dst, skipping the paths skip reports by their slash-separated path
// relative to src.
func copyTree(t testing.TB, src, dst string, skip func(rel string) bool) {
	t.Helper()

	if err := copyDir(src, dst, skip); err != nil {
		t.Fatalf("copying %s: %v", src, err)
	}
}

// copyDir is copyTree without a test to fail, for the callers that carry their own error.
func copyDir(src, dst string, skip func(rel string) bool) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if skip != nil && skip(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
}

// expandHomeToken rewrites the {{home}} placeholder the fixture config carries, which is how a
// checked-in config names paths inside the directory it is copied to.
func expandHomeToken(t testing.TB, path, home string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("reading %s: %v", path, err)
	}

	escaped := strings.ReplaceAll(home, `\`, `\\`)
	expanded := strings.ReplaceAll(string(raw), homeToken, escaped)
	if expanded == string(raw) {
		return
	}
	if err := os.WriteFile(path, []byte(expanded), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
