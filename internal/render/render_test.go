package render

import (
	"testing"
	"time"
)

var now = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

const second = time.Second

// ago is an ISO timestamp d before now.
func ago(d time.Duration) string {
	return now.Add(-d).UTC().Format("2006-01-02T15:04:05.000Z")
}

func TestRelTimeBucketsAtEveryBoundary(t *testing.T) {
	cases := []struct {
		elapsed time.Duration
		want    string
	}{
		{0, "just now"},
		{59 * second, "just now"},
		{minute, "1m"},
		{59 * minute, "59m"},
		{hour, "1h"},
		{23 * hour, "23h"},
		{day, "1d"},
		{6 * day, "6d"},
		{week, "1w"},
		{51 * week, "51w"},
		{52 * week, "1y"},
		{120 * week, "2y"},
	}

	for _, c := range cases {
		if got := RelTime(ago(c.elapsed), now); got != c.want {
			t.Errorf("RelTime(%v ago) = %q, want %q", c.elapsed, got, c.want)
		}
	}
}

func TestRelTimeTreatsFutureTimestampsAsNow(t *testing.T) {
	if got := RelTime(ago(-day), now); got != "just now" {
		t.Fatalf("RelTime(a day ahead) = %q, want %q", got, "just now")
	}
}

func TestRelTimeReturnsAPlaceholderForAnUnparseableTimestamp(t *testing.T) {
	for _, input := range []string{"", "not a date"} {
		if got := RelTime(input, now); got != "?" {
			t.Errorf("RelTime(%q) = %q, want %q", input, got, "?")
		}
	}
}

func TestRelTimeAcceptsAnOffsetTimestamp(t *testing.T) {
	// The same instant as two hours before now, written with a zone offset rather than as UTC.
	if got := RelTime("2026-08-14T12:00:00+02:00", now); got != "2h" {
		t.Fatalf("RelTime(offset timestamp) = %q, want %q", got, "2h")
	}
}

