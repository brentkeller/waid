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

// refile writes the move in the frame the key was pressed in. A destination that is the parent the
// item already sits under is not a write — the log would hold a move that moved nothing (§6).
func (m Model) refile(id string, parent *string) (Model, tea.Cmd) {
	item, loaded := m.loadedItem(id)
	if !loaded {
		m.hint = noItems
		return m, nil
	}
	if sameRef(parent, item.Parent) {
		return m, nil
	}

	ts, err := events.Append(m.opts.Cfg.EventsPath, events.ParentEvent{Ev: "update", Id: id, Parent: parent}, m.now())
	if err != nil {
		m.hint = writeFailed("parent", err)
		return m, nil
	}

	moved := item
	moved.Parent, moved.Updated = parent, ts

	m = m.applyItem(moved).pushUndo(undoRefile(item, m.parentLabel(item.Parent)))
	return m.record(receipt{verb: verbFiled, subject: id, detail: m.parentLabel(parent)}), nil
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
