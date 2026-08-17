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

func TestReadRecordsOfAnAbsentLogIsEmpty(t *testing.T) {
	records := ReadRecords(filepath.Join(t.TempDir(), "nothing.jsonl"))

	if len(records) != 0 {
		t.Errorf("records = %v, want none", records)
	}
}
