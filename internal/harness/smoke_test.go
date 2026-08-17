package harness_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/harness"
)

// smokeTimeout bounds a spawned run, so a command that waits on something it should not fails the
// test instead of hanging the suite.
const smokeTimeout = 30 * time.Second

var idPattern = regexp.MustCompile(`^[0-9a-z]{4}$`)

type smokeResult struct {
	Code   int
	Stdout string
	Stderr string
}

// smokeHome is a temp home with config.json written up front, so the spawned binary never walks the
// real C:\dev or ~/.claude: discovery has no roots and the transcript directory does not exist.
func smokeHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	settings := config.Defaults()
	settings.ScanRoots = []string{}
	settings.ClaudeDir = filepath.Join(home, "no-such-claude-dir")

	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("encoding the smoke config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.json"), append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("writing the smoke config: %v", err)
	}
	return home
}

// waid runs the built binary as a real child process — no in-process shortcuts, so the exit code,
// the streams and the process's own view of its stdio are the ones a shell would see.
func waid(t *testing.T, home string, args ...string) smokeResult {
	t.Helper()

	argv := append([]string{"--waid-home", home}, args...)
	command := exec.Command(harness.GoBinary(t), argv...)
	command.Dir = home
	command.Env = append(os.Environ(), "WAID_HOME=", "WAID_SKIP_GH_DETECT=1")

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		t.Fatalf("starting waid %v: %v", args, err)
	}

	done := make(chan error, 1)
	go func() { done <- command.Wait() }()

	var exit *exec.ExitError
	select {
	case err := <-done:
		if err != nil && !errors.As(err, &exit) {
			t.Fatalf("running waid %v: %v", args, err)
		}
	case <-time.After(smokeTimeout):
		command.Process.Kill()
		<-done
		t.Fatalf("waid %v did not finish within %s", args, smokeTimeout)
	}

	return smokeResult{Code: command.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}
}

func TestSmokeHelpRunsFromTheBuiltBinary(t *testing.T) {
	result := waid(t, smokeHome(t), "--help")

	if result.Code != 0 {
		t.Fatalf("--help exited %d: %s", result.Code, result.Stderr)
	}
	if result.Stderr != "" {
		t.Errorf("--help wrote to stderr: %q", result.Stderr)
	}
	if !bytes.Contains([]byte(result.Stdout), []byte("waid loops")) {
		t.Errorf("usage is missing the command table:\n%s", result.Stdout)
	}
}

func TestSmokeAddedItemComesBackFromLoops(t *testing.T) {
	home := smokeHome(t)

	added := waid(t, home, "add", "ship the README", "-p", home, "--json")
	if added.Code != 0 {
		t.Fatalf("add exited %d: %s", added.Code, added.Stderr)
	}
	var item struct {
		Id    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(added.Stdout), &item); err != nil {
		t.Fatalf("decoding add: %v\n%s", err, added.Stdout)
	}
	if !idPattern.MatchString(item.Id) {
		t.Errorf("id %q is not four base32 characters", item.Id)
	}

	loops := waid(t, home, "loops", "--json")
	if loops.Code != 0 {
		t.Fatalf("loops exited %d: %s", loops.Code, loops.Stderr)
	}
	var report struct {
		Groups []struct {
			Items []struct {
				Title string `json:"title"`
			} `json:"items"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(loops.Stdout), &report); err != nil {
		t.Fatalf("decoding loops: %v\n%s", err, loops.Stdout)
	}

	titles := []string{}
	for _, group := range report.Groups {
		for _, entry := range group.Items {
			titles = append(titles, entry.Title)
		}
	}
	if len(titles) != 1 || titles[0] != "ship the README" {
		t.Errorf("loops reported %v, want [ship the README]", titles)
	}
}

// The app refuses to start without a terminal, and a spawned process is the only place that guard
// can be observed for real: the streams here are pipes, which is exactly what it exists to reject.
func TestSmokeUiRefusesPipedStdio(t *testing.T) {
	result := waid(t, smokeHome(t), "ui")

	if result.Code != 1 {
		t.Fatalf("ui exited %d, want 1: %s", result.Code, result.Stderr)
	}
	if result.Stderr != "waid ui requires an interactive terminal\n" {
		t.Errorf("stderr = %q", result.Stderr)
	}
	if result.Stdout != "" {
		t.Errorf("ui wrote to stdout: %q", result.Stdout)
	}
}

func TestSmokeJsonIsASingleDocumentOnStdout(t *testing.T) {
	home := smokeHome(t)
	waid(t, home, "add", "only item")

	result := waid(t, home, "list", "--json")

	if result.Code != 0 {
		t.Fatalf("list exited %d: %s", result.Code, result.Stderr)
	}
	if result.Stderr != "" {
		t.Errorf("list --json wrote to stderr: %q", result.Stderr)
	}
	// Parsing the whole stream, unmodified, is the assertion: any banner or trailing line breaks it.
	var listed struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &listed); err != nil {
		t.Fatalf("decoding list: %v\n%s", err, result.Stdout)
	}
	if len(listed.Items) != 1 {
		t.Errorf("list reported %d items, want 1", len(listed.Items))
	}
}
