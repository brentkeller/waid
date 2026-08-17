package events

import (
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
