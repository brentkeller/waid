package tui

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/events"
)

// The write a move commits with. The picker that chooses the destination lives in move.go; this is
// the half that appends, so the two are separable — the parent write landed before the tree it is
// navigated in existed, and refiling never stopped working while that was being built (§7.1).

// topLevel is what an item with no parent is called wherever one is named: the receipt, the undo
// that puts it back, and the footer of the add prompt.
const topLevel = "(top level)"

// refile writes the move in the frame the key was pressed in, for one item or for several dropped
// together. A destination that is the parent an item already sits under is not a write for that item
// — the log would hold a move that moved nothing (§6) — so it is skipped, and a drop that moves
// nothing at all writes nothing.
func (m Model) refile(ids []string, parent *string) (Model, tea.Cmd) {
	label := m.parentLabel(parent)

	var writes []itemWrite
	for _, id := range ids {
		item, loaded := m.loadedItem(id)
		if !loaded || sameRef(parent, item.Parent) {
			continue
		}

		moved := item
		moved.Parent = parent
		writes = append(writes, itemWrite{
			event:   events.ParentEvent{Ev: "update", Id: id, Parent: parent},
			after:   moved,
			receipt: receipt{verb: verbFiled, subject: id, detail: label},
			inverse: undoRefile(item, m.parentLabel(item.Parent)),
		})
	}
	if len(writes) == 0 {
		return m, nil
	}

	return m.commit("parent", writes, receipt{verb: verbFiled, subject: plural(len(writes), "item"), detail: label})
}

// descendants are the ids hanging beneath an item, however deep. The walk is bounded by the loaded
// set rather than by depth, so a log that already holds a cycle is traversed once and not forever.
func (m Model) descendants(id string) []string {
	found := []string{}
	for frontier := []string{id}; len(frontier) > 0; {
		parent := frontier[0]
		frontier = frontier[1:]
		for _, item := range m.loops.items {
			if item.Parent == nil || *item.Parent != parent {
				continue
			}
			if item.Id == id || slices.Contains(found, item.Id) {
				continue
			}
			found = append(found, item.Id)
			frontier = append(frontier, item.Id)
		}
	}
	return found
}

// parentLabel names a parent the way a line of prose has room for: the item's title, or the top
// level for an item under nothing. An id the loaded set does not know is drawn as itself, since a
// title is all that is missing.
func (m Model) parentLabel(id *string) string {
	if id == nil {
		return topLevel
	}
	if parent, found := m.loadedItem(*id); found {
		return parent.Title
	}
	return *id
}

// sameRef reports whether two optional ids are the same one, counting the top level as a place two
// items can share.
func sameRef(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// sharedParent is the parent every item of a list sits under, or nil when they sit under different
// ones — which is also what the top level is, and the same row either way in the picker.
func sharedParent(items []events.Item) *string {
	parent := items[0].Parent
	for _, item := range items[1:] {
		if !sameRef(item.Parent, parent) {
			return nil
		}
	}
	return parent
}
