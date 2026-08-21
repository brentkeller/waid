package events

import (
	"reflect"
	"testing"
)

// ptr returns a pointer to value, for the nullable item fields.
func ptr[T any](value T) *T { return &value }

// reasons reduces problems to the line-and-reason pairs the assertions compare.
func reasons(problems []Problem) [][2]any {
	pairs := make([][2]any, 0, len(problems))
	for _, problem := range problems {
		pairs = append(pairs, [2]any{problem.Line, string(problem.Reason)})
	}
	return pairs
}

func TestAddNoteUpdateAndCloseFoldToOneItem(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:22:01.004Z","ev":"add","id":"k3f9","title":"Chart legend overflows at 4+ series","project":"C:\\dev\\dr\\devresults","status":"open","session":"34782fc3","tags":["bug"]}`,
		`{"ts":"2026-08-14T19:02:55.881Z","ev":"note","id":"k3f9","text":"repro only in Firefox"}`,
		`{"ts":"2026-08-15T14:10:02.113Z","ev":"update","id":"k3f9","status":"waiting","waitingOn":"design review"}`,
		`{"ts":"2026-08-19T09:31:44.207Z","ev":"close","id":"k3f9"}`,
	})

	if len(state.Problems) != 0 {
		t.Fatalf("problems = %v, want none", state.Problems)
	}
	want := []Item{{
		Id:        "k3f9",
		Title:     "Chart legend overflows at 4+ series",
		Status:    StatusDone,
		WaitingOn: ptr("design review"),
		Origin:    ptr(`C:\dev\dr\devresults`),
		Session:   ptr("34782fc3"),
		Tags:      []string{"bug"},
		Notes:     []Note{{Ts: "2026-08-14T19:02:55.881Z", Text: "repro only in Firefox"}},
		Created:   "2026-08-14T18:22:01.004Z",
		Updated:   "2026-08-19T09:31:44.207Z",
	}}
	if !reflect.DeepEqual(state.Items, want) {
		t.Fatalf("items = %+v, want %+v", state.Items, want)
	}
}

func TestAddAppliesDefaultsForEveryOmittedField(t *testing.T) {
	state := Fold([]string{`{"ts":"2026-08-14T18:22:01.004Z","ev":"add","id":"aaaa","title":"bare"}`})

	want := []Item{{
		Id:      "aaaa",
		Title:   "bare",
		Status:  StatusOpen,
		Tags:    []string{},
		Notes:   []Note{},
		Created: "2026-08-14T18:22:01.004Z",
		Updated: "2026-08-14T18:22:01.004Z",
	}}
	if !reflect.DeepEqual(state.Items, want) {
		t.Fatalf("items = %+v, want %+v", state.Items, want)
	}
}

func TestUpdatePatchesOnlyTheSuppliedFields(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"original","project":"C:\\dev\\waid","tags":["bug"],"session":"sess-1"}`,
		`{"ts":"2026-08-15T18:00:00.000Z","ev":"update","id":"aaaa","title":"renamed"}`,
	})

	if len(state.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(state.Items))
	}
	item := state.Items[0]
	if item.Title != "renamed" {
		t.Fatalf("title = %q, want %q", item.Title, "renamed")
	}
	if item.Origin == nil || *item.Origin != `C:\dev\waid` {
		t.Fatalf("project = %v, want C:\\dev\\waid", item.Origin)
	}
	if !reflect.DeepEqual(item.Tags, []string{"bug"}) {
		t.Fatalf("tags = %v, want [bug]", item.Tags)
	}
	if item.Session == nil || *item.Session != "sess-1" {
		t.Fatalf("session = %v, want sess-1", item.Session)
	}
	if item.Status != StatusOpen {
		t.Fatalf("status = %q, want open", item.Status)
	}
	if item.Created != "2026-08-14T18:00:00.000Z" {
		t.Fatalf("created = %q", item.Created)
	}
	if item.Updated != "2026-08-15T18:00:00.000Z" {
		t.Fatalf("updated = %q", item.Updated)
	}
}

func TestReopenSetsStatusOpenAndClearsWaitingOn(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"blocked","status":"waiting","waitingOn":"review"}`,
		`{"ts":"2026-08-15T18:00:00.000Z","ev":"reopen","id":"aaaa"}`,
	})

	if len(state.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(state.Items))
	}
	if state.Items[0].Status != StatusOpen {
		t.Fatalf("status = %q, want open", state.Items[0].Status)
	}
	if state.Items[0].WaitingOn != nil {
		t.Fatalf("waitingOn = %v, want nil", *state.Items[0].WaitingOn)
	}
}

