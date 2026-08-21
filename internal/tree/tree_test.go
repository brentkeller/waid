package tree

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/events"
)

// addLine writes an add line, giving parent when one is supplied.
func addLine(id string, title string, parent string) string {
	line := fmt.Sprintf(`{"ts":"2026-08-14T09:00:00.000Z","ev":"add","id":%q,"title":%q`, id, title)
	if parent != "" {
		line += fmt.Sprintf(`,"parent":%q`, parent)
	}
	return line + "}"
}

// shape renders a forest as indented ids, so a test states the nesting it expects rather than
// walking the nodes by hand.
func shape(nodes []Node) string {
	lines := []string{}
	var walk func(nodes []Node, depth int)
	walk = func(nodes []Node, depth int) {
		for _, node := range nodes {
			lines = append(lines, strings.Repeat("  ", depth)+node.Item.Id)
			walk(node.Children, depth+1)
		}
	}
	walk(nodes, 0)
	return strings.Join(lines, "\n")
}

func TestBuildOnAFlatLogYieldsAllRoots(t *testing.T) {
	state := events.Fold([]string{
		addLine("aaaa", "First", ""),
		addLine("bbbb", "Second", ""),
		addLine("cccc", "Third", ""),
	})

	roots, anomalies := Build(state)
	if got, want := shape(roots), "aaaa\nbbbb\ncccc"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
	if len(anomalies) != 0 {
		t.Errorf("anomalies = %+v, want none", anomalies)
	}
}

func TestBuildNestsAccordingToParents(t *testing.T) {
	state := events.Fold([]string{
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
		addLine("kid2", "Second child", "root"),
		addLine("gkid", "Grandchild", "kid1"),
		addLine("solo", "Elsewhere", ""),
	})

	roots, anomalies := Build(state)
	want := strings.Join([]string{"root", "  kid1", "    gkid", "  kid2", "solo"}, "\n")
	if got := shape(roots); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
	if len(anomalies) != 0 {
		t.Errorf("anomalies = %+v, want none", anomalies)
	}
}

func TestBuildCarriesTheWholeItem(t *testing.T) {
	state := events.Fold([]string{addLine("aaaa", "Only", "")})

	roots, _ := Build(state)
	if len(roots) != 1 || roots[0].Item.Title != "Only" {
		t.Fatalf("roots = %+v, want the folded item", roots)
	}
}

func TestBuildTreatsAnUnknownParentAsTopLevel(t *testing.T) {
	state := events.Fold([]string{
		addLine("aaaa", "Anchored", ""),
		addLine("lost", "Orphan", "gone"),
	})

	roots, anomalies := Build(state)
	if got, want := shape(roots), "aaaa\nlost"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
	if want := []Anomaly{{Id: "lost", Kind: UnknownParent}}; !reflect.DeepEqual(anomalies, want) {
		t.Errorf("anomalies = %+v, want %+v", anomalies, want)
	}
}

func TestBuildTreatsATwoNodeCycleAsTwoRoots(t *testing.T) {
	state := events.Fold([]string{
		addLine("aaaa", "A", ""),
		addLine("bbbb", "B", "aaaa"),
		`{"ts":"2026-08-14T09:01:00.000Z","ev":"update","id":"aaaa","parent":"bbbb"}`,
	})

	roots, anomalies := Build(state)
	if got, want := shape(roots), "aaaa\nbbbb"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
	want := []Anomaly{{Id: "aaaa", Kind: ParentCycle}, {Id: "bbbb", Kind: ParentCycle}}
	if !reflect.DeepEqual(anomalies, want) {
		t.Errorf("anomalies = %+v, want %+v", anomalies, want)
	}
}

func TestBuildTreatsASelfParentAsARoot(t *testing.T) {
	state := events.Fold([]string{
		addLine("aaaa", "A", ""),
		`{"ts":"2026-08-14T09:01:00.000Z","ev":"update","id":"aaaa","parent":"aaaa"}`,
	})

	roots, anomalies := Build(state)
	if got, want := shape(roots), "aaaa"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
	if want := []Anomaly{{Id: "aaaa", Kind: ParentCycle}}; !reflect.DeepEqual(anomalies, want) {
		t.Errorf("anomalies = %+v, want %+v", anomalies, want)
	}
}

// An item hanging below a cycle cannot be drawn under its parent without drawing the loop, so it is
// lifted to the top level too rather than disappearing from the tree.
func TestBuildLiftsAnItemHangingBelowACycle(t *testing.T) {
	state := events.Fold([]string{
		addLine("aaaa", "A", ""),
		addLine("bbbb", "B", "aaaa"),
		addLine("cccc", "C", "aaaa"),
		`{"ts":"2026-08-14T09:01:00.000Z","ev":"update","id":"aaaa","parent":"bbbb"}`,
	})

	roots, anomalies := Build(state)
	if got, want := shape(roots), "aaaa\nbbbb\ncccc"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
	if len(anomalies) != 3 {
		t.Errorf("anomalies = %+v, want one per item caught in the loop", anomalies)
	}
}

func TestBuildNestsADeepChainToFullDepth(t *testing.T) {
	lines := []string{addLine("i000", "Item 0", "")}
	for depth := 1; depth < 50; depth++ {
		lines = append(lines, addLine(fmt.Sprintf("i%03d", depth), fmt.Sprintf("Item %d", depth), fmt.Sprintf("i%03d", depth-1)))
	}

	roots, anomalies := Build(events.Fold(lines))
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want the one chain head", len(roots))
	}
	if len(anomalies) != 0 {
		t.Errorf("anomalies = %+v, want none", anomalies)
	}

	depth, node := 1, roots[0]
	for len(node.Children) > 0 {
		if len(node.Children) != 1 {
			t.Fatalf("%s has %d children, want one", node.Item.Id, len(node.Children))
		}
		node = node.Children[0]
		depth++
	}
	if depth != 50 {
		t.Errorf("depth = %d, want 50", depth)
	}
}

func TestBuildOnAnEmptyLogYieldsNothing(t *testing.T) {
	roots, anomalies := Build(events.Fold(nil))
	if len(roots) != 0 || len(anomalies) != 0 {
		t.Errorf("Build(empty) = %+v, %+v, want neither", roots, anomalies)
	}
}
