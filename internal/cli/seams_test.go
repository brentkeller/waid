package cli_test

import (
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