func TestFileOrderWinsOverTimestamps(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"first"}`,
		`{"ts":"2026-08-20T18:00:00.000Z","ev":"update","id":"aaaa","title":"later clock"}`,
		`{"ts":"2026-08-01T09:00:00.000Z","ev":"update","id":"aaaa","title":"earlier clock"}`,
	})

	if len(state.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(state.Items))
	}
	if state.Items[0].Title != "earlier clock" {
		t.Fatalf("title = %q, want the last line's title", state.Items[0].Title)
	}
	if state.Items[0].Updated != "2026-08-01T09:00:00.000Z" {
		t.Fatalf("updated = %q, want the last line's ts", state.Items[0].Updated)
	}
}

func TestItemsKeepTheirFirstSeenOrder(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"bbbb","title":"second"}`,
		`{"ts":"2026-08-14T18:00:01.000Z","ev":"add","id":"aaaa","title":"first"}`,
		`{"ts":"2026-08-14T18:00:02.000Z","ev":"update","id":"bbbb","title":"still second"}`,
	})

	ids := []string{}
	for _, item := range state.Items {
		ids = append(ids, item.Id)
	}
	if !reflect.DeepEqual(ids, []string{"bbbb", "aaaa"}) {
		t.Fatalf("ids = %v, want [bbbb aaaa]", ids)
	}
}

func TestDismissAndUndismissMaintainTheKeySet(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"dismiss","key":"pr:o/r#123"}`,
		`{"ts":"2026-08-14T18:00:01.000Z","ev":"dismiss","key":"pr:o/r#124"}`,
		`{"ts":"2026-08-14T18:00:02.000Z","ev":"dismiss","key":"pr:o/r#123"}`,
		`{"ts":"2026-08-14T18:00:03.000Z","ev":"undismiss","key":"pr:o/r#124"}`,
		`{"ts":"2026-08-14T18:00:04.000Z","ev":"undismiss","key":"dirty:C:\\dev\\waid"}`,
	})

	if !reflect.DeepEqual(state.Dismissed, []string{"pr:o/r#123"}) {
		t.Fatalf("dismissed = %v, want [pr:o/r#123]", state.Dismissed)
	}
	if len(state.Problems) != 0 {
		t.Fatalf("problems = %v, want none", state.Problems)
	}
}

func TestMixedMalformedBatchYieldsSurvivorsAndTheExactProblemList(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"good"}`,
		"",
		"   ",
		"{not json",
		`["an","array"]`,
		`{"ts":"2026-08-14T18:00:01.000Z","ev":"note","id":"zzzz","text":"orphan"}`,
		`{"ts":"2026-08-14T18:00:02.000Z","ev":"teleport","id":"aaaa"}`,
		`{"ts":"2026-08-14T18:00:03.000Z","ev":"add","id":"aaaa","title":"duplicate"}`,
		`{"ts":"2026-08-14T18:00:04.000Z","ev":"add","id":"bbbb"}`,
		`{"ts":"2026-08-14T18:00:05.000Z","ev":"update","id":"aaaa","status":"sideways"}`,
		`{"ts":"2026-08-14T18:00:06.000Z","ev":"note","id":"aaaa"}`,
		`{"ts":"2026-08-14T18:00:07.000Z","ev":"dismiss"}`,
	})

	survivors := [][3]string{}
	for _, item := range state.Items {
		survivors = append(survivors, [3]string{item.Id, item.Title, string(item.Status)})
	}
	if !reflect.DeepEqual(survivors, [][3]string{{"aaaa", "good", "open"}}) {
		t.Fatalf("items = %v, want the one good add", survivors)
	}
	if len(state.Dismissed) != 0 {
		t.Fatalf("dismissed = %v, want none", state.Dismissed)
	}

	want := [][2]any{
		{4, "unparseable"},
		{5, "not-an-object"},
		{6, "unknown-id"},
		{7, "unknown-ev"},
		{8, "duplicate-id"},
		{9, "add-missing-fields"},
		{10, "bad-status"},
		{11, "note-missing-text"},
		{12, "missing-key"},
	}
	if got := reasons(state.Problems); !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %v, want %v", got, want)
	}
	if got, expected := state.Problems[2], (Problem{Line: 6, Reason: ReasonUnknownId, Id: ptr("zzzz"), Ev: ptr("note")}); !reflect.DeepEqual(got, expected) {
		t.Fatalf("problems[2] = %+v, want %+v", got, expected)
	}
	if got, expected := state.Problems[3], (Problem{Line: 7, Reason: ReasonUnknownEv, Id: ptr("aaaa"), Ev: ptr("teleport")}); !reflect.DeepEqual(got, expected) {
		t.Fatalf("problems[3] = %+v, want %+v", got, expected)
	}
	if state.Problems[0].Id != nil || state.Problems[0].Ev != nil {
		t.Fatalf("problems[0] = %+v, want null id and ev", state.Problems[0])
	}
}

