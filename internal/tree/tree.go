// Package tree builds the forest of items the Loops views render, and owns every question about the
// item hierarchy that needs the whole set at once. The fold is per-line and stores a parent id
// verbatim, so whether that id resolves — and whether the chain it starts loops — is answered here.
package tree

import (
	"sort"
	"strings"

	"github.com/brentkeller/waid/internal/events"
)

// Node is an item together with the items filed under it. A node whose Synthetic flag is set is a
// rendering artifact rather than a logged item: it holds no id and nothing can act on it.
type Node struct {
	Item      events.Item `json:"item"`
	Children  []Node      `json:"children"`
	Synthetic bool        `json:"synthetic,omitempty"`
}

// AnomalyKind is why an item could not sit where its parent id says it should.
type AnomalyKind string

const (
	// UnknownParent is an item whose parent id no item in the log holds.
	UnknownParent AnomalyKind = "unknown-parent"
	// ParentCycle is an item whose ancestor chain loops rather than reaching the top level.
	ParentCycle AnomalyKind = "parent-cycle"
)

// Anomaly is one item the build had to lift to the top level, and why. Reported by doctor; never
// fatal, since a corrupt log still has to render.
type Anomaly struct {
	Id   string      `json:"id"`
	Kind AnomalyKind `json:"kind"`
}

// Build assembles the forest from folded items, returning the roots and the anomalies it resolved
// on the way. Every level comes back in the order sortSiblings gives it.
//
// An item whose parent id is absent from the log, and an item whose ancestor chain loops, are both
// lifted to the top level and reported. Lifting the whole loop rather than picking an arbitrary
// place to cut it is what keeps the tree finite: every item caught in or hanging below a cycle
// becomes a root, so no edge of the loop is ever drawn.
func Build(state events.State) ([]Node, []Anomaly) {
	roots := []Node{}
	anomalies := []Anomaly{}
	lifted := map[string]bool{}

	for _, item := range state.Items {
		if item.Parent == nil {
			continue
		}
		if _, exists := state.Find(*item.Parent); !exists {
			lifted[item.Id] = true
			anomalies = append(anomalies, Anomaly{Id: item.Id, Kind: UnknownParent})
			continue
		}
		if chainLoops(state, item) {
			lifted[item.Id] = true
			anomalies = append(anomalies, Anomaly{Id: item.Id, Kind: ParentCycle})
		}
	}

	for _, item := range state.Items {
		if item.Parent != nil && !lifted[item.Id] {
			continue
		}
		roots = append(roots, node(state, item, lifted))
	}
	sortSiblings(roots)
	return roots, anomalies
}

// node assembles one item and everything below it, skipping the children that were lifted to the
// top level and are therefore drawn there instead.
func node(state events.State, item events.Item, lifted map[string]bool) Node {
	built := Node{Item: item, Children: []Node{}}
	for _, child := range state.Children(item.Id) {
		if lifted[child.Id] {
			continue
		}
		built.Children = append(built.Children, node(state, child, lifted))
	}
	sortSiblings(built.Children)
	return built
}

// heads reports whether a node heads a level: it holds children, or it was declared a heading. A
// declared heading heads its level whether or not anything is filed under it.
func heads(node Node) bool { return len(node.Children) > 0 || node.Item.Heading }

// sortSiblings orders one level in place: the nodes that head it first by title, then the rest
// most-recently-updated first. Headings are landmarks, so a head sorts by the one field a rename
// changes rather than drifting every time a row beneath it is touched. Ties keep log order.
func sortSiblings(nodes []Node) {
	sort.SliceStable(nodes, func(left, right int) bool {
		a, b := nodes[left], nodes[right]
		isHead := heads(a)
		if isHead != heads(b) {
			return isHead
		}
		if isHead {
			return compareTitles(a.Item.Title, b.Item.Title) < 0
		}
		return a.Item.Updated > b.Item.Updated
	})
}

// compareTitles orders titles case-insensitively, falling back to byte order so titles differing
// only in case still order stably against each other.
func compareTitles(left, right string) int {
	if folded := strings.Compare(strings.ToLower(left), strings.ToLower(right)); folded != 0 {
		return folded
	}
	return strings.Compare(left, right)
}

