package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unicode/utf8"
)

// The two separators encoding/json escapes whether or not HTML escaping is on, and JSON.stringify
// leaves raw.
const (
	lineSeparator      = rune(0x2028)
	paragraphSeparator = rune(0x2029)
)

// TsLayout is the timestamp shape Node's `Date#toISOString` produces: UTC, always three fractional
// digits. Go's RFC3339Nano trims trailing zeros, so it cannot be used here.
const TsLayout = "2006-01-02T15:04:05.000Z"

// FormatTs renders t as the log stores timestamps.
func FormatTs(t time.Time) string {
	return t.UTC().Format(TsLayout)
}

// Encode renders one event as the exact line the log holds, without its trailing newline. Two
// details of Go's default JSON output would break compatibility with the lines Node has already
// written, so both are undone here: `<`, `>` and `&` stay raw, and so do U+2028 and U+2029.
func Encode(event WaidEvent) ([]byte, error) {
	// Node emits an empty array rather than null when no tags were given, and a TagsEvent says "no
	// tags" the same way — it exists to carry the set whatever the set is.
	switch typed := event.(type) {
	case AddEvent:
		if typed.Tags == nil {
			typed.Tags = []string{}
			event = typed
		}
	case TagsEvent:
		if typed.Tags == nil {
			typed.Tags = []string{}
			event = typed
		}
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(event); err != nil {
		return nil, fmt.Errorf("encoding %T: %w", event, err)
	}

	return rawLineSeparators(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))), nil
}

// Append stamps event with now unless it already carries a timestamp, then appends it to path as a
// single line, returning the timestamp as written. The whole line goes out in one write, which is
// atomic enough at this size for the parallel agents that share the log.
func Append(path string, event WaidEvent, now time.Time) (string, error) {
	stamped := event.stamp(FormatTs(now))

	line, err := Encode(stamped)
	if err != nil {
		return "", err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer file.Close()

	if _, err := file.Write(append(line, '\n')); err != nil {
		return "", fmt.Errorf("appending to %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("closing %s: %w", path, err)
	}

	return stamped.timestamp(), nil
}

// rawLineSeparators rewrites the U+2028 and U+2029 escapes encoding/json emits unconditionally back
// into their raw UTF-8 bytes, which is what JSON.stringify writes. Escapes are consumed whole, so a
// backslash that is itself part of the payload never combines with the text after it.
func rawLineSeparators(line []byte) []byte {
	if !bytes.Contains(line, []byte(`\u202`)) {
		return line
	}

	out := make([]byte, 0, len(line))
	for index := 0; index < len(line); {
		if line[index] != '\\' {
			out = append(out, line[index])
			index++
			continue
		}

		if index+6 <= len(line) && line[index+1] == 'u' {
			switch string(line[index+2 : index+6]) {
			case "2028":
				out = utf8.AppendRune(out, lineSeparator)
				index += 6
				continue
			case "2029":
				out = utf8.AppendRune(out, paragraphSeparator)
				index += 6
				continue
			}
		}

		// Any other escape is copied as a pair so its second byte is never re-read as an opener.
		out = append(out, line[index:min(index+2, len(line))]...)
		index += 2
	}

	return out
}
