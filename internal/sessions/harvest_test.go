package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtures holds the transcripts ported from the Node tree, read relative to this package.
var fixtures = filepath.Join("..", "..", "testdata", "transcripts")

func ptr[T any](value T) *T { return &value }

// harvest streams a fixture through the accumulator exactly as the cache will: split on \n only, so
// a CRLF file hands each line a trailing \r.
func harvest(t *testing.T, name string, fallbackProject *string) Session {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(fixtures, name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}

	acc := NewAccumulator()
	for _, line := range strings.Split(string(raw), "\n") {
		acc.FeedLine(line)
	}
	return acc.Finalize(fallbackProject)
}

func assertSession(t *testing.T, got Session, want Session) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("session mismatch\n got: %s\nwant: %s", show(t, got), show(t, want))
	}
}

func show(t *testing.T, session Session) string {
	t.Helper()

	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("encoding session: %v", err)
	}
	return string(encoded)
}

func TestNormalSessionFinalizesWithLastAiTitleCwdBranchSpanAndPrompts(t *testing.T) {
	assertSession(t, harvest(t, "normal.jsonl", ptr(`C:\fallback`)), Session{
		Id:      "a1b2c3d4",
		Title:   "Fix budget chart legend overflow",
		Project: ptr(`C:\dev\waid`),
		Branch:  ptr("fix/legend"),
		Started: ptr("2026-08-14T09:00:00.000Z"),
		Ended:   ptr("2026-08-14T09:06:30.000Z"),
		Prompts: 2,
	})
}

func TestLastAiTitleWins(t *testing.T) {
	acc := NewAccumulator()
	for _, title := range []string{"first", "second", "third"} {
		acc.FeedLine(`{"type":"ai-title","aiTitle":"` + title + `","sessionId":"z9"}`)
	}

	if got := acc.Finalize(nil).Title; got != "third" {
		t.Errorf("title = %q, want %q", got, "third")
	}
}

func TestSidechainRecordsRaiseEndedButNotPrompts(t *testing.T) {
	session := harvest(t, "normal.jsonl", nil)

	// The 09:02 sidechain prompt sits between the two counted prompts and is not one of them.
	if session.Prompts != 2 {
		t.Errorf("prompts = %d, want 2", session.Prompts)
	}
	if got := deref(session.Ended); got != "2026-08-14T09:06:30.000Z" {
		t.Errorf("ended = %q, want %q", got, "2026-08-14T09:06:30.000Z")
	}
}

func TestSessionWithNoAiTitleTitlesItselfFromFirstNonMetaUserMessage(t *testing.T) {
	session := harvest(t, "no-ai-title.jsonl", nil)

	want := "Investigate why the nightly export job silently drops rows whenever the source t"
	if session.Title != want {
		t.Errorf("title = %q, want %q", session.Title, want)
	}
	if len(session.Title) != 80 {
		t.Errorf("title length = %d, want 80", len(session.Title))
	}
	assertSession(t, session, Session{
		Id:      "b2c3d4e5",
		Title:   want,
		Project: ptr(`C:\dev\demo`),
		Branch:  ptr("export/fix"),
		Started: ptr("2026-08-14T10:00:00.000Z"),
		Ended:   ptr("2026-08-14T10:04:00.000Z"),
		Prompts: 2,
	})
}

func TestSidechainOnlySessionCountsZeroPromptsAndFallsBackForItsProject(t *testing.T) {
	assertSession(t, harvest(t, "sidechain-only.jsonl", ptr(`C:\dev\decoded`)), Session{
		Id:      "c3d4e5f6",
		Title:   "Grep for every caller of renderLegend.",
		Project: ptr(`C:\dev\decoded`),
		Branch:  ptr("agent-work"),
		Started: ptr("2026-08-14T11:00:00.000Z"),
		Ended:   ptr("2026-08-14T11:00:30.000Z"),
		Prompts: 0,
	})
}

func TestTruncatedFinalLineIsSkippedAndTheRestStillParses(t *testing.T) {
	assertSession(t, harvest(t, "truncated.jsonl", nil), Session{
		Id:      "d4e5f6a7",
		Title:   "Run the migration",
		Project: ptr(`C:\dev\waid`),
		Branch:  ptr("main"),
		Started: ptr("2026-08-14T12:00:00.000Z"),
		Ended:   ptr("2026-08-14T12:01:00.000Z"),
		Prompts: 2,
	})
}

func TestCrlfLinesParseIdenticallyToLf(t *testing.T) {
	fallback := ptr(`C:\fallback`)
	assertSession(t, harvest(t, "crlf.jsonl", fallback), harvest(t, "normal.jsonl", fallback))
}

func TestEmptyFileFinalizesToUntitledWithZeroPrompts(t *testing.T) {
	assertSession(t, harvest(t, "empty.jsonl", ptr(`C:\dev\decoded`)), Session{
		Id:      "",
		Title:   "(untitled)",
		Project: ptr(`C:\dev\decoded`),
		Branch:  nil,
		Started: nil,
		Ended:   nil,
		Prompts: 0,
	})
}

func TestRecordsThatAreNotJsonObjectsAreIgnored(t *testing.T) {
	acc := NewAccumulator()
	for _, line := range []string{"", "   ", "null", "[1,2,3]", `"a string"`, "{oops"} {
		acc.FeedLine(line)
	}

	assertSession(t, acc.Finalize(nil), Session{
		Id:      "",
		Title:   "(untitled)",
		Project: nil,
		Branch:  nil,
		Started: nil,
		Ended:   nil,
		Prompts: 0,
	})
}

func TestDecodeProjectSlug(t *testing.T) {
	cases := []struct{ slug, want string }{
		{"C--dev-waid", `C:\dev\waid`},
		{"c--dev-dr-devresults", `C:\dev\dr\devresults`},
		{"-home-brent-dev", "/home/brent/dev"},
		{"C--", `C:\`},
		{"not-a-slug", "not-a-slug"},
		{"", ""},
	}

	for _, testCase := range cases {
		if got := DecodeProjectSlug(testCase.slug); got != testCase.want {
			t.Errorf("DecodeProjectSlug(%q) = %q, want %q", testCase.slug, got, testCase.want)
		}
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
