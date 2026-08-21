package events

// stateIndex answers the questions that need every item at once. It is derived from State.Items and
// built once per fold, so the queries the tree leans on — cycle checks, close guards, ancestor walks
// — cost a map lookup rather than a scan of the whole log.
type stateIndex struct {
	// byId locates an item in Items by its id.
	byId map[string]int
	// children holds each parent's direct children as positions in Items, in log order.
	children map[string][]int
}

// buildIndex indexes items by id and by parent. A parent id nothing holds is not recorded, so the
// children of a missing parent are simply absent rather than a bucket of orphans.
func buildIndex(items []Item) *stateIndex {
	index := &stateIndex{
		byId:     make(map[string]int, len(items)),
		children: map[string][]int{},
	}
	for position, item := range items {
		index.byId[item.Id] = position
	}
	for position, item := range items {
		if item.Parent == nil {
			continue
		}
		if _, exists := index.byId[*item.Parent]; !exists {
			continue
		}
		index.children[*item.Parent] = append(index.children[*item.Parent], position)
	}
	return index
}

// indexed returns the state's index, building one for a State that carries none. Fold sets it, so
// the fallback covers a State assembled by hand or decoded from JSON, where the derived field
// cannot survive.
func (s State) indexed() *stateIndex {
	if s.index != nil {
		return s.index
	}
	return buildIndex(s.Items)
}

// Find returns the item carrying id, and whether the log holds one.
func (s State) Find(id string) (Item, bool) {
	position, found := s.indexed().byId[id]
	if !found {
		return Item{}, false
	}
	return s.Items[position], true
}

// Children returns the items whose parent is id, in log order. An id nothing holds, and an item
// nothing points at, both have none.
func (s State) Children(id string) []Item {
	positions := s.indexed().children[id]
	children := make([]Item, 0, len(positions))
	for _, position := range positions {
		children = append(children, s.Items[position])
	}
	return children
}

// Ancestors returns the chain above id, nearest parent first, up to the top level. The walk ends at
// a parent the log does not hold and at an id it has already visited, so a log holding a loop
// answers rather than spinning — internal/tree reports the loop itself.
func (s State) Ancestors(id string) []Item {
	index := s.indexed()
	ancestors := []Item{}
	seen := map[string]bool{id: true}

	position, found := index.byId[id]
	for found {
		parent := s.Items[position].Parent
		if parent == nil || seen[*parent] {
			break
		}
		seen[*parent] = true
		position, found = index.byId[*parent]
		if !found {
			break
		}
		ancestors = append(ancestors, s.Items[position])
	}
	return ancestors
}
