package events

import (
	"encoding/json"
	"os"
	"strings"
)

// ReadLines reads the log as lines, returning nothing when the log does not exist yet. Both line
// endings are accepted, since the log is edited by hand often enough to have picked up CRLF.
func ReadLines(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Record is one log line parsed as JSON, tagged with the 1-based line it was read from.
type Record struct {
	Line int
	// Value is the untrusted object the line held; callers narrow the fields they need.
	Value map[string]any
}

// ReadRecords reads the log as parsed objects tagged with their 1-based line number. Blank lines,
// lines that do not parse, and lines holding something other than an object are skipped here and
// reported by Fold instead, so line numbers stay comparable between the two.
func ReadRecords(path string) []Record {
	records := []Record{}
	for index, line := range ReadLines(path) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var parsed any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			continue
		}
		if object, ok := parsed.(map[string]any); ok {
			records = append(records, Record{Line: index + 1, Value: object})
		}
	}
	return records
}

// Load folds the whole log at path into the current state.
func Load(path string) State {
	return Fold(ReadLines(path))
}

// Find returns the item carrying id, and whether the log holds one.
func (s State) Find(id string) (Item, bool) {
	for _, item := range s.Items {
		if item.Id == id {
			return item, true
		}
	}
	return Item{}, false
}
