// Package harness runs both waid builds against the same data and diffs what they produce. Read
// commands are compared by their output, write commands by the log they leave behind, so the port
// is verified against the implementation it replaces rather than against a description of it.
//
// Every run is pinned: the clock and the id generator come from the environment seams both builds
// honour, and neither build is ever pointed at a home it could damage — the checked-in fixture and
// the real data directory are copied first.
package harness

import (
	"bytes"
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

// DefaultNow is the instant a run is pinned to when the caller pins none.
var DefaultNow = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

// maxReportedLines caps how much of a differing stream is printed, so one wrong header does not
// bury the failure under the whole document.
const maxReportedLines = 5

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
	// detection is diffed against fixed data rather than the network.
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
// call yields its own copy, so two arms of the same comparison can never see each other's writes.
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

// RunGo runs the Go build against home.
func RunGo(t testing.TB, home string, inv Invocation) Output {
	t.Helper()
	return run(t, goBinary(t), nil, home, inv)
}

// RunNode runs the Node build against home, skipping the test when that build is not there to run —
// an absent Node arm retires the comparison rather than failing it.
func RunNode(t testing.TB, home string, inv Invocation) Output {
	t.Helper()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	entry := filepath.Join(RepoRoot(t), "bin", "waid.ts")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("no Node build at %s", entry)
	}
	return run(t, node, []string{entry}, home, inv)
}

// CompareRead runs inv against both builds, each on its own copy of the home, and reports every
// difference in stdout, stderr and the exit code.
func CompareRead(t testing.TB, source HomeSource, inv Invocation) {
	t.Helper()

	nodeHome := source(t)
	goHome := source(t)

	node := RunNode(t, nodeHome, inv)
	built := RunGo(t, goHome, inv)

	reportDifferences(t, inv, DiffOutputs(node, built))
}

// CompareWrite runs inv against both builds on separate copies of the home and additionally diffs
// the log each one left behind, byte for byte.
func CompareWrite(t testing.TB, source HomeSource, inv Invocation) {
	t.Helper()

	nodeHome := source(t)
	goHome := source(t)

	node := RunNode(t, nodeHome, inv)
	built := RunGo(t, goHome, inv)

	differences := DiffOutputs(node, built)
	differences = append(differences, diffText(
		"events.jsonl",
		readLog(t, nodeHome),
		readLog(t, goHome),
	)...)
	reportDifferences(t, inv, differences)
}

// DiffOutputs describes every way the two runs disagree, in the order the streams are produced.
func DiffOutputs(node, built Output) []string {
	differences := diffText("stdout", node.Stdout, built.Stdout)
	differences = append(differences, diffText("stderr", node.Stderr, built.Stderr)...)
	if node.Code != built.Code {
		differences = append(differences, fmt.Sprintf("exit code: node %d, go %d", node.Code, built.Code))
	}
	return differences
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

// run executes one build and captures both streams and the exit code. A command that fails to
// start is a harness fault, not a difference, so it fails the test outright.
func run(t testing.TB, program string, prefix []string, home string, inv Invocation) Output {
	t.Helper()

	args := append(append([]string{}, prefix...), "--waid-home", home)
	args = append(args, inv.Args...)

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
			path = filepath.Join(dir, "waid"+exeSuffix())
			build := exec.Command("go", "build", "-o", path, "./cmd/waid")
			build.Dir = RepoRoot(t)
			if out, err := build.CombinedOutput(); err != nil {
				buildErr = fmt.Errorf("go build: %v\n%s", err, out)
			}
		})
		if buildErr != nil {
			t.Fatalf("building the Go arm: %v", buildErr)
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

	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
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
	if err != nil {
		t.Fatalf("copying %s: %v", src, err)
	}
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
	expanded := strings.ReplaceAll(string(raw), "{{home}}", escaped)
	if expanded == string(raw) {
		return
	}
	if err := os.WriteFile(path, []byte(expanded), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func readLog(t testing.TB, home string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("reading the log in %s: %v", home, err)
	}
	return string(raw)
}

// diffText compares one stream or file, naming the first differing lines rather than printing both
// documents in full.
func diffText(label, node, built string) []string {
	if node == built {
		return nil
	}

	nodeLines := strings.Split(node, "\n")
	builtLines := strings.Split(built, "\n")
	differences := []string{}

	for index := 0; index < max(len(nodeLines), len(builtLines)); index++ {
		left, right := lineAt(nodeLines, index), lineAt(builtLines, index)
		if left == right {
			continue
		}
		if len(differences) == maxReportedLines {
			differences = append(differences, fmt.Sprintf("%s: further differences suppressed", label))
			break
		}
		differences = append(differences, fmt.Sprintf(
			"%s line %d:\n  node: %q\n  go:   %q", label, index+1, left, right,
		))
	}
	return differences
}

func lineAt(lines []string, index int) string {
	if index >= len(lines) {
		return ""
	}
	return lines[index]
}

func reportDifferences(t testing.TB, inv Invocation, differences []string) {
	t.Helper()

	for _, difference := range differences {
		t.Errorf("waid %s\n%s", strings.Join(inv.Args, " "), difference)
	}
}
