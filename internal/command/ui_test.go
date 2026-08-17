package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
)

// uiRun is one `waid ui` invocation with both terminal seams replaced, so the guard is exercised
// without a terminal and the app is never actually started.
type uiRun struct {
	code    int
	out     string
	err     string
	started bool
}

func runUiCommand(t *testing.T, interactive bool) uiRun {
	t.Helper()

	t.Setenv("WAID_HOME", "")
	t.Setenv(cli.EnvNow, "")
	t.Setenv(cli.EnvIds, "")

	started := false
	swap(t, &stdoutIsTerminal, func() bool { return interactive })
	swap(t, &startApp, func(*cli.Ctx) error {
		started = true
		return nil
	})

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"--waid-home", t.TempDir(), "ui"}, cli.Io{Out: &out, Err: &errOut}, Registry)
	return uiRun{code: code, out: out.String(), err: errOut.String(), started: started}
}

// swap replaces a seam for the duration of the test.
func swap[T any](t *testing.T, seam *T, replacement T) {
	t.Helper()

	original := *seam
	t.Cleanup(func() { *seam = original })
	*seam = replacement
}

func TestUiResolvesFromTheRegistry(t *testing.T) {
	if _, known := Registry["ui"]; !known {
		t.Fatal("ui is not in the dispatch table")
	}
}

func TestUiOnANonInteractiveStdoutExitsOneWithoutStartingTheApp(t *testing.T) {
	run := runUiCommand(t, false)

	if run.code != cli.ExitUser {
		t.Fatalf("ui exited %d, want %d", run.code, cli.ExitUser)
	}
	if run.err != "waid ui requires an interactive terminal\n" {
		t.Errorf("stderr = %q", run.err)
	}
	if run.out != "" {
		t.Errorf("ui wrote to stdout: %q", run.out)
	}
	// The alt screen is only ever entered by the program, so never starting it is the assertion.
	if run.started {
		t.Error("ui started the app despite stdout not being a terminal")
	}
}

func TestUiStartsTheAppOnAnInteractiveStdout(t *testing.T) {
	run := runUiCommand(t, true)

	if run.code != cli.ExitOK {
		t.Fatalf("ui exited %d: %s", run.code, run.err)
	}
	if !run.started {
		t.Error("ui did not start the app")
	}
	if run.out != "" {
		t.Errorf("ui wrote to stdout: %q", run.out)
	}
}

func TestTheTerminalGuardRejectsAPipeAndAFile(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("opening a pipe: %v", err)
	}
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})
	if isTerminal(writer) {
		t.Error("a pipe is reported as a terminal")
	}

	file, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatalf("creating a file: %v", err)
	}
	t.Cleanup(func() { file.Close() })
	if isTerminal(file) {
		t.Error("a regular file is reported as a terminal")
	}
}
