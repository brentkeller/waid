package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// realLogPath is the live log the port has to stay byte-compatible with. Overridable so the check
// can be pointed at a copy.
func realLogPath() string {
	if override := os.Getenv("WAID_REAL_EVENTS"); override != "" {
		return override
	}
	return `C:\data\waid\events.jsonl`
}

func encode(t *testing.T, event WaidEvent) string {
	t.Helper()
	line, err := Encode(event)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return string(line)
}

func TestEncodeKeyOrderMatchesNodeEmissionOrder(t *testing.T) {
	project := `C:\dev\waid`
	tests := []struct {
		name  string
		event WaidEvent
		want  string
	}{
		{
			name: "add",
			event: AddEvent{
				Ts:     "2026-08-15T00:31:16.973Z",
				Ev:     "add",
				Id:     "sga9",
				Title:  "Design template",
				Status: StatusOpen,
				Origin: &project,
				Tags:   []string{"bug", "promoted"},
			},
			want: `{"ts":"2026-08-15T00:31:16.973Z","ev":"add","id":"sga9","title":"Design template",` +
				`"status":"open","origin":"C:\\dev\\waid","session":null,"tags":["bug","promoted"],"waitingOn":null}`,
		},
		{
			name:  "note",
			event: NoteEvent{Ts: "2026-08-17T15:00:42.428Z", Ev: "note", Id: "sga9", Text: "hello"},
			want:  `{"ts":"2026-08-17T15:00:42.428Z","ev":"note","id":"sga9","text":"hello"}`,
		},
		{
			name:  "close",
			event: CloseEvent{Ts: "2026-08-17T15:32:34.977Z", Ev: "close", Id: "624q"},
			want:  `{"ts":"2026-08-17T15:32:34.977Z","ev":"close","id":"624q"}`,
		},
		{
			name:  "reopen",
			event: ReopenEvent{Ts: "2026-08-17T15:32:34.977Z", Ev: "reopen", Id: "624q"},
			want:  `{"ts":"2026-08-17T15:32:34.977Z","ev":"reopen","id":"624q"}`,
		},
		{
			name:  "dismiss",
			event: DismissEvent{Ts: "2026-08-15T01:52:04.587Z", Ev: "dismiss", Key: "pr:DevResults/DevResults#7237"},
			want:  `{"ts":"2026-08-15T01:52:04.587Z","ev":"dismiss","key":"pr:DevResults/DevResults#7237"}`,
		},
		{
			name:  "undismiss",
			event: UndismissEvent{Ts: "2026-08-15T01:52:04.587Z", Ev: "undismiss", Key: "review:a/b#1"},
			want:  `{"ts":"2026-08-15T01:52:04.587Z","ev":"undismiss","key":"review:a/b#1"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := encode(t, test.event); got != test.want {
				t.Errorf("Encode\n got %s\nwant %s", got, test.want)
			}
		})
	}
}

func TestEncodeEmptyTagsIsAnArrayNotNull(t *testing.T) {
	got := encode(t, AddEvent{Ts: "2026-08-17T15:32:04.642Z", Ev: "add", Id: "624q", Title: "tmp probe", Status: StatusOpen})
	want := `{"ts":"2026-08-17T15:32:04.642Z","ev":"add","id":"624q","title":"tmp probe",` +
		`"status":"open","origin":null,"session":null,"tags":[],"waitingOn":null}`
	if got != want {
		t.Errorf("Encode\n got %s\nwant %s", got, want)
	}
}

func TestEncodeLeavesHTMLCharactersRaw(t *testing.T) {
	got := encode(t, NoteEvent{Ts: "2026-08-17T15:00:42.428Z", Ev: "note", Id: "sga9", Text: "a <b> & c"})
	want := `{"ts":"2026-08-17T15:00:42.428Z","ev":"note","id":"sga9","text":"a <b> & c"}`
	if got != want {
		t.Errorf("Encode\n got %s\nwant %s", got, want)
	}
}

func TestEncodeLeavesLineSeparatorsRaw(t *testing.T) {
	got := encode(t, NoteEvent{Ts: "2026-08-17T15:00:42.428Z", Ev: "note", Id: "sga9", Text: "a\u2028b\u2029c"})
	want := "{\"ts\":\"2026-08-17T15:00:42.428Z\",\"ev\":\"note\",\"id\":\"sga9\",\"text\":\"a\u2028b\u2029c\"}"
	if got != want {
		t.Errorf("Encode\n got %q\nwant %q", got, want)
	}
}

// A payload that happens to contain the six characters of a \u2028 escape must survive as an
// escaped backslash rather than being folded into a line separator.
func TestEncodeKeepsLiteralEscapeText(t *testing.T) {
	got := encode(t, NoteEvent{Ts: "2026-08-17T15:00:42.428Z", Ev: "note", Id: "sga9", Text: `x\u2028y`})
	want := `{"ts":"2026-08-17T15:00:42.428Z","ev":"note","id":"sga9","text":"x\\u2028y"}`
	if got != want {
		t.Errorf("Encode\n got %s\nwant %s", got, want)
	}
	var decoded NoteEvent
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Text != `x\u2028y` {
		t.Errorf("round trip = %q, want %q", decoded.Text, `x\u2028y`)
	}
}

func TestEncodeUsesNodesControlCharacterEscapes(t *testing.T) {
	got := encode(t, NoteEvent{Ts: "2026-08-17T15:00:42.428Z", Ev: "note", Id: "sga9", Text: "\b\f\n\r\t\x01\x7f"})
	want := `{"ts":"2026-08-17T15:00:42.428Z","ev":"note","id":"sga9","text":"\b\f\n\r\t\u0001` + "\x7f" + `"}`
	if got != want {
		t.Errorf("Encode\n got %q\nwant %q", got, want)
	}
}

// Origin paths are opaque identifiers in the log, so a Windows path is stored exactly as given.
func TestEncodeStoresWindowsPathsLiterally(t *testing.T) {
	project := `C:\dev\dr\devresults\devresults\`
	got := encode(t, AddEvent{Ts: "2026-08-15T00:31:16.973Z", Ev: "add", Id: "sga9", Title: "t", Status: StatusOpen, Origin: &project})
	if !strings.Contains(got, `"origin":"C:\\dev\\dr\\devresults\\devresults\\"`) {
		t.Errorf("Encode = %s, want the path stored literally", got)
	}
	var decoded AddEvent
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Origin == nil || *decoded.Origin != project {
		t.Errorf("round trip = %v, want %q", decoded.Origin, project)
	}
}

func TestFormatTsMatchesToISOString(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "milliseconds kept",
			in:   time.Date(2026, 8, 17, 15, 32, 4, 642_000_000, time.UTC),
			want: "2026-08-17T15:32:04.642Z",
		},
		{
			name: "trailing zeros are not trimmed",
			in:   time.Date(2026, 8, 17, 15, 32, 4, 600_000_000, time.UTC),
			want: "2026-08-17T15:32:04.600Z",
		},
		{
			name: "whole second still carries milliseconds",
			in:   time.Date(2026, 8, 17, 15, 32, 4, 0, time.UTC),
			want: "2026-08-17T15:32:04.000Z",
		},
		{
			name: "sub-millisecond precision is dropped",
			in:   time.Date(2026, 8, 17, 15, 32, 4, 642_999_999, time.UTC),
			want: "2026-08-17T15:32:04.642Z",
		},
		{
			name: "converted to UTC",
			in:   time.Date(2026, 8, 17, 11, 32, 4, 642_000_000, time.FixedZone("EDT", -4*60*60)),
			want: "2026-08-17T15:32:04.642Z",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FormatTs(test.in); got != test.want {
				t.Errorf("FormatTs = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAppendStampsAndWritesOneLinePerEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	now := time.Date(2026, 8, 17, 15, 32, 4, 642_000_000, time.UTC)

	ts, err := Append(path, AddEvent{Ev: "add", Id: "624q", Title: "tmp probe", Status: StatusOpen}, now)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if ts != "2026-08-17T15:32:04.642Z" {
		t.Errorf("Append ts = %q, want the stamped now", ts)
	}
	if _, err := Append(path, CloseEvent{Ev: "close", Id: "624q"}, now.Add(30*time.Second)); err != nil {
		t.Fatalf("Append: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := `{"ts":"2026-08-17T15:32:04.642Z","ev":"add","id":"624q","title":"tmp probe","status":"open",` +
		`"origin":null,"session":null,"tags":[],"waitingOn":null}` + "\n" +
		`{"ts":"2026-08-17T15:32:34.642Z","ev":"close","id":"624q"}` + "\n"
	if string(raw) != want {
		t.Errorf("log\n got %s\nwant %s", raw, want)
	}
}

func TestAppendPreservesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	existing := `{"ts":"2026-08-15T00:31:16.973Z","ev":"add","id":"sga9","title":"t","status":"open","project":null,"session":null,"tags":[],"waitingOn":null}` + "\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Append(path, CloseEvent{Ev: "close", Id: "sga9"}, time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Append: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := existing + `{"ts":"2026-08-17T00:00:00.000Z","ev":"close","id":"sga9"}` + "\n"
	if string(raw) != want {
		t.Errorf("log\n got %s\nwant %s", raw, want)
	}
}

func TestAppendKeepsACallerSuppliedTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	event := DismissEvent{Ts: "2026-08-15T01:52:04.587Z", Ev: "dismiss", Key: "pr:a/b#1"}

	ts, err := Append(path, event, time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if ts != event.Ts {
		t.Errorf("Append ts = %q, want the supplied %q", ts, event.Ts)
	}
}

func TestAppendReportsAnUnwritablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "events.jsonl")
	if _, err := Append(path, CloseEvent{Ev: "close", Id: "624q"}, time.Now()); err == nil {
		t.Error("Append into a missing directory = nil error, want a failure")
	}
}

// The authoritative check: every line Node has already written must come back byte-identical when
// the Go writer re-encodes it. The one sanctioned difference is an add's provenance key, which
// stored lines spell `project` and the writer now spells `origin`; nothing else may move.
func TestEncodeReproducesTheRealLogByteForByte(t *testing.T) {
	raw, err := os.ReadFile(realLogPath())
	if err != nil {
		t.Skipf("real log unavailable: %v", err)
	}

	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	checked := 0
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		event := eventFromLine(t, index+1, line)
		if event == nil {
			continue
		}
		want := line
		if _, isAdd := event.(AddEvent); isAdd {
			want = originKeyRenamed(line)
		}
		if got := encode(t, event); got != want {
			t.Errorf("line %d\n got %s\nwant %s", index+1, got, want)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no events read from the real log")
	}
	t.Logf("re-encoded %d real lines", checked)
}

func TestRealLogTimestampsUseTheNodeLayout(t *testing.T) {
	raw, err := os.ReadFile(realLogPath())
	if err != nil {
		t.Skipf("real log unavailable: %v", err)
	}

	for index, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var probe struct {
			Ts string `json:"ts"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("line %d: %v", index+1, err)
		}
		parsed, err := time.Parse(TsLayout, probe.Ts)
		if err != nil {
			t.Errorf("line %d: ts %q does not match %q", index+1, probe.Ts, TsLayout)
			continue
		}
		if got := FormatTs(parsed); got != probe.Ts {
			t.Errorf("line %d: FormatTs = %q, want %q", index+1, got, probe.Ts)
		}
	}
}

