package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/cli"
)

func TestNowReadsThePinnedInstant(t *testing.T) {
	t.Setenv(cli.EnvNow, "2026-08-17T15:32:04.642Z")

	now := cli.Now()

	if got := now.UTC().Format("2006-01-02T15:04:05.000Z"); got != "2026-08-17T15:32:04.642Z" {
		t.Fatalf("Now() = %s, want the pinned instant", got)
	}
}

func TestNowFallsBackToTheSystemClock(t *testing.T) {
	t.Setenv(cli.EnvNow, "")

	before := time.Now()
	now := cli.Now()

	if now.Before(before.Add(-time.Minute)) || now.After(time.Now().Add(time.Minute)) {
		t.Fatalf("Now() = %s, want a value near the system clock", now)
	}
}

func TestNowIgnoresAnUnparseablePin(t *testing.T) {
	t.Setenv(cli.EnvNow, "not a timestamp")

	before := time.Now()
	now := cli.Now()

	if now.Before(before.Add(-time.Minute)) {
		t.Fatalf("Now() = %s, want a value near the system clock", now)
	}
}

func TestIdGeneratorYieldsThePinnedSequence(t *testing.T) {
	t.Setenv(cli.EnvIds, "a1b2,c3d4")

	generate := cli.IdGenerator()

	first, err := generate(nil)
	if err != nil {
		t.Fatalf("first id: %v", err)
	}
	second, err := generate(nil)
	if err != nil {
		t.Fatalf("second id: %v", err)
	}
	if first != "a1b2" || second != "c3d4" {
		t.Fatalf("ids = %q, %q, want a1b2, c3d4", first, second)
	}
	if _, err := generate(nil); err == nil {
		t.Fatal("want an error once the pinned sequence is exhausted")
	}
}

func TestIdGeneratorSkipsTakenPinnedIds(t *testing.T) {
	t.Setenv(cli.EnvIds, "a1b2,c3d4")

	generate := cli.IdGenerator()

	id, err := generate(func(candidate string) bool { return candidate == "a1b2" })
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	if id != "c3d4" {
		t.Fatalf("id = %q, want c3d4", id)
	}
}

func TestIdGeneratorDefaultsToRandomIds(t *testing.T) {
	t.Setenv(cli.EnvIds, "")

	generate := cli.IdGenerator()

	id, err := generate(nil)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	if len(id) != 4 {
		t.Fatalf("id = %q, want 4 characters", id)
	}
}

func TestGhClientServesThePinnedFixture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh.json")
	recorded := `{"reviewRequested": {"data": {"search": {"nodes": [
	  {"number": 3, "title": "one", "repository": {"nameWithOwner": "octo/widgets"}}
	]}}}}`
	if err := os.WriteFile(path, []byte(recorded), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	t.Setenv(cli.EnvGhFixture, path)

	client := cli.GhClient()
	if client == nil {
		t.Fatal("GhClient() = nil, want the fixture client")
	}

	prs, err := client.ReviewRequested()
	if err != nil {
		t.Fatalf("ReviewRequested: %v", err)
	}
	if len(prs) != 1 || prs[0].Number != 3 {
		t.Fatalf("reviewRequested = %+v, want the one recorded pull request", prs)
	}
}

func TestGhClientIsAbsentWithoutAFixture(t *testing.T) {
	t.Setenv(cli.EnvGhFixture, "")

	if client := cli.GhClient(); client != nil {
		t.Fatalf("GhClient() = %v, want nil so detection reaches for the real gh", client)
	}
}
