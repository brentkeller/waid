// Package tree builds the forest of items the Loops views render, and owns every question about the
// item hierarchy that needs the whole set at once. The fold is per-line and stores a parent id
// verbatim, so whether that id resolves — and whether the chain it starts loops — is answered here.
package tree

import "github.com/brentkeller/waid/internal/events"

// Node is an item together with the items filed under it.
type Node struct {
	Item     events.Item `json:"item"`
	Children []Node      `json:"children"`
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
// on the way. Roots and siblings come back in log order.
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
	return built
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