// originKeyRenamed spells a stored add line's provenance key the way the writer spells it now. The
// key sits directly after status, which is what makes the rewrite unambiguous: the value, its
// position and its escaping are left exactly as stored.
func originKeyRenamed(line string) string {
	for _, status := range Statuses {
		prefix := `"status":"` + string(status) + `",`
		marker := prefix + `"project":`
		if index := strings.Index(line, marker); index != -1 {
			return line[:index] + prefix + `"origin":` + line[index+len(marker):]
		}
	}
	return line
}

// eventFromLine rebuilds the typed event a stored line came from, returning nil for shapes the Go
// build never writes.
func eventFromLine(t *testing.T, line int, raw string) WaidEvent {
	t.Helper()

	var probe struct {
		Ev string `json:"ev"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("line %d: %v", line, err)
	}

	decode := func(into any) {
		t.Helper()
		if err := json.Unmarshal([]byte(raw), into); err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
	}

	switch probe.Ev {
	case "add":
		var event AddEvent
		decode(&event)
		if event.Origin == nil {
			var legacy struct {
				Project *string `json:"project"`
			}
			decode(&legacy)
			event.Origin = legacy.Project
		}
		if event.Tags == nil {
			event.Tags = []string{}
		}
		return event
	case "note":
		var event NoteEvent
		decode(&event)
		return event
	case "close":
		var event CloseEvent
		decode(&event)
		return event
	case "reopen":
		var event ReopenEvent
		decode(&event)
		return event
	case "dismiss":
		var event DismissEvent
		decode(&event)
		return event
	case "undismiss":
		var event UndismissEvent
		decode(&event)
		return event
	default:
		return nil
	}
}

// A ParentEvent carries its parent whether or not there is one, since the fold reads the field's
// presence: an omitted parent leaves the item where it was, and only an explicit null moves it to
// the top level.
func TestEncodeParentEventCarriesAnExplicitNull(t *testing.T) {
	line, err := Encode(ParentEvent{Ts: "2026-08-17T12:00:00.000Z", Ev: "update", Id: "7k3m"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := `{"ts":"2026-08-17T12:00:00.000Z","ev":"update","id":"7k3m","parent":null}`
	if got := string(line); got != want {
		t.Errorf("Encode wrote\n%s\nwant\n%s", got, want)
	}
}

// An UpdateEvent carrying no project omits the field, so correcting a title or marking an item
// waiting cannot move it out of the project it is filed under.
func TestEncodeUpdateEventOmitsAnAbsentProject(t *testing.T) {
	title := "Push the tui branch"
	line, err := Encode(UpdateEvent{Ts: "2026-08-17T12:00:00.000Z", Ev: "update", Id: "7k3m", Title: &title})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if strings.Contains(string(line), "project") {
		t.Errorf("an update with no project wrote %s, want the field omitted", line)
	}
}

// An add carrying a parent writes it, so an item can land under one in a single line rather than an
// add followed by a move.
func TestEncodeAddEventCarriesItsParent(t *testing.T) {
	parent := "aaaa"
	got := encode(t, AddEvent{Ts: "2026-08-17T15:32:04.642Z", Ev: "add", Id: "624q", Title: "tmp probe", Status: StatusOpen, Parent: &parent})

	want := `{"ts":"2026-08-17T15:32:04.642Z","ev":"add","id":"624q","title":"tmp probe",` +
		`"status":"open","parent":"aaaa","origin":null,"session":null,"tags":[],"waitingOn":null}`
	if got != want {
		t.Errorf("Encode\n got %s\nwant %s", got, want)
	}
}

// A HeadingEvent always writes the field: an omitempty boolean could never say false, so a toggle
// that dropped the key would silently leave the heading marked.
func TestEncodeHeadingEventCarriesTheFlagBothWays(t *testing.T) {
	tests := []struct {
		event HeadingEvent
		want  string
	}{
		{
			event: HeadingEvent{Ts: "2026-08-24T12:00:00.000Z", Ev: "update", Id: "7k3m", Heading: true},
			want:  `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"7k3m","heading":true}`,
		},
		{
			event: HeadingEvent{Ts: "2026-08-24T12:00:00.000Z", Ev: "update", Id: "7k3m"},
			want:  `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"7k3m","heading":false}`,
		},
	}

	for _, test := range tests {
		if got := encode(t, test.event); got != test.want {
			t.Errorf("Encode wrote\n%s\nwant\n%s", got, test.want)
		}
	}
}

// An AddEvent that marks no heading omits the key, so every line already in the log re-encodes byte
// for byte.
func TestEncodeAddOmitsAnUnmarkedHeading(t *testing.T) {
	project := `C:\dev\waid`
	add := AddEvent{
		Ts:     "2026-08-15T00:31:16.973Z",
		Ev:     "add",
		Id:     "sga9",
		Title:  "Design template",
		Status: StatusOpen,
		Origin: &project,
		Tags:   []string{"bug"},
	}

	want := `{"ts":"2026-08-15T00:31:16.973Z","ev":"add","id":"sga9","title":"Design template",` +
		`"status":"open","origin":"C:\\dev\\waid","session":null,"tags":["bug"],"waitingOn":null}`
	if got := encode(t, add); got != want {
		t.Fatalf("Encode wrote\n%s\nwant\n%s", got, want)
	}

	add.Heading = true
	want = `{"ts":"2026-08-15T00:31:16.973Z","ev":"add","id":"sga9","title":"Design template",` +
		`"status":"open","origin":"C:\\dev\\waid","session":null,"tags":["bug"],"waitingOn":null,"heading":true}`
	if got := encode(t, add); got != want {
		t.Fatalf("Encode wrote\n%s\nwant\n%s", got, want)
	}
}

// A TagsEvent always writes the field: an omitempty array could never say "no tags", so a write
// that dropped the key would silently leave the tags standing.
func TestEncodeTagsEventCarriesTheSetBothWays(t *testing.T) {
	tests := []struct {
		event TagsEvent
		want  string
	}{
		{
			event: TagsEvent{Ts: "2026-08-24T12:00:00.000Z", Ev: "update", Id: "7k3m", Tags: []string{"bug", "ui"}},
			want:  `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"7k3m","tags":["bug","ui"]}`,
		},
		{
			event: TagsEvent{Ts: "2026-08-24T12:00:00.000Z", Ev: "update", Id: "7k3m", Tags: []string{}},
			want:  `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"7k3m","tags":[]}`,
		},
		{
			event: TagsEvent{Ts: "2026-08-24T12:00:00.000Z", Ev: "update", Id: "7k3m"},
			want:  `{"ts":"2026-08-24T12:00:00.000Z","ev":"update","id":"7k3m","tags":[]}`,
		},
	}

	for _, test := range tests {
		if got := encode(t, test.event); got != test.want {
			t.Errorf("Encode wrote\n%s\nwant\n%s", got, test.want)
		}
	}
}
