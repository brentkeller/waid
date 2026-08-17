package harness_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/harness"
)

// TestMain drops what the harness materialized once for the whole suite — the built binary and the
// transcript snapshot — which no single test's cleanup can own.
func TestMain(m *testing.M) {
	code := m.Run()
	harness.Release()
	os.Exit(code)
}

func TestGoldenHomeExercisesEveryEventType(t *testing.T) {
	lines := goldenLines(t)

	for _, ev := range []string{"add", "update", "note", "close", "reopen", "dismiss", "undismiss"} {
		if !containsSubstring(lines, `"ev":"`+ev+`"`) {
			t.Errorf("the fixture log carries no %s event", ev)
		}
	}

	state := events.Fold(lines)
	if len(state.Items) != 4 {
		t.Errorf("items = %d, want 4", len(state.Items))
	}
	if len(state.Dismissed) != 1 || state.Dismissed[0] != "review:DevResults/DevResults#6886" {
		t.Errorf("dismissed = %v, want the single un-restored key", state.Dismissed)
	}
}

func TestGoldenHomeCoversEveryProblemReason(t *testing.T) {
	reasons := []events.ProblemReason{
		events.ReasonUnparseable,
		events.ReasonNotAnObject,
		events.ReasonAddMissingField,
		events.ReasonDuplicateId,
		events.ReasonUnknownId,
		events.ReasonNoteMissingText,
		events.ReasonBadStatus,
		events.ReasonMissingKey,
		events.ReasonUnknownEv,
	}

	state := events.Fold(goldenLines(t))

	counts := map[events.ProblemReason]int{}
	for _, problem := range state.Problems {
		counts[problem.Reason]++
	}
	for _, reason := range reasons {
		if counts[reason] != 1 {
			t.Errorf("%s appears %d times in the fixture log, want exactly 1", reason, counts[reason])
		}
	}
	if len(state.Problems) != len(reasons) {
		t.Errorf("problems = %d, want %d", len(state.Problems), len(reasons))
	}
}

func TestGoldenHomeIsAFreshCopy(t *testing.T) {
	fixture := filepath.Join(harness.RepoRoot(t), "testdata", "golden", "events.jsonl")
	original, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading the fixture log: %v", err)
	}

	home := harness.GoldenHome(t)
	if err := os.WriteFile(filepath.Join(home, "events.jsonl"), []byte("clobbered\n"), 0o644); err != nil {
		t.Fatalf("writing to the copy: %v", err)
	}

	second := harness.GoldenHome(t)
	if second == home {
		t.Fatal("two materialized homes share a directory")
	}
	copied, err := os.ReadFile(filepath.Join(second, "events.jsonl"))
	if err != nil {
		t.Fatalf("reading the second copy: %v", err)
	}
	if string(copied) != string(original) {
		t.Error("the second copy does not match the checked-in fixture")
	}

	after, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("re-reading the fixture log: %v", err)
	}
	if string(after) != string(original) {
		t.Error("the checked-in fixture was modified; the harness must never run in place")
	}
}

func TestGoldenHomeResolvesTheClaudeDirectory(t *testing.T) {
	home := harness.GoldenHome(t)

	raw, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatalf("reading the copied config: %v", err)
	}
	if strings.Contains(string(raw), "{{home}}") {
		t.Fatalf("config.json still carries an unexpanded token:\n%s", raw)
	}
	if _, err := os.Stat(filepath.Join(home, "claude", "projects")); err != nil {
		t.Fatalf("the fixture transcript directory is missing: %v", err)
	}
}

func TestRunReportsStderrAndExitCode(t *testing.T) {
	out := harness.Run(t, harness.GoldenHome(t), harness.Invocation{Args: []string{"nonsense"}})

	if out.Code != 1 {
		t.Errorf("code = %d, want 1", out.Code)
	}
	if out.Stderr != "unknown command: nonsense\n" {
		t.Errorf("stderr = %q", out.Stderr)
	}
	if out.Stdout != "" {
		t.Errorf("stdout = %q, want empty", out.Stdout)
	}
}

// The clock and the id generator reach the spawned process, which is what makes a run repeatable:
// without them, every appended line would carry the wall clock and a random id.
func TestRunPinsTheClockAndTheIdGenerator(t *testing.T) {
	home := harness.GoldenHome(t)

	out := harness.Run(t, home, harness.Invocation{
		Args: []string{"add", "harness pinned"},
		Now:  time.Date(2026, 8, 17, 15, 32, 4, 642_000_000, time.UTC),
		Ids:  []string{"z9z9"},
	})
	if out.Code != 0 {
		t.Fatalf("code = %d, stderr = %q", out.Code, out.Stderr)
	}

	want := `{"ts":"2026-08-17T15:32:04.642Z","ev":"add","id":"z9z9","title":"harness pinned",` +
		`"status":"open","project":null,"session":null,"tags":[],"waitingOn":null}`
	lines := readLines(t, filepath.Join(home, "events.jsonl"))
	if got := lines[len(lines)-1]; got != want {
		t.Errorf("appended line:\n got %s\nwant %s", got, want)
	}
}

func TestRealHomeIsCopiedNeverUsedInPlace(t *testing.T) {
	source := harness.RealHomePath()
	home := harness.RealHome(t)

	original, err := os.ReadFile(filepath.Join(source, "events.jsonl"))
	if err != nil {
		t.Fatalf("reading the real log: %v", err)
	}
	copied, err := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if err != nil {
		t.Fatalf("reading the copied log: %v", err)
	}
	if string(copied) != string(original) {
		t.Error("the copied log does not match the real one")
	}
	if _, err := os.Stat(filepath.Join(home, "cache")); !os.IsNotExist(err) {
		t.Error("the derived cache was copied; it is disposable and must be rebuilt")
	}

	if err := os.WriteFile(filepath.Join(home, "events.jsonl"), []byte("clobbered\n"), 0o644); err != nil {
		t.Fatalf("writing to the copy: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(source, "events.jsonl"))
	if err != nil {
		t.Fatalf("re-reading the real log: %v", err)
	}
	if string(after) != string(original) {
		t.Fatal("the real log was modified; the harness must never run in place")
	}
}

func goldenLines(t *testing.T) []string {
	t.Helper()
	return readLines(t, filepath.Join(harness.RepoRoot(t), "testdata", "golden", "events.jsonl"))
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if strings.Contains(string(raw), "\r\n") {
		t.Fatalf("%s holds CRLF line endings; the fixtures encode exact bytes", path)
	}
	return lines
}

func containsSubstring(lines []string, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}