func TestBadStatusIsReportedWithoutDiscardingTheRestOfTheEvent(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"kept","status":"sideways"}`,
		`{"ts":"2026-08-14T18:00:01.000Z","ev":"update","id":"aaaa","status":"nope","title":"still patched"}`,
	})

	if len(state.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(state.Items))
	}
	if state.Items[0].Status != StatusOpen {
		t.Fatalf("status = %q, want open", state.Items[0].Status)
	}
	if state.Items[0].Title != "still patched" {
		t.Fatalf("title = %q, want %q", state.Items[0].Title, "still patched")
	}
	want := [][2]any{{1, "bad-status"}, {2, "bad-status"}}
	if got := reasons(state.Problems); !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %v, want %v", got, want)
	}
}

func TestEveryProblemReasonHasACase(t *testing.T) {
	seen := map[ProblemReason]bool{}
	state := Fold([]string{
		"{not json",
		`42`,
		`{"ev":"add","id":"aaaa"}`,
		`{"ev":"add","id":"bbbb","title":"one"}`,
		`{"ev":"add","id":"bbbb","title":"again"}`,
		`{"ev":"note","id":"nope","text":"orphan"}`,
		`{"ev":"note","id":"bbbb"}`,
		`{"ev":"update","id":"bbbb","status":"sideways"}`,
		`{"ev":"dismiss"}`,
		`{"ev":"teleport"}`,
	})
	for _, problem := range state.Problems {
		seen[problem.Reason] = true
	}

	for _, reason := range []ProblemReason{
		ReasonUnparseable, ReasonNotAnObject, ReasonAddMissingField, ReasonDuplicateId,
		ReasonUnknownId, ReasonNoteMissingText, ReasonBadStatus, ReasonMissingKey, ReasonUnknownEv,
	} {
		if !seen[reason] {
			t.Errorf("reason %q was never reported", reason)
		}
	}
}

func TestFoldNeverPanicsOnHostileInput(t *testing.T) {
	state := Fold([]string{"null", "42", `"a string"`, "[]", "{}", `{"ev":"add"}`})

	if len(state.Items) != 0 {
		t.Fatalf("items = %v, want none", state.Items)
	}
	if len(state.Problems) != 6 {
		t.Fatalf("problems = %d, want 6", len(state.Problems))
	}
}

func TestFoldOfAnEmptyLogReturnsEmptySlices(t *testing.T) {
	state := Fold(nil)

	if !reflect.DeepEqual(state, State{Items: []Item{}, Dismissed: []string{}, Problems: []Problem{}}) {
		t.Fatalf("state = %+v, want empty non-nil slices", state)
	}
}

// An origin written as null clears it, which is what makes an assignment reversible for an item
// that had none to begin with.
func TestUpdateWithANullProjectClearsTheOrigin(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"filed","project":"C:\\dev\\waid"}`,
		`{"ts":"2026-08-15T18:00:00.000Z","ev":"update","id":"aaaa","project":null}`,
	})

	if len(state.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(state.Items))
	}
	if origin := state.Items[0].Origin; origin != nil {
		t.Fatalf("origin = %q, want it cleared", *origin)
	}
}