func TestPadRightPadsToTheRequestedWidth(t *testing.T) {
	cases := []struct {
		text  string
		width int
		want  string
	}{
		{"ab", 5, "ab   "},
		{"", 3, "   "},
		{"abcde", 5, "abcde"},
	}

	for _, c := range cases {
		if got := Pad(c.text, c.width); got != c.want {
			t.Errorf("Pad(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

func TestPadTruncatesWithAnEllipsisWhenTheTextIsTooLong(t *testing.T) {
	cases := []struct {
		text  string
		width int
		want  string
	}{
		{"abcdef", 5, "abcd…"},
		{"abc", 1, "a"},
		{"abc", 0, ""},
		{"abc", -1, ""},
	}

	for _, c := range cases {
		if got := Pad(c.text, c.width); got != c.want {
			t.Errorf("Pad(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

func TestPadCountsUtf16CodeUnitsAsNodeDoes(t *testing.T) {
	// `é` is one code unit, so five of them already fill a width of five.
	if got := Pad("ééééé", 5); got != "ééééé" {
		t.Errorf("Pad(five accents, 5) = %q, want the text unchanged", got)
	}
	if got := Pad("ééé", 5); got != "ééé  " {
		t.Errorf("Pad(three accents, 5) = %q, want two trailing spaces", got)
	}
	// An emoji outside the BMP is a surrogate pair: two code units, not one.
	if got := Pad("🙂", 3); got != "🙂 " {
		t.Errorf("Pad(emoji, 3) = %q, want one trailing space", got)
	}
	// Truncating between the halves of a surrogate pair leaves a lone surrogate, which Node writes
	// to a UTF-8 stream as the replacement character.
	if got := Pad("a🙂b", 3); got != "a�…" {
		t.Errorf("Pad(text split mid-pair, 3) = %q, want the half pair replaced", got)
	}
}

func TestTruncateFitsTextToAWidthWithoutPaddingIt(t *testing.T) {
	cases := []struct {
		text  string
		width int
		want  string
	}{
		{"abcdef", 5, "abcd…"},
		{"abc", 5, "abc"},
		{"abcde", 5, "abcde"},
		{"abc", 1, "a"},
		{"abc", 0, ""},
		{"abc", -1, ""},
		// An emoji outside the BMP costs two code units, so three of them overflow a width of five.
		{"🙂🙂🙂", 5, "🙂🙂…"},
	}

	for _, c := range cases {
		if got := Truncate(c.text, c.width); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

func TestLocalYmdFormatsATimeInLocalTime(t *testing.T) {
	cases := []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 8, 14, 23, 30, 0, 0, time.Local), "2026-08-14"},
		{time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local), "2026-01-02"},
	}

	for _, c := range cases {
		if got := LocalYmd(c.at); got != c.want {
			t.Errorf("LocalYmd(%v) = %q, want %q", c.at, got, c.want)
		}
	}
}

func TestLocalYmdConvertsAwayFromTheHeldZone(t *testing.T) {
	local := time.Date(2026, 8, 14, 9, 15, 0, 0, time.Local)
	if got := LocalYmd(local.UTC()); got != "2026-08-14" {
		t.Fatalf("LocalYmd(the same instant as UTC) = %q, want %q", got, "2026-08-14")
	}
}

func TestLocalYmdISOAcceptsAnISOString(t *testing.T) {
	local := time.Date(2026, 8, 14, 9, 15, 0, 0, time.Local)
	iso := local.UTC().Format("2006-01-02T15:04:05.000Z")

	if got := LocalYmdISO(iso); got != "2026-08-14" {
		t.Fatalf("LocalYmdISO(%q) = %q, want %q", iso, got, "2026-08-14")
	}
}

func TestLocalYmdISOMirrorsNodeOnAnUnparseableTimestamp(t *testing.T) {
	if got := LocalYmdISO("not a date"); got != "NaN-NaN-NaN" {
		t.Fatalf("LocalYmdISO(garbage) = %q, want %q", got, "NaN-NaN-NaN")
	}
}

func TestDayBoundsSpansExactlyOneLocalDay(t *testing.T) {
	start, end := DayBounds("2026-08-14")

	if got := LocalYmd(start); got != "2026-08-14" {
		t.Errorf("start = %q, want %q", got, "2026-08-14")
	}
	if h, m, s := start.Clock(); h != 0 || m != 0 || s != 0 || start.Nanosecond() != 0 {
		t.Errorf("start clock = %02d:%02d:%02d.%09d, want midnight", h, m, s, start.Nanosecond())
	}
	if got := LocalYmd(end); got != "2026-08-15" {
		t.Errorf("end = %q, want %q", got, "2026-08-15")
	}
	if end.Hour() != 0 {
		t.Errorf("end hour = %d, want 0", end.Hour())
	}
}

func TestWeekBoundsStartsOnTheMondayOfAMidWeekDate(t *testing.T) {
	// 2026-08-12 is a Wednesday.
	start, end := WeekBounds(time.Date(2026, 8, 12, 16, 45, 0, 0, time.Local), 0)

	if got := LocalYmd(start); got != "2026-08-10" {
		t.Errorf("start = %q, want %q", got, "2026-08-10")
	}
	if start.Weekday() != time.Monday {
		t.Errorf("start weekday = %v, want Monday", start.Weekday())
	}
	if start.Hour() != 0 {
		t.Errorf("start hour = %d, want 0", start.Hour())
	}
	if got := LocalYmd(end); got != "2026-08-17" {
		t.Errorf("end = %q, want %q", got, "2026-08-17")
	}
}

func TestWeekBoundsPutsASundayInTheWeekThatBeganTheMondayBefore(t *testing.T) {
	// 2026-08-16 is a Sunday.
	start, end := WeekBounds(time.Date(2026, 8, 16, 23, 0, 0, 0, time.Local), 0)

	if got := LocalYmd(start); got != "2026-08-10" {
		t.Errorf("start = %q, want %q", got, "2026-08-10")
	}
	if got := LocalYmd(end); got != "2026-08-17" {
		t.Errorf("end = %q, want %q", got, "2026-08-17")
	}
}

func TestWeekBoundsShiftsBackAWholeWeekForOffsetWeeksMinusOne(t *testing.T) {
	start, end := WeekBounds(time.Date(2026, 8, 12, 16, 45, 0, 0, time.Local), -1)

	if got := LocalYmd(start); got != "2026-08-03" {
		t.Errorf("start = %q, want %q", got, "2026-08-03")
	}
	if got := LocalYmd(end); got != "2026-08-10" {
		t.Errorf("end = %q, want %q", got, "2026-08-10")
	}
}

func TestWeekBoundsConvertsAwayFromTheHeldZone(t *testing.T) {
	local := time.Date(2026, 8, 12, 16, 45, 0, 0, time.Local)
	start, _ := WeekBounds(local.UTC(), 0)

	if got := LocalYmd(start); got != "2026-08-10" {
		t.Fatalf("start = %q, want %q", got, "2026-08-10")
	}
}
