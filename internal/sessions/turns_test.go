package sessions

import (
	"path/filepath"
	"reflect"
	"testing"
)

// readFixture reads a ported transcript through the real line reader, which is what the command does.
func readFixture(t *testing.T, name string) []Turn {
	t.Helper()

	turns, err := ReadTurns(filepath.Join(fixtures, name), ReadLines)
	if err != nil {
		t.Fatalf("reading turns from %s: %v", name, err)
	}
	return turns
}

func assertTurns(t *testing.T, got []Turn, want []Turn) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("turns mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestTurnsKeepFileOrderAndCarryRoleTimestampAndText(t *testing.T) {
	assertTurns(t, readFixture(t, "normal.jsonl"), []Turn{
		{Role: "user", Ts: ptr("2026-08-14T09:00:05.000Z"), Text: "Fix the budget chart legend so it stops overflowing"},
		{Role: "assistant", Ts: ptr("2026-08-14T09:00:09.000Z"), Text: "Looking at the legend now."},
		{Role: "user", Ts: ptr("2026-08-14T09:05:00.000Z"), Text: "Now add a regression test."},
		{Role: "assistant", Ts: ptr("2026-08-14T09:06:30.000Z"), Text: "Test added."},
	})
}

func TestTurnsDropSidechainAndMetaRecords(t *testing.T) {
	for _, turn := range readFixture(t, "normal.jsonl") {
		if turn.Text == "Search the repo for legend layout code." {
			t.Error("a sidechain record was reported as a turn")
		}
		if turn.Text == "<command-name>/clear</command-name>" {
			t.Error("a meta record was reported as a turn")
		}
	}

	assertTurns(t, readFixture(t, "sidechain-only.jsonl"), []Turn{})
}

func TestTurnsSurviveCrlfAndATruncatedFinalLine(t *testing.T) {
	assertTurns(t, readFixture(t, "crlf.jsonl"), readFixture(t, "normal.jsonl"))

	assertTurns(t, readFixture(t, "truncated.jsonl"), []Turn{
		{Role: "user", Ts: ptr("2026-08-14T12:00:00.000Z"), Text: "Start the migration."},
		{Role: "user", Ts: ptr("2026-08-14T12:01:00.000Z"), Text: "Keep going."},
	})
}

func TestAnEmptyTranscriptHasNoTurns(t *testing.T) {
	assertTurns(t, readFixture(t, "empty.jsonl"), []Turn{})
}

func TestAMissingTranscriptIsAnError(t *testing.T) {
	if _, err := ReadTurns(filepath.Join(t.TempDir(), "absent.jsonl"), ReadLines); err == nil {
		t.Error("reading an absent transcript succeeded")
	}
}
