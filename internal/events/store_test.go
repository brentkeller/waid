package events

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLog writes a log holding the given raw lines and returns its path.
func writeLog(t *testing.T, lines ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	raw := ""
	for _, line := range lines {
		raw += line + "\n"
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("writing the log: %v", err)
	}
	return path
}

func TestReadRecordsSkipsWhatItCannotUseAndKeepsLineNumbers(t *testing.T) {
	path := writeLog(t,
		`{"ts":"2026-08-14T09:00:00.000Z","ev":"add","id":"a1b2"}`,
		``,
		`{"ts":"2026-08-14T09:01:00.000Z","ev":"note","id":`,
		`["not","an","object"]`,
		`{"ts":"2026-08-14T09:02:00.000Z","ev":"close","id":"a1b2"}`,
	)

	records := ReadRecords(path)

	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].Line != 1 || records[0].Value["ev"] != "add" {
		t.Errorf("first record = %+v", records[0])
	}
	if records[1].Line != 5 || records[1].Value["ev"] != "close" {
		t.Errorf("second record = %+v", records[1])
	}
}

// A home that has never been written to has no log at all, which reads as an empty one rather than
// as a failure — every read command has to work on a fresh install.
func TestAnAbsentLogReadsAsAnEmptyOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nothing.jsonl")

	if lines := ReadLines(path); len(lines) != 0 {
		t.Errorf("lines = %v, want none", lines)
	}

	state := Load(path)
	if len(state.Items) != 0 || len(state.Dismissed) != 0 || len(state.Problems) != 0 {
		t.Errorf("state = %+v, want an empty one", state)
	}
}

func TestReadLinesDropsOnlyTheTrailingNewline(t *testing.T) {
	path := writeLog(t, `{"ev":"add"}`, ``, `{"ev":"close"}`)

	if lines := ReadLines(path); len(lines) != 3 {
		t.Errorf("lines = %q, want the blank one kept and the trailing newline dropped", lines)
	}
}

func TestFindReportsWhetherTheLogHoldsAnItem(t *testing.T) {
	path := writeLog(t, `{"ts":"2026-08-14T09:00:00.000Z","ev":"add","id":"a1b2","title":"One"}`)
	state := Load(path)

	item, found := state.Find("a1b2")
	if !found || item.Title != "One" {
		t.Errorf("Find(a1b2) = %+v, %v", item, found)
	}
	if _, found := state.Find("zzzz"); found {
		t.Errorf("Find(zzzz) found an item the log does not hold")
	}
}

func TestReadRecordsOfAnAbsentLogIsEmpty(t *testing.T) {
	records := ReadRecords(filepath.Join(t.TempDir(), "nothing.jsonl"))

	if len(records) != 0 {
		t.Errorf("records = %v, want none", records)
	}
}
