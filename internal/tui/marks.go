package tui

import (
	"maps"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/events"
)

// A mark says which before a write says what: rows are marked one at a time, and a move, a close, a
// waiting or a tag made while any are marked is made against all of them. Marks are held by item id
// rather than by row, so they survive everything that re-sorts or redraws the list — a reload, a
// fold, a query, the status row.

// markKeys are the footer's hints while anything is marked: the four writes that take the set, and
// the two keys that change it.
const markKeys = "m move · d done · w waiting · t tag · x mark · X clear"

// toggleMark sets or clears the mark on the row under the cursor and steps down a row, so a run of
// rows is marked by repeating the key. A row holding no item takes no mark, and the footer says so.
//
// The set is replaced rather than written through: Model is copied by value through the update loop,
// and two copies sharing one map would see each other's marks.
func (m Model) toggleMark(t Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(t, "x marks an item")
	if !ok {
		return m, nil, true
	}

	marked := make(map[string]bool, len(m.loops.marked)+1)
	maps.Copy(marked, m.loops.marked)
	if marked[item.Id] {
		delete(marked, item.Id)
	} else {
		marked[item.Id] = true
	}
	m.loops.marked = marked

	t.Down()
	m.loops.cursor = t.Cursor
	return m, nil, true
}

// markedItems are the loaded items carrying a mark, in the order the log holds them — which is the
// order a write against the set appends in, whatever order the marks were made in.
func (m Model) markedItems() []events.Item {
	if len(m.loops.marked) == 0 {
		return nil
	}

	var items []events.Item
	for _, item := range m.loops.items {
		if m.loops.marked[item.Id] {
			items = append(items, item)
		}
	}
	return items
}

// keptMarks are the marks whose items a fresh read still holds.
func (m Model) keptMarks(items []events.Item) map[string]bool {
	if len(m.loops.marked) == 0 {
		return nil
	}

	kept := map[string]bool{}
	for _, item := range items {
		if m.loops.marked[item.Id] {
			kept[item.Id] = true
		}
	}
	return kept
}

// targets are the items a write addresses: every marked item, or the row under the cursor when
// nothing is marked. A marked item the query or the status row is hiding is a target all the same —
// the header's count is what keeps it in view.
func (m Model) targets(t Tree[events.Item], action string) (Model, []events.Item, bool) {
	if marked := m.markedItems(); len(marked) > 0 {
		return m, marked, true
	}

	m, item, ok := m.loopTarget(t, action)
	if !ok {
		return m, nil, false
	}
	return m, []events.Item{item}, true
}

// topmost drops every item that sits beneath another item of the set. A move carries a subtree with
// its root, so an item written a parent of its own as well would be pulled out from under the
// ancestor that was already taking it along.
func (m Model) topmost(items []events.Item) []events.Item {
	chosen := make(map[string]bool, len(items))
	for _, item := range items {
		chosen[item.Id] = true
	}

	state := events.State{Items: m.loops.items}
	var kept []events.Item
	for _, item := range items {
		carried := false
		for _, ancestor := range state.Ancestors(item.Id) {
			if chosen[ancestor.Id] {
				carried = true
				break
			}
		}
		if !carried {
			kept = append(kept, item)
		}
	}
	return kept
}

// idsOf are the ids of a list of items, in its order.
func idsOf(items []events.Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Id)
	}
	return ids
}