// chainLoops reports whether walking up from item revisits an id instead of reaching the top level
// or a parent the log does not hold.
func chainLoops(state events.State, item events.Item) bool {
	seen := map[string]bool{item.Id: true}
	for current := item; current.Parent != nil; {
		if seen[*current.Parent] {
			return true
		}
		seen[*current.Parent] = true

		parent, found := state.Find(*current.Parent)
		if !found {
			return false
		}
		current = parent
	}
	return false
}

// UnassignedTitle is the title the synthetic bucket carries.
const UnassignedTitle = "(unassigned)"

// Bucket gathers the leaves of every level that also holds parents into one synthetic
// (unassigned) node, pinned after its siblings. A level whose children are all leaves keeps them
// where they are, since a bucket holding an entire level says nothing about it.
//
// Leaf-ness is read off the nodes given rather than off the log, so a parent filtered down to no
// children renders as a leaf and joins the bucket alongside the others. Call this after any
// filtering, so the bucket follows whatever is in force. An item declared a heading is never
// gathered, empty or not: the inbox is where unfiled work waits, and a shelf is not work.
func Bucket(nodes []Node) []Node {
	parents, leaves := []Node{}, []Node{}
	for _, current := range nodes {
		if !heads(current) {
			leaves = append(leaves, current)
			continue
		}
		current.Children = Bucket(current.Children)
		parents = append(parents, current)
	}
	switch {
	case len(parents) == 0:
		return nodes
	case len(leaves) == 0:
		return parents
	}
	return append(parents, Node{
		Item:      events.Item{Title: UnassignedTitle},
		Children:  leaves,
		Synthetic: true,
	})
}

// BucketBelow buckets every level under the roots, leaving the top level as it stands. The move
// picker draws the tree that way: a root with nothing under it is where an item goes to sit beside
// the other roots, and the bucket is a rendering artifact nothing can be filed into.
func BucketBelow(nodes []Node) []Node {
	bucketed := make([]Node, 0, len(nodes))
	for _, current := range nodes {
		current.Children = Bucket(current.Children)
		bucketed = append(bucketed, current)
	}
	return bucketed
}

// Filter narrows a forest to the nodes matching keep, together with the ancestors that lead to
// them. A match keeps its whole subtree, since filtering on a heading's name asks for what is under
// it; a node that does not match is kept only as the path to a descendant that did, and so brings
// none of its other children with it. A branch holding no match at all is dropped.
//
// The forest given is left untouched, so several filters can be rendered off one Build.
func Filter(nodes []Node, keep func(events.Item) bool) []Node {
	kept := []Node{}
	for _, current := range nodes {
		if keep(current.Item) {
			kept = append(kept, current)
			continue
		}
		children := Filter(current.Children, keep)
		if len(children) == 0 {
			continue
		}
		current.Children = children
		kept = append(kept, current)
	}
	sortSiblings(kept)
	return kept
}

// Prune narrows a forest to the nodes matching keep, together with the ancestors that lead to them.
// It differs from Filter in that keep is asked of every node: a match does not bring its subtree
// along, so what survives is exactly the nodes that matched plus the paths down to them.
//
// That is the difference between a query and a gate. Filtering on a heading asks for what is under
// it, so the subtree comes too; gating on status asks the same question of every row in its own
// right, and a done row under an open parent is still done. A parent a gate leaves with no visible
// children renders as a leaf, which is the leaf-ness Bucket reads.
//
// The forest given is left untouched.
func Prune(nodes []Node, keep func(events.Item) bool) []Node {
	kept := []Node{}
	for _, current := range nodes {
		children := Prune(current.Children, keep)
		if !keep(current.Item) && len(children) == 0 {
			continue
		}
		current.Children = children
		kept = append(kept, current)
	}
	sortSiblings(kept)
	return kept
}

// Subtree narrows a forest to the one node carrying id, together with everything beneath it. The
// ancestors above it and the branches beside it are dropped: naming a heading asks what it holds,
// not where it sits. A forest holding no such node comes back empty.
func Subtree(nodes []Node, id string) []Node {
	for _, current := range nodes {
		if current.Item.Id == id {
			return []Node{current}
		}
		if found := Subtree(current.Children, id); len(found) > 0 {
			return found
		}
	}
	return []Node{}
}
