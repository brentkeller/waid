package command_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/gh"
)

// dismissRecord is the --json payload dismiss prints.
type dismissRecord struct {
	Key       string `json:"key"`
	Dismissed bool   `json:"dismissed"`
	Already   bool   `json:"already"`
}

// promoteRecord is the --json payload promote prints.
type promoteRecord struct {
	Id      string  `json:"id"`
	Key     string  `json:"key"`
	Title   string  `json:"title"`
	Project *string `json:"project"`
}

// logEvents parses every appended line, which is what a write command is ultimately judged on.
func logEvents(t *testing.T, home string) []map[string]any {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading the log: %v", err)
	}
	if len(raw) == 0 {
		return nil
	}

	parsed := []map[string]any{}
	for _, line := range logLines(t, home) {
		event := map[string]any{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decoding %q: %v", line, err)
		}
		parsed = append(parsed, event)
	}
	return parsed
}

// dismissJson runs dismiss --json and decodes what it printed.
func dismissJson(t *testing.T, home string, seams cli.Seams, key string) (result, dismissRecord) {
	t.Helper()

	run := waidWith(t, home, seams, "dismiss", key, "--no-sync", "--json")
	var record dismissRecord
	run.decode(t, &record)
	return run, record
}

// promoteJson runs promote --json and decodes what it printed.
func promoteJson(t *testing.T, home string, seams cli.Seams, key string) (result, promoteRecord) {
	t.Helper()

	run := waidWith(t, home, seams, "promote", key, "--no-sync", "--json")
	var record promoteRecord
	run.decode(t, &record)
	return run, record
}

func TestDismissAppendsOneEventAndTheKeyStopsAppearingInScan(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	home := detectHome(t, root)
	seams := cli.Seams{
		Git: fakeGit{repo: {branch: ptr("main"), dirty: 4}},
		Gh:  fakeGh{review: []gh.Pr{pr("acme/web", 123, nil)}},
	}

	_, before := scanJson(t, home, seams)
	assertKeys(t, signalKeys(before.Signals), []string{"review:acme/web#123", "dirty:" + repo})

	run, record := dismissJson(t, home, seams, "review:acme/web#123")
	if run.code != cli.ExitOK {
		t.Fatalf("dismiss exited %d: %s", run.code, run.err)
	}
	if record.Key != "review:acme/web#123" || !record.Dismissed || record.Already {
		t.Errorf("dismiss reported %+v", record)
	}

	log := logEvents(t, home)
	if len(log) != 1 {
		t.Fatalf("the log holds %d events, want 1", len(log))
	}
	if log[0]["ev"] != "dismiss" || log[0]["key"] != "review:acme/web#123" {
		t.Errorf("appended %v", log[0])
	}

	_, after := scanJson(t, home, seams)
	assertKeys(t, signalKeys(after.Signals), []string{"dirty:" + repo})
	if after.DismissedCount != 1 {
		t.Errorf("dismissed count is %d, want 1", after.DismissedCount)
	}
}

func TestDismissingAnAlreadyDismissedKeyIsANoOpThatExitsZero(t *testing.T) {
	home := detectHome(t, t.TempDir())
	seams := cli.Seams{Git: fakeGit{}, Gh: fakeGh{}}

	dismissJson(t, home, seams, "pr:acme/web#9")
	run, record := dismissJson(t, home, seams, "pr:acme/web#9")

	if run.code != cli.ExitOK {
		t.Fatalf("dismiss exited %d: %s", run.code, run.err)
	}
	if !record.Already {
		t.Error("the second dismissal did not report the key as already hidden")
	}
	if log := logEvents(t, home); len(log) != 1 {
		t.Errorf("the log holds %d events, want 1", len(log))
	}
}

func TestDismissRendersTheKeyItHid(t *testing.T) {
	home := detectHome(t, t.TempDir())
	seams := cli.Seams{Git: fakeGit{}, Gh: fakeGh{}}

	first := waidWith(t, home, seams, "dismiss", "pr:acme/web#9", "--no-sync")
	if first.code != cli.ExitOK {
		t.Fatalf("dismiss exited %d: %s", first.code, first.err)
	}
	if first.out != "dismissed pr:acme/web#9\n" {
		t.Errorf("stdout = %q", first.out)
	}

	second := waidWith(t, home, seams, "dismiss", "pr:acme/web#9", "--no-sync")
	if second.out != "already dismissed pr:acme/web#9\n" {
		t.Errorf("stdout = %q", second.out)
	}
}

