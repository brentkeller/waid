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

// BucketBelow buckets what Bucket does one level down: the roots keep their loose leaves, and every
// level under them gathers its own.
func TestBucketBelowLeavesTheRootsAsTheyAre(t *testing.T) {
	state := events.Fold([]string{
		addLine("head", "Parent", ""),
		addLine("subh", "Subproject", "head"),
		addLine("gkid", "Under the subproject", "subh"),
		addLine("kid1", "Loose under the parent", "head"),
		addLine("loos", "Loose root", ""),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"head", "  subh", "    gkid", "  (unassigned)", "    kid1", "loos"}, "\n")
	if got := shape(BucketBelow(roots)); got != want {
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

// headingAt writes an add line for an item declared a heading, so a fixture can hold a shelf that
// nothing is filed under yet.
func headingAt(ts string, id string, title string, parent string) string {
	return strings.TrimSuffix(addAt(ts, id, title, parent), "}") + `,"heading":true}`
}

// headingLine is headingAt at the timestamp addLine uses.
func headingLine(id string, title string, parent string) string {
	return headingAt("2026-08-14T09:00:00.000Z", id, title, parent)
}

// A declared heading heads its level whether or not anything is filed under it, so it stands beside
// the parents rather than being swept into the inbox with the loose work.
func TestBucketKeepsAnEmptyHeadingOutOfTheBucket(t *testing.T) {
	state := events.Fold([]string{
		addLine("live", "Zulu project", ""),
		addLine("kidl", "Open child", "live"),
		headingLine("bare", "Alpha project", ""),
		addLine("loos", "Loose task", ""),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"bare", "live", "  kidl", "(unassigned)", "  loos"}, "\n")
	if got := shape(Bucket(roots)); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// An empty heading sorts by title with the other heads rather than drifting through the leaves by
// age, which is what keeping a landmark still asks for.
func TestBuildSortsAnEmptyHeadingWithTheParentsByTitle(t *testing.T) {
	state := events.Fold([]string{
		headingAt("2026-08-14T09:00:00.000Z", "zulu", "Zulu", ""),
		headingAt("2026-08-14T09:01:00.000Z", "alfa", "alpha", ""),
		addAt("2026-08-14T09:02:00.000Z", "midl", "Middle", ""),
		addAt("2026-08-14T09:03:00.000Z", "kidm", "Child of Middle", "midl"),
		addAt("2026-08-14T09:04:00.000Z", "leaf", "Newest leaf", ""),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"alfa", "midl", "  kidm", "zulu", "leaf"}, "\n")
	if got := shape(roots); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// Only a declared heading is spared: an ordinary item with no children lands in the inbox exactly
// as it did before the flag existed.
func TestBucketStillGathersAnUnmarkedChildlessItem(t *testing.T) {
	state := events.Fold([]string{
		headingLine("head", "Project", ""),
		addLine("kid1", "Child", "head"),
		`{"ts":"2026-08-14T09:00:00.000Z","ev":"add","id":"loos","title":"Loose task","heading":false}`,
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"head", "  kid1", "(unassigned)", "  loos"}, "\n")
	if got := shape(Bucket(roots)); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// A level of nothing but leaves keeps them where they are, heading above it or not: a bucket
// holding an entire level says nothing about it.
func TestBucketLeavesALevelOfOnlyLeavesUnderAHeadingAlone(t *testing.T) {
	state := events.Fold([]string{
		headingLine("head", "Project", ""),
		addLine("kid1", "First", "head"),
		addLine("kid2", "Second", "head"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"head", "  kid1", "  kid2"}, "\n")
	if got := shape(Bucket(roots)); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// titled matches items whose title contains fragment, standing in for the query a view filters on.
func titled(fragment string) func(events.Item) bool {
	return func(item events.Item) bool {
		return strings.Contains(strings.ToLower(item.Title), strings.ToLower(fragment))
	}
}

func TestFilterKeepsTheAncestorChainOfADeepMatch(t *testing.T) {
	state := events.Fold([]string{
		addLine("area", "Area", ""),
		addLine("proj", "Project", "area"),
		addLine("task", "Needle", "proj"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"area", "  proj", "    task"}, "\n")
	if got := shape(Filter(roots, titled("needle"))); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

func TestFilterDropsABranchHoldingNoMatch(t *testing.T) {
	state := events.Fold([]string{
		addLine("keep", "Keeper", ""),
		addLine("kkid", "Needle", "keep"),
		addLine("drop", "Other", ""),
		addLine("dkid", "Nothing here", "drop"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"keep", "  kkid"}, "\n")
	if got := shape(Filter(roots, titled("needle"))); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

func TestFilterKeepsTheWholeSubtreeOfAMatchingParent(t *testing.T) {
	state := events.Fold([]string{
		addLine("proj", "Needle", ""),
		addLine("kid1", "First", "proj"),
		addLine("kid2", "Second", "proj"),
		addLine("gkid", "Deeper", "kid1"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"proj", "  kid1", "    gkid", "  kid2"}, "\n")
	if got := shape(Filter(roots, titled("needle"))); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// An ancestor kept only as context brings nothing with it but the path to the match.
func TestFilterKeepsAContextAncestorWithoutItsOtherChildren(t *testing.T) {
	state := events.Fold([]string{
		addLine("area", "Area", ""),
		addLine("hit_", "Needle", "area"),
		addLine("miss", "Sibling", "area"),
		addLine("mkid", "Sibling child", "miss"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"area", "  hit_"}, "\n")
	if got := shape(Filter(roots, titled("needle"))); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

func TestFilterMatchingNothingYieldsAnEmptyForest(t *testing.T) {
	state := events.Fold([]string{
		addLine("area", "Area", ""),
		addLine("task", "Task", "area"),
	})

	roots, _ := Build(state)
	if got := Filter(roots, titled("needle")); len(got) != 0 {
		t.Errorf("shape =\n%s\nwant nothing", shape(got))
	}
}

// Filtering leaves the forest it was given untouched, so a view can render several filters off one
// build.
func TestFilterDoesNotDisturbTheForestGiven(t *testing.T) {
	state := events.Fold([]string{
		addLine("area", "Area", ""),
		addLine("hit_", "Needle", "area"),
		addLine("miss", "Sibling", "area"),
	})

	roots, _ := Build(state)
	before := shape(roots)
	Filter(roots, titled("needle"))
	if after := shape(roots); after != before {
		t.Errorf("shape after filtering =\n%s\nwant\n%s", after, before)
	}
}

func TestPruneAsksEveryNodeRatherThanKeepingASubtree(t *testing.T) {
	state := events.Fold([]string{
		addLine("proj", "Needle", ""),
		addLine("kid1", "First", "proj"),
		addLine("kid2", "Needle too", "proj"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"proj", "  kid2"}, "\n")
	if got := shape(Prune(roots, titled("needle"))); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

func TestPruneKeepsAnAncestorAsContextForADeepMatch(t *testing.T) {
	state := events.Fold([]string{
		addLine("area", "Area", ""),
		addLine("proj", "Project", "area"),
		addLine("task", "Needle", "proj"),
		addLine("drop", "Other", ""),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"area", "  proj", "    task"}, "\n")
	if got := shape(Prune(roots, titled("needle"))); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// A parent a gate strips of children is a leaf from then on, so it re-sorts among the leaves rather
// than staying with the headings it no longer resembles.
func TestPruneReordersAParentItLeavesChildless(t *testing.T) {
	state := events.Fold([]string{
		addLine("aaaa", "Needle area", ""),
		addLine("kid1", "Nothing here", "aaaa"),
		addLine("zzzz", "Needle other", ""),
		addLine("kid2", "Needle child", "zzzz"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"zzzz", "  kid2", "aaaa"}, "\n")
	if got := shape(Prune(roots, titled("needle"))); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

func TestPruneDoesNotDisturbTheForestGiven(t *testing.T) {
	state := events.Fold([]string{
		addLine("area", "Area", ""),
		addLine("hit_", "Needle", "area"),
		addLine("miss", "Sibling", "area"),
	})

	roots, _ := Build(state)
	before := shape(roots)
	Prune(roots, titled("needle"))
	if after := shape(roots); after != before {
		t.Errorf("shape after pruning =\n%s\nwant\n%s", after, before)
	}
}

func TestSubtreeYieldsTheNodeAndEverythingUnderIt(t *testing.T) {
	state := events.Fold([]string{
		addLine("area", "Area", ""),
		addLine("proj", "Project", "area"),
		addLine("task", "Task", "proj"),
	})

	roots, _ := Build(state)
	want := strings.Join([]string{"proj", "  task"}, "\n")
	if got := shape(Subtree(roots, "proj")); got != want {
		t.Errorf("shape =\n%s\nwant\n%s", got, want)
	}
}

// Naming a heading asks for what it holds, so the branches beside it and the ancestors above it go.
func TestSubtreeDropsAncestorsAndSiblings(t *testing.T) {
	state := events.Fold([]string{
		addLine("area", "Area", ""),
		addLine("proj", "Project", "area"),
		addLine("othr", "Other project", "area"),
		addLine("else", "Elsewhere", ""),
	})

	roots, _ := Build(state)
	if got := shape(Subtree(roots, "proj")); got != "proj" {
		t.Errorf("shape =\n%s\nwant\nproj", got)
	}
}

func TestSubtreeOfAnAbsentIdIsEmpty(t *testing.T) {
	state := events.Fold([]string{addLine("area", "Area", "")})

	roots, _ := Build(state)
	if found := Subtree(roots, "nope"); len(found) != 0 {
		t.Errorf("subtree = %s, want none", shape(found))
	}
}
