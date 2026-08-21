package events

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// foldLines folds raw lines the way Load would, without touching disk.
func foldLines(lines ...string) State { return Fold(lines) }

// addLine writes an add line, giving parent when one is supplied.
func addLine(id string, title string, parent string) string {
	line := fmt.Sprintf(`{"ts":"2026-08-14T09:00:00.000Z","ev":"add","id":%q,"title":%q`, id, title)
	if parent != "" {
		line += fmt.Sprintf(`,"parent":%q`, parent)
	}
	return line + "}"
}

// A few hundred items is well past the point where a linear scan shows up in a cycle check or an
// ancestor walk, so the index is exercised at the size the tree work actually hits it with.
func TestFindOnALargeLogAnswersForEveryItem(t *testing.T) {
	lines := []string{}
	for index := range 400 {
		lines = append(lines, addLine(fmt.Sprintf("i%03d", index), fmt.Sprintf("Item %d", index), ""))
	}
	state := Fold(lines)

	for index := range 400 {
		id := fmt.Sprintf("i%03d", index)
		item, found := state.Find(id)
		if !found || item.Title != fmt.Sprintf("Item %d", index) {
			t.Fatalf("Find(%s) = %+v, %v", id, item, found)
		}
	}
	if _, found := state.Find("nope"); found {
		t.Errorf("Find(nope) found an item the log does not hold")
	}
}

func TestChildrenReturnsDirectChildrenInLogOrder(t *testing.T) {
	state := foldLines(
		addLine("root", "Root", ""),
		addLine("bbbb", "Second", "root"),
		addLine("aaaa", "First", "root"),
		addLine("deep", "Grandchild", "aaaa"),
		addLine("else", "Elsewhere", ""),
	)

	got := []string{}
	for _, child := range state.Children("root") {
		got = append(got, child.Id)
	}
	if !reflect.DeepEqual(got, []string{"bbbb", "aaaa"}) {
		t.Errorf("Children(root) = %v, want the direct children in log order", got)
	}
	if children := state.Children("deep"); len(children) != 0 {
		t.Errorf("Children(deep) = %v, want none", children)
	}
}

func TestAncestorsWalksToTheRootAndStops(t *testing.T) {
	state := foldLines(
		addLine("root", "Root", ""),
		addLine("mid1", "Middle", "root"),
		addLine("leaf", "Leaf", "mid1"),
	)

	got := []string{}
	for _, ancestor := range state.Ancestors("leaf") {
		got = append(got, ancestor.Id)
	}
	if !reflect.DeepEqual(got, []string{"mid1", "root"}) {
		t.Errorf("Ancestors(leaf) = %v, want nearest first up to the root", got)
	}
	if ancestors := state.Ancestors("root"); len(ancestors) != 0 {
		t.Errorf("Ancestors(root) = %v, want none", ancestors)
	}
}

// A parent id nothing holds ends the walk rather than failing it — a corrupt log still answers.
func TestAncestorsStopsAtAParentTheLogDoesNotHold(t *testing.T) {
	state := foldLines(addLine("leaf", "Leaf", "gone"))

	if ancestors := state.Ancestors("leaf"); len(ancestors) != 0 {
		t.Errorf("Ancestors(leaf) = %v, want none for a parent the log does not hold", ancestors)
	}
}

// The fold stores parents verbatim, so a looping chain reaches these methods; they end the walk
// rather than spinning. internal/tree reports the loop.
func TestAncestorsEndsOnALoopingChain(t *testing.T) {
	state := foldLines(
		`{"ts":"2026-08-14T09:00:00.000Z","ev":"add","id":"aaaa","title":"A","parent":"bbbb"}`,
		`{"ts":"2026-08-14T09:00:00.000Z","ev":"add","id":"bbbb","title":"B","parent":"aaaa"}`,
		`{"ts":"2026-08-14T09:00:00.000Z","ev":"add","id":"self","title":"S","parent":"self"}`,
	)

	got := []string{}
	for _, ancestor := range state.Ancestors("aaaa") {
		got = append(got, ancestor.Id)
	}
	if !reflect.DeepEqual(got, []string{"bbbb"}) {
		t.Errorf("Ancestors(aaaa) = %v, want the walk to stop where it loops", got)
	}
	if ancestors := state.Ancestors("self"); len(ancestors) != 0 {
		t.Errorf("Ancestors(self) = %v, want none for a self-parent", ancestors)
	}
}

func TestChildrenAndAncestorsAreEmptyForAnUnknownId(t *testing.T) {
	state := foldLines(addLine("aaaa", "One", ""))

	if children := state.Children("zzzz"); len(children) != 0 {
		t.Errorf("Children(zzzz) = %v, want none", children)
	}
	if ancestors := state.Ancestors("zzzz"); len(ancestors) != 0 {
		t.Errorf("Ancestors(zzzz) = %v, want none", ancestors)
	}
}

// The index is derived, so a State built by hand or decoded from JSON still answers.
func TestTheQueriesAnswerOnAStateBuiltWithoutAnIndex(t *testing.T) {
	root := "root"
	state := State{Items: []Item{
		{Id: "root", Title: "Root"},
		{Id: "leaf", Title: "Leaf", Parent: &root},
	}}

	if item, found := state.Find("leaf"); !found || item.Title != "Leaf" {
		t.Errorf("Find(leaf) = %+v, %v", item, found)
	}
	if children := state.Children("root"); len(children) != 1 || children[0].Id != "leaf" {
		t.Errorf("Children(root) = %+v", children)
	}
	if ancestors := state.Ancestors("leaf"); len(ancestors) != 1 || ancestors[0].Id != "root" {
		t.Errorf("Ancestors(leaf) = %+v", ancestors)
	}
}

// The index is not part of the encoded shape, so --json output is what it was.
func TestTheIndexIsNotEncoded(t *testing.T) {
	state := foldLines(addLine("root", "Root", ""), addLine("leaf", "Leaf", "root"))

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encoding the state: %v", err)
	}
	for _, unwanted := range []string{"index", "children", "byId"} {
		if strings.Contains(string(encoded), unwanted) {
			t.Errorf("encoded state names %q: %s", unwanted, encoded)
		}
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatalf("decoding the state: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("encoded keys = %v, want items, dismissed and problems only", keys)
	}
}