// A path on the legacy project key lands on the origin, leaving everything else the item holds
// alone.
func TestUpdateWithAProjectSetsTheOrigin(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"filed","status":"waiting","waitingOn":"maria"}`,
		`{"ts":"2026-08-15T18:00:00.000Z","ev":"update","id":"aaaa","project":"C:\\dev\\waid"}`,
	})

	item := state.Items[0]
	if item.Origin == nil || *item.Origin != `C:\dev\waid` {
		t.Fatalf("origin = %v, want C:\\dev\\waid", item.Origin)
	}
	if item.Status != StatusWaiting || item.WaitingOn == nil || *item.WaitingOn != "maria" {
		t.Fatalf("setting the origin moved the status to %q waiting on %v", item.Status, item.WaitingOn)
	}
	if item.Title != "filed" {
		t.Fatalf("title = %q, want it left alone", item.Title)
	}
}

// The rename is read-compatible: lines written before it carry `project` and lines written after it
// carry `origin`, and the fold cannot tell the two apart.
func TestAddReadsOriginFromEitherKey(t *testing.T) {
	legacy := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"filed","project":"C:\\dev\\waid"}`,
	})
	current := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"filed","origin":"C:\\dev\\waid"}`,
	})

	if !reflect.DeepEqual(legacy.Items, current.Items) {
		t.Fatalf("project folded to %+v, origin folded to %+v", legacy.Items, current.Items)
	}
	if legacy.Items[0].Origin == nil || *legacy.Items[0].Origin != `C:\dev\waid` {
		t.Fatalf("origin = %v, want C:\\dev\\waid", legacy.Items[0].Origin)
	}
}

// An add carrying both keys prefers origin, since a line holding both was written by a build that
// knows the new name.
func TestAddPrefersOriginOverProject(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"filed","project":"C:\\dev\\old","origin":"C:\\dev\\waid"}`,
	})

	if origin := state.Items[0].Origin; origin == nil || *origin != `C:\dev\waid` {
		t.Fatalf("origin = %v, want C:\\dev\\waid", origin)
	}
}

func TestUpdateWithAnOriginOverwritesIt(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"filed","project":"C:\\dev\\old"}`,
		`{"ts":"2026-08-15T18:00:00.000Z","ev":"update","id":"aaaa","origin":"C:\\dev\\waid"}`,
	})

	if origin := state.Items[0].Origin; origin == nil || *origin != `C:\dev\waid` {
		t.Fatalf("origin = %v, want C:\\dev\\waid", origin)
	}
}

func TestUpdateCarryingNeitherKeyLeavesTheOriginAlone(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"filed","origin":"C:\\dev\\waid"}`,
		`{"ts":"2026-08-15T18:00:00.000Z","ev":"update","id":"aaaa","title":"renamed"}`,
	})

	if origin := state.Items[0].Origin; origin == nil || *origin != `C:\dev\waid` {
		t.Fatalf("origin = %v, want it left alone", origin)
	}
}

// An update carrying a null origin clears it, the same as the legacy key does.
func TestUpdateWithANullOriginClearsIt(t *testing.T) {
	state := Fold([]string{
		`{"ts":"2026-08-14T18:00:00.000Z","ev":"add","id":"aaaa","title":"filed","origin":"C:\\dev\\waid"}`,
		`{"ts":"2026-08-15T18:00:00.000Z","ev":"update","id":"aaaa","origin":null}`,
	})

	if origin := state.Items[0].Origin; origin != nil {
		t.Fatalf("origin = %q, want it cleared", *origin)
	}
}
