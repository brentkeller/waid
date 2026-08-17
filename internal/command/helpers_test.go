package command_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/command"
	"github.com/brentkeller/waid/internal/events"
)

// result is one CLI run, judged the way the process would be judged.
type result struct {
	code int
	out  string
	err  string
}

// decode parses whichever stream carried output — stdout for results, stderr for --json errors.
func (r result) decode(t *testing.T, target any) {
	t.Helper()

	payload := strings.TrimSpace(r.out)
	if payload == "" {
		payload = strings.TrimSpace(r.err)
	}
	if err := json.Unmarshal([]byte(payload), target); err != nil {
		t.Fatalf("decoding %q: %v", payload, err)
	}
}

// makeHome returns a fresh data directory, with every environment seam a developer's own shell
// might carry cleared so the run never reaches for gh or an outside home.
func makeHome(t *testing.T) string {
	t.Helper()

	t.Setenv("WAID_SKIP_GH_DETECT", "1")
	t.Setenv("WAID_HOME", "")
	t.Setenv(cli.EnvNow, "")
	t.Setenv(cli.EnvIds, "")
	return t.TempDir()
}

// waid runs the CLI in-process against home, capturing both streams.
func waid(t *testing.T, home string, args ...string) result {
	t.Helper()

	return waidWith(t, home, cli.Seams{}, args...)
}

// waidWith is waid with the detection seams supplied, which is how a test drives detection without
// touching real repos or the network.
func waidWith(t *testing.T, home string, seams cli.Seams, args ...string) result {
	t.Helper()

	var out, errOut bytes.Buffer
	argv := append([]string{"--waid-home", home}, args...)
	code := cli.RunWith(argv, cli.Io{Out: &out, Err: &errOut}, command.Registry, seams)
	return result{code: code, out: out.String(), err: errOut.String()}
}

// logLines reads the appended log, which is what a write command is ultimately judged on.
func logLines(t *testing.T, home string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// item folds the log and returns the item, which is the observable surface until list and show land.
func item(t *testing.T, home, id string) events.Item {
	t.Helper()

	state := events.Load(filepath.Join(home, "events.jsonl"))
	for _, candidate := range state.Items {
		if candidate.Id == id {
			return candidate
		}
	}
	t.Fatalf("no item %s", id)
	return events.Item{}
}

// addItem adds an item and returns its id.
func addItem(t *testing.T, home string, args ...string) string {
	t.Helper()

	run := waid(t, home, append(append([]string{"add"}, args...), "--json")...)
	if run.code != cli.ExitOK {
		t.Fatalf("add exited %d: %s", run.code, run.err)
	}
	var record struct {
		Id string `json:"id"`
	}
	run.decode(t, &record)
	return record.Id
}
