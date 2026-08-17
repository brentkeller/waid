package command_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/command"
)

// transcriptFixtures holds the transcripts ported from the Node tree, read relative to this package.
var transcriptFixtures = filepath.Join("..", "..", "testdata", "transcripts")

// transcriptHome is a home whose Claude Code directory holds copies of the named fixtures, each
// filed under the session id the command will be asked for.
func transcriptHome(t *testing.T, transcripts map[string]string) string {
	t.Helper()

	home := makeHome(t)
	claudeDir := filepath.Join(home, "claude")
	seedConfig(t, home, `{ "scanRoots": [], "claudeDir": `+quote(t, claudeDir)+` }`)

	dir := filepath.Join(claudeDir, "projects", "C--dev-waid")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	for id, fixture := range transcripts {
		raw, err := os.ReadFile(filepath.Join(transcriptFixtures, fixture))
		if err != nil {
			t.Fatalf("reading fixture %s: %v", fixture, err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), raw, 0o644); err != nil {
			t.Fatalf("writing a transcript: %v", err)
		}
	}
	return home
}

// transcriptJson runs transcript --json and decodes what it printed.
func transcriptJson(t *testing.T, home, id string) (result, command.TranscriptResult) {
	t.Helper()

	run := waid(t, home, "transcript", id, "--json")
	var data command.TranscriptResult
	run.decode(t, &data)
	return run, data
}

func TestTranscriptReturnsTheTurnsOfAKnownSession(t *testing.T) {
	home := transcriptHome(t, map[string]string{"a1b2c3d4": "normal.jsonl"})

	run, data := transcriptJson(t, home, "a1b2c3d4")

	if run.code != cli.ExitOK {
		t.Fatalf("transcript exited %d: %s", run.code, run.err)
	}
	if data.Session.Id != "a1b2c3d4" || data.Session.Title != "Fix budget chart legend overflow" {
		t.Errorf("session = %+v", data.Session)
	}
	if len(data.Turns) != 4 {
		t.Fatalf("got %d turns, want 4", len(data.Turns))
	}
	first := data.Turns[0]
	if first.Role != "user" || first.Text != "Fix the budget chart legend so it stops overflowing" {
		t.Errorf("the first turn is %+v", first)
	}
	if last := data.Turns[3]; last.Role != "assistant" || last.Text != "Test added." {
		t.Errorf("the last turn is %+v", last)
	}
}

func TestTranscriptResolvesAUniqueIdPrefix(t *testing.T) {
	home := transcriptHome(t, map[string]string{"a1b2c3d4": "normal.jsonl", "d4e5f6a7": "truncated.jsonl"})

	run, data := transcriptJson(t, home, "d4e5")

	if run.code != cli.ExitOK {
		t.Fatalf("transcript exited %d: %s", run.code, run.err)
	}
	if data.Session.Id != "d4e5f6a7" {
		t.Errorf("resolved to %s, want d4e5f6a7", data.Session.Id)
	}
}

func TestTranscriptRejectsAnAmbiguousPrefixAndListsTheCandidates(t *testing.T) {
	// empty.jsonl carries no session id of its own, so it takes the one its filename gives it.
	home := transcriptHome(t, map[string]string{"a1b2c3d4": "normal.jsonl", "a1b29999": "empty.jsonl"})

	run := waid(t, home, "transcript", "a1b2")

	if run.code != cli.ExitUser {
		t.Fatalf("transcript exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `ambiguous session id: a1b2`)
	for _, candidate := range []string{"a1b29999", "a1b2c3d4"} {
		if !strings.Contains(run.err, candidate) {
			t.Errorf("stderr is missing the candidate %s:\n%s", candidate, run.err)
		}
	}
}

func TestTranscriptRendersTheSessionHeaderAndItsTurns(t *testing.T) {
	home := transcriptHome(t, map[string]string{"a1b2c3d4": "normal.jsonl"})

	run := waid(t, home, "transcript", "a1b2c3d4")

	if run.code != cli.ExitOK {
		t.Fatalf("transcript exited %d: %s", run.code, run.err)
	}
	for _, want := range []string{
		"a1b2c3d4  Fix budget chart legend overflow",
		`  project  C:\dev\waid`,
		"  branch   fix/legend",
		"  prompts  2",
		"Fix the budget chart legend so it stops overflowing",
		"Test added.",
	} {
		if !strings.Contains(run.out, want) {
			t.Errorf("stdout is missing %q:\n%s", want, run.out)
		}
	}
	if strings.Contains(run.out, "Search the repo for legend layout code.") {
		t.Errorf("a sidechain turn was rendered:\n%s", run.out)
	}
}

func TestTranscriptRendersASessionWithNoTurns(t *testing.T) {
	home := transcriptHome(t, map[string]string{"c3d4e5f6": "sidechain-only.jsonl"})

	run := waid(t, home, "transcript", "c3d4e5f6")

	if run.code != cli.ExitOK {
		t.Fatalf("transcript exited %d: %s", run.code, run.err)
	}
	if !strings.Contains(run.out, "(no turns)") {
		t.Errorf("stdout = %q", run.out)
	}
}

func TestTranscriptOfAnUnknownIdExitsOne(t *testing.T) {
	home := transcriptHome(t, map[string]string{"a1b2c3d4": "normal.jsonl"})

	run := waid(t, home, "transcript", "nope")

	if run.code != cli.ExitUser {
		t.Fatalf("transcript exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `unknown session id: nope`)
}

func TestTranscriptWithNoIdExitsOne(t *testing.T) {
	home := transcriptHome(t, map[string]string{"a1b2c3d4": "normal.jsonl"})

	run := waid(t, home, "transcript")

	if run.code != cli.ExitUser {
		t.Fatalf("transcript exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `transcript requires a session id`)
}

func TestTranscriptWhoseFileHasGoneExitsOne(t *testing.T) {
	home := transcriptHome(t, map[string]string{"a1b2c3d4": "normal.jsonl"})
	if _, data := transcriptJson(t, home, "a1b2c3d4"); len(data.Turns) == 0 {
		t.Fatal("the seeded transcript produced no turns")
	}

	path := filepath.Join(home, "claude", "projects", "C--dev-waid", "a1b2c3d4.jsonl")
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing the transcript: %v", err)
	}

	run := waid(t, home, "transcript", "a1b2c3d4", "--no-sync")
	if run.code != cli.ExitUser {
		t.Fatalf("transcript exited %d, want %d", run.code, cli.ExitUser)
	}
	assertMatches(t, run.err, `cannot read the transcript`)
}
