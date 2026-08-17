// Package render holds the column formatting, truncation and calendar-window helpers every human
// rendering shares. Widths are measured the way Node measures them, in UTF-16 code units, so the
// columns line up identically byte for byte.
package render

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	minute = time.Minute
	hour   = time.Hour
	day    = 24 * hour
	week   = 7 * day
	year   = 52 * week
)

// ymdLayout is the local calendar date shape used for grouping and display.
const ymdLayout = "2006-01-02"

// isoLayouts are the timestamp shapes accepted from the event log, git and gh, in the order they
// are tried. A layout without a zone is read as local time, matching how JavaScript's Date parser
// treats a bare date-time.
var isoLayouts = []struct {
	layout string
	zone   *time.Location
}{
	{time.RFC3339, nil},
	{"2006-01-02T15:04:05.999999999", time.Local},
	{"2006-01-02T15:04", time.Local},
	{ymdLayout, time.UTC},
}

// ParseTime reads one of the timestamp shapes waid stores or reads back from git and gh. The second
// result is false when the value is unparseable; the log is untrusted input, so every caller has to
// have an answer for that rather than failing.
func ParseTime(iso string) (time.Time, bool) {
	for _, candidate := range isoLayouts {
		if candidate.zone == nil {
			if parsed, err := time.Parse(candidate.layout, iso); err == nil {
				return parsed, true
			}
			continue
		}
		if parsed, err := time.ParseInLocation(candidate.layout, iso, candidate.zone); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// RelTime renders a compact age for a column: `just now` under a minute, then `45m` / `3h` / `5d` /
// `2w` / `1y`. Unparseable timestamps render as `?` rather than failing. Future timestamps read as
// `just now`, since clock skew between agents is not worth a `-2m`.
func RelTime(iso string, now time.Time) string {
	then, ok := ParseTime(iso)
	if !ok {
		return "?"
	}

	elapsed := now.Sub(then)
	switch {
	case elapsed < minute:
		return "just now"
	case elapsed < hour:
		return fmt.Sprintf("%dm", elapsed/minute)
	case elapsed < day:
		return fmt.Sprintf("%dh", elapsed/hour)
	case elapsed < week:
		return fmt.Sprintf("%dd", elapsed/day)
	case elapsed < year:
		return fmt.Sprintf("%dw", elapsed/week)
	default:
		return fmt.Sprintf("%dy", elapsed/year)
	}
}

// Pad fits text to exactly width columns, padding with spaces or truncating with an ellipsis. Width
// counts UTF-16 code units, so an astral character such as an emoji costs two. Truncating between
// the halves of a surrogate pair leaves a lone surrogate, which decodes to the replacement
// character — the same byte sequence Node writes to a UTF-8 stream.
func Pad(text string, width int) string {
	if width <= 0 {
		return ""
	}

	units := utf16.Encode([]rune(text))
	if len(units) == width {
		return text
	}
	if len(units) < width {
		return text + strings.Repeat(" ", width-len(units))
	}
	if width == 1 {
		return string(utf16.Decode(units[:1]))
	}
	return string(utf16.Decode(units[:width-1])) + "…"
}

// LocalYmd renders the local calendar date of an instant, as `YYYY-MM-DD`.
func LocalYmd(t time.Time) string {
	return t.Local().Format(ymdLayout)
}

// LocalYmdISO renders the local calendar date of a timestamp string. An unparseable timestamp
// renders as `NaN-NaN-NaN`, which is what Node's date formatting produces for one.
func LocalYmdISO(iso string) string {
	parsed, ok := ParseTime(iso)
	if !ok {
		return "NaN-NaN-NaN"
	}
	return LocalYmd(parsed)
}

// DayBounds returns the half-open local-time window [start, end) covering one calendar day.
func DayBounds(ymd string) (start, end time.Time) {
	parts := strings.Split(ymd, "-")
	year, month, dayOfMonth := field(parts, 0, 0), field(parts, 1, 1), field(parts, 2, 1)

	start = time.Date(year, time.Month(month), dayOfMonth, 0, 0, 0, 0, time.Local)
	end = time.Date(year, time.Month(month), dayOfMonth+1, 0, 0, 0, 0, time.Local)
	return start, end
}

// WeekBounds returns the half-open local-time window [start, end) for the Monday-start week
// containing t, shifted by offsetWeeks (-1 for last week).
func WeekBounds(t time.Time, offsetWeeks int) (start, end time.Time) {
	local := t.Local()
	// Weekday is Sunday-based; Monday-start means Sunday belongs to the week that began six days ago.
	daysSinceMonday := (int(local.Weekday()) + 6) % 7
	firstDay := local.Day() - daysSinceMonday + offsetWeeks*7

	start = time.Date(local.Year(), local.Month(), firstDay, 0, 0, 0, 0, time.Local)
	end = time.Date(local.Year(), local.Month(), firstDay+7, 0, 0, 0, 0, time.Local)
	return start, end
}

// field reads the index'th dash-separated component of a date as a number, falling back to
// fallback when it is absent or not a number.
func field(parts []string, index int, fallback int) int {
	if index >= len(parts) {
		return fallback
	}
	value, err := strconv.Atoi(parts[index])
	if err != nil {
		return fallback
	}
	return value
}