func TestDismissWithNoKeyExitsOne(t *testing.T) {
	home := detectHome(t, t.TempDir())

	run := waidWith(t, home, cli.Seams{}, "dismiss", "--no-sync")
	if run.code != cli.ExitUser {
		t.Fatalf("dismiss exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `dismiss requires a signal key`)
}

func TestPromoteWritesAnAddThenADismissAndMovesTheSignalIntoLoops(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	home := detectHome(t, root)
	seams := cli.Seams{
		Git: fakeGit{repo: {branch: ptr("topic"), ahead: ptr(3), dirty: 0}},
		Gh:  fakeGh{},
	}

	key := "ahead:" + repo + ":topic"
	run, record := promoteJson(t, home, seams, key)

	if run.code != cli.ExitOK {
		t.Fatalf("promote exited %d: %s", run.code, run.err)
	}
	if record.Key != key {
		t.Errorf("key = %q, want %q", record.Key, key)
	}
	if record.Project == nil || *record.Project != repo {
		t.Errorf("project = %v, want %q", record.Project, repo)
	}
	if record.Title != "3 unpushed commits on topic in alpha" {
		t.Errorf("title = %q", record.Title)
	}
	assertMatches(t, record.Id, `^[a-z0-9]{4}$`)

	log := logEvents(t, home)
	if len(log) != 2 {
		t.Fatalf("the log holds %d events, want 2", len(log))
	}
	if log[0]["ev"] != "add" || log[0]["id"] != record.Id || log[0]["origin"] != repo {
		t.Errorf("the add event is %v", log[0])
	}
	if tags, ok := log[0]["tags"].([]any); !ok || len(tags) != 1 || tags[0] != "promoted" {
		t.Errorf("tags = %v, want [promoted]", log[0]["tags"])
	}
	if log[1]["ev"] != "dismiss" || log[1]["key"] != key {
		t.Errorf("the dismiss event is %v", log[1])
	}

	_, scanned := scanJson(t, home, seams)
	if len(scanned.Signals) != 0 {
		t.Errorf("signals are %v, want none", signalKeys(scanned.Signals))
	}
	if scanned.DismissedCount != 1 {
		t.Errorf("dismissed count is %d, want 1", scanned.DismissedCount)
	}

	_, loops := loopsJson(t, home, seams)
	assertKeys(t, loops.itemIds(), []string{record.Id})
	origins := []string{}
	for _, entry := range loops.Items {
		if entry.Origin != nil {
			origins = append(origins, *entry.Origin)
		}
	}
	assertKeys(t, origins, []string{repo})
	if got := item(t, home, record.Id).Tags; len(got) != 1 || got[0] != "promoted" {
		t.Errorf("the promoted item carries tags %v, want [promoted]", got)
	}
}

func TestPromoteSeedsAPrSignalFromItsTitle(t *testing.T) {
	home := detectHome(t, t.TempDir())
	seams := cli.Seams{
		Git: fakeGit{},
		Gh: fakeGh{review: []gh.Pr{pr("acme/web", 123, func(item *gh.Pr) {
			item.Title = "Fix the chart legend"
		})}},
	}

	run, record := promoteJson(t, home, seams, "review:acme/web#123")
	if run.code != cli.ExitOK {
		t.Fatalf("promote exited %d: %s", run.code, run.err)
	}
	if record.Title != "Fix the chart legend" {
		t.Errorf("title = %q", record.Title)
	}
	// No local clone was discovered, so the item carries no project.
	if record.Project != nil {
		t.Errorf("project = %q, want none", *record.Project)
	}
}

func TestPromoteRendersOneLineNamingTheNewId(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	home := detectHome(t, root)

	run := waidWith(t, home, cli.Seams{
		Git: fakeGit{repo: {branch: ptr("main"), dirty: 14}},
		Gh:  fakeGh{},
	}, "promote", "dirty:"+repo, "--no-sync")

	if run.code != cli.ExitOK {
		t.Fatalf("promote exited %d: %s", run.code, run.err)
	}
	assertMatches(t, run.out, `^promoted [a-z0-9]{4}  14 uncommitted files in alpha\n$`)
}

func TestPromotingAnUnknownKeyExitsOne(t *testing.T) {
	home := detectHome(t, t.TempDir())

	run := waidWith(t, home, cli.Seams{Git: fakeGit{}, Gh: fakeGh{}},
		"promote", "pr:acme/web#404", "--no-sync")

	if run.code != cli.ExitUser {
		t.Fatalf("promote exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `unknown signal key: pr:acme/web#404`)
	if log := logEvents(t, home); len(log) != 0 {
		t.Errorf("the log holds %d events, want none", len(log))
	}
}

func TestPromotingAnAlreadyDismissedKeyExitsOneAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	repo := makeRepo(t, root, "alpha")
	home := detectHome(t, root)
	seams := cli.Seams{Git: fakeGit{repo: {branch: ptr("main"), dirty: 2}}, Gh: fakeGh{}}

	dismissJson(t, home, seams, "dirty:"+repo)
	run := waidWith(t, home, seams, "promote", "dirty:"+repo, "--no-sync")

	if run.code != cli.ExitUser {
		t.Fatalf("promote exited %d, want %d", run.code, cli.ExitUser)
	}
	if log := logEvents(t, home); len(log) != 1 {
		t.Errorf("the log holds %d events, want 1", len(log))
	}
}

func TestPromoteWithNoKeyExitsOne(t *testing.T) {
	home := detectHome(t, t.TempDir())

	run := waidWith(t, home, cli.Seams{Git: fakeGit{}, Gh: fakeGh{}}, "promote", "--no-sync")
	if run.code != cli.ExitUser {
		t.Fatalf("promote exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `promote requires a signal key`)
}
