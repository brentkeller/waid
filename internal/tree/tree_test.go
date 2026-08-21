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

// shape renders a forest as indented ids — or titles, for the synthetic nodes that have no id — so
// a test states the nesting it expects rather than walking the nodes by hand.
func shape(nodes []Node) string {
	lines := []string{}
	var walk func(nodes []Node, depth int)
	walk = func(nodes []Node, depth int) {
		for _, node := range nodes {
			label := node.Item.Id
			if label == "" {
				label = node.Item.Title
			}
			lines = append(lines, strings.Repeat("  ", depth)+label)
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

// addAt writes an add line at a given timestamp, so a test can state the order the log holds
// separately from the order the tree should render.
func addAt(ts string, id string, title string, parent string) string {
	line := fmt.Sprintf(`{"ts":%q,"ev":"add","id":%q,"title":%q`, ts, id, title)
	if parent != "" {
		line += fmt.Sprintf(`,"parent":%q`, parent)
	}
	return line + "}"
}

func TestBuildSortsParentsBeforeLeaves(t *testing.T) {
	state := events.Fold([]string{
		addLine("leaf", "Zulu leaf", ""),
		addLine("head", "Alpha parent", ""),
		addLine("kid1", "Child", "head"),
	})

	roots, _ := Build(state)
	if got, want := shape(roots), "head\n  kid1\nleaf"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// A parent sorts by title however recently it was touched, so headings stay where the eye left them.
func TestBuildSortsParentsByTitle(t *testing.T) {
	state := events.Fold([]string{
		addAt("2026-08-14T09:00:00.000Z", "zulu", "Zulu", ""),
		addAt("2026-08-14T09:01:00.000Z", "kidz", "Child of Zulu", "zulu"),
		addAt("2026-08-14T09:02:00.000Z", "alfa", "alpha", ""),
		addAt("2026-08-14T09:03:00.000Z", "kida", "Child of alpha", "alfa"),
		addAt("2026-08-14T09:04:00.000Z", "midl", "Middle", ""),
		addAt("2026-08-14T09:05:00.000Z", "kidm", "Child of Middle", "midl"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"alfa", "  kida", "midl", "  kidm", "zulu", "  kidz"}, "\n")
	if got := shape(roots); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

func TestBuildSortsLeavesByUpdatedDescending(t *testing.T) {
	state := events.Fold([]string{
		addAt("2026-08-14T09:00:00.000Z", "oldd", "Oldest", ""),
		addAt("2026-08-14T09:01:00.000Z", "midd", "Middle", ""),
		addAt("2026-08-14T09:02:00.000Z", "neww", "Newest", ""),
		`{"ts":"2026-08-14T09:03:00.000Z","ev":"note","id":"oldd","text":"touched"}`,
	})

	roots, _ := Build(state)
	if got, want := shape(roots), "oldd\nneww\nmidd"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// Retitling a parent moves it; touching a leaf reorders the leaves and leaves the headings alone.
func TestBuildReordersOnRetitleButNotOnATouchedLeaf(t *testing.T) {
	lines := []string{
		addAt("2026-08-14T09:00:00.000Z", "alfa", "Alpha", ""),
		addAt("2026-08-14T09:01:00.000Z", "kida", "Child of Alpha", "alfa"),
		addAt("2026-08-14T09:02:00.000Z", "zulu", "Zulu", ""),
		addAt("2026-08-14T09:03:00.000Z", "kidz", "Child of Zulu", "zulu"),
		addAt("2026-08-14T09:04:00.000Z", "lf01", "First leaf", ""),
		addAt("2026-08-14T09:05:00.000Z", "lf02", "Second leaf", ""),
	}

	roots, _ := Build(events.Fold(lines))
	want := strings.Join([]string{"alfa", "  kida", "zulu", "  kidz", "lf02", "lf01"}, "\n")
	if got := shape(roots); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}

	touched := append(lines, `{"ts":"2026-08-14T09:06:00.000Z","ev":"note","id":"lf01","text":"touched"}`)
	roots, _ = Build(events.Fold(touched))
	want = strings.Join([]string{"alfa", "  kida", "zulu", "  kidz", "lf01", "lf02"}, "\n")
	if got := shape(roots); got != want {
		t.Errorf("after touching a leaf, shape =\n%s\nwant\n%s", got, want)
	}

	retitled := append(touched, `{"ts":"2026-08-14T09:07:00.000Z","ev":"update","id":"zulu","title":"Aardvark"}`)
	roots, _ = Build(events.Fold(retitled))
	want = strings.Join([]string{"zulu", "  kidz", "alfa", "  kida", "lf01", "lf02"}, "\n")
	if got := shape(roots); got != want {
		t.Errorf("after retitling a parent, shape =\n%s\nwant\n%s", got, want)
	}
}

// Titles that differ only in case sort together rather than splitting on byte order.
func TestBuildSortsParentTitlesCaseInsensitively(t *testing.T) {
	state := events.Fold([]string{
		addLine("bigb", "Beta", ""),
		addLine("kidb", "Child", "bigb"),
		addLine("lila", "alpha", ""),
		addLine("kida", "Child", "lila"),
	})

	roots, _ := Build(state)
	if got, want := shape(roots), "lila\n  kida\nbigb\n  kidb"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// openOnly drops the done items from a forest, standing in for the filter the views apply, so the
// bucket can be tested against a level whose children are all hidden.
func openOnly(nodes []Node) []Node {
	kept := []Node{}
	for _, node := range nodes {
		if node.Item.Status == events.StatusDone {
			continue
		}
		node.Children = openOnly(node.Children)
		kept = append(kept, node)
	}
	return kept
}

func TestBucketLeavesALevelOfOnlyLeavesAlone(t *testing.T) {
	state := events.Fold([]string{
		addLine("head", "Parent", ""),
		addLine("kid1", "First", "head"),
		addLine("kid2", "Second", "head"),
	})

	roots, _ := Build(state)
	if got, want := shape(Bucket(roots)), "head\n  kid1\n  kid2"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

func TestBucketGathersTheLeavesOfAMixedLevel(t *testing.T) {
	state := events.Fold([]string{
		addLine("head", "Parent", ""),
		addLine("subh", "Subproject", "head"),
		addLine("gkid", "Under the subproject", "subh"),
		addLine("loos", "Loose task", "head"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"head", "  subh", "    gkid", "  (unassigned)", "    loos"}, "\n")
	if got := shape(Bucket(roots)); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// The bucket is pinned after its siblings however their titles sort.
func TestBucketSortsLast(t *testing.T) {
	state := events.Fold([]string{
		addLine("zulu", "Zulu project", ""),
		addLine("kidz", "Child of Zulu", "zulu"),
		addLine("loos", "Loose task", ""),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"zulu", "  kidz", "(unassigned)", "  loos"}, "\n")
	if got := shape(Bucket(roots)); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// The top level is a level like any other: it buckets when it mixes and does not when it does not.
func TestBucketLeavesATopLevelOfOnlyLeavesAlone(t *testing.T) {
	state := events.Fold([]string{
		addLine("aaaa", "First", ""),
		addLine("bbbb", "Second", ""),
	})

	roots, _ := Build(state)
	if got, want := shape(Bucket(roots)), "aaaa\nbbbb"; got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

func TestBucketIsNeverAnItem(t *testing.T) {
	state := events.Fold([]string{
		addLine("head", "Parent", ""),
		addLine("kid1", "Child", "head"),
		addLine("loos", "Loose task", ""),
	})

	built, _ := Build(state)
	roots := Bucket(built)
	bucket := roots[len(roots)-1]
	if !bucket.Synthetic {
		t.Errorf("bucket.Synthetic = false, want true")
	}
	if bucket.Item.Id != "" {
		t.Errorf("bucket id = %q, want empty", bucket.Item.Id)
	}
	if bucket.Item.Title != UnassignedTitle {
		t.Errorf("bucket title = %q, want %q", bucket.Item.Title, UnassignedTitle)
	}
	if _, found := state.Find(bucket.Item.Id); found {
		t.Errorf("the bucket resolves to an item, want no item behind it")
	}
	for _, node := range roots[:len(roots)-1] {
		if node.Synthetic {
			t.Errorf("%s is marked synthetic, want only the bucket", node.Item.Id)
		}
	}
}

// A parent whose children are all done has no visible children, so it renders as a leaf and joins
// the bucket rather than standing as an empty heading.
func TestBucketTreatsAParentWithNoVisibleChildrenAsALeaf(t *testing.T) {
	state := events.Fold([]string{
		addLine("live", "Live project", ""),
		addLine("kidl", "Open child", "live"),
		addLine("spnt", "Spent project", ""),
		addLine("kids", "Closed child", "spnt"),
		`{"ts":"2026-08-14T09:01:00.000Z","ev":"update","id":"kids","status":"done"}`,
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"live", "  kidl", "(unassigned)", "  spnt"}, "\n")
	if got := shape(Bucket(openOnly(roots))); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}
