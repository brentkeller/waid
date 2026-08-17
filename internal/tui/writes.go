package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/events"
)

// The four writes Loops makes: close an item, mark it waiting on someone, correct its title, and note
// something against it. They follow the rule Scan's triage already does (§3) — the write lands on the
// keypress with no confirm step, and the footer's receipt is what says a press did anything.
//
// Each one edits the loaded copy of the item beside appending to the log, so the row moves in the
// frame the key was pressed in rather than waiting for the next read to catch up with it.

// doneSelected closes the item under the cursor, which is the only way out of the list (§1.1).
func (m Model) doneSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "x closes an item")
	if !ok {
		return m, nil, true
	}
	// The done and all segments show items that are already closed, and a second close would be a log
	// line and a receipt for nothing (§4).
	if item.Status == events.StatusDone {
		m.hint = item.Id + " is already closed"
		return m, nil, true
	}

	ts, err := events.Append(m.opts.Cfg.EventsPath, events.CloseEvent{Ev: "close", Id: item.Id}, m.now())
	if err != nil {
		m.hint = writeFailed("done", err)
		return m, nil, true
	}

	closed := item
	closed.Status, closed.Updated = events.StatusDone, ts

	m = m.applyItem(closed).pushUndo(undoDone(item))
	return m.record(receipt{verb: "closed", subject: item.Id, detail: item.Title}), nil, true
}

// waitingSelected asks who the item is waiting on. The name is what the status is for — an item
// waiting on nobody is just open — so it is collected before anything is written.
func (m Model) waitingSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "w marks an item waiting")
	if !ok {
		return m, nil, true
	}

	m.prompt = prompt{kind: promptWaiting, label: "waiting on", subject: item.Id}
	return m, nil, true
}

// waitOn writes the status and the name together, since an update carrying one without the other
// would leave the item describing half a state.
func (m Model) waitOn(id, who string) (Model, tea.Cmd) {
	item, loaded := m.loadedItem(id)
	if !loaded {
		m.hint = noItems
		return m, nil
	}

	event := events.UpdateEvent{Ev: "update", Id: id, Status: events.StatusWaiting, WaitingOn: &who}
	ts, err := events.Append(m.opts.Cfg.EventsPath, event, m.now())
	if err != nil {
		m.hint = writeFailed("waiting", err)
		return m, nil
	}

	waiting := item
	waiting.Status, waiting.WaitingOn, waiting.Updated = events.StatusWaiting, &who, ts

	m = m.applyItem(waiting).pushUndo(undoWaiting(item))
	return m.record(receipt{verb: "waiting", subject: id, detail: who}), nil
}

// editSelected corrects the title of the item under the cursor. Scan's `e` acts on the receipt of the
// promotion it just made, since the row it addresses has already left the list; here the row is on
// screen, so the cursor is what says which title is being corrected (§4).
func (m Model) editSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "e edits an item's title")
	if !ok {
		return m, nil, true
	}

	// The input opens empty rather than filled with the title being replaced: a correction is a new
	// title, and clearing a prefilled one costs a keypress per character.
	m.prompt = prompt{kind: promptRename, label: "edit", subject: item.Id, prior: item.Title}
	return m, nil, true
}

// noteSelected collects a note against the item under the cursor.
func (m Model) noteSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "n notes against an item")
	if !ok {
		return m, nil, true
	}

	m.prompt = prompt{kind: promptNote, label: "note", subject: item.Id}
	return m, nil, true
}

// addNote appends the note to the log and to the item the detail pane is drawing, so it is readable
// where it was written.
//
// A note leaves nothing on the undo stack: the log holds no event that removes one, and an undo that
// reversed the write before it would be worse than none at all. The footer stops offering `u` instead.
func (m Model) addNote(id, text string) (Model, tea.Cmd) {
	item, loaded := m.loadedItem(id)
	if !loaded {
		m.hint = noItems
		return m, nil
	}

	ts, err := events.Append(m.opts.Cfg.EventsPath, events.NoteEvent{Ev: "note", Id: id, Text: text}, m.now())
	if err != nil {
		m.hint = writeFailed("note", err)
		return m, nil
	}

	noted := item
	noted.Updated = ts
	// The append is capped to the notes already held so it allocates a fresh array: Model is copied by
	// value through the update loop, and two copies sharing one would overwrite each other's note.
	noted.Notes = append(item.Notes[:len(item.Notes):len(item.Notes)], events.Note{Ts: ts, Text: text})

	m.reversible = false
	m = m.applyItem(noted)
	return m.record(receipt{verb: "noted", subject: id, detail: text}), nil
}

// loopTarget is the item an action addresses. A row that is not one — a collapsed project fold — is
// inert, and the footer says which row it was pressed on rather than staying silent (§4).
func (m Model) loopTarget(tree Tree[events.Item], action string) (Model, events.Item, bool) {
	item, ok := tree.SelectedItem()
	if !ok {
		m.hint = inertOn(tree, noItems, action+" — this row is a project")
		return m, events.Item{}, false
	}
	return m, item, true
}

// loadedItem is the loaded copy of an item, which is the state a write acts on and the state its
// inverse puts back. An item the app has not read is not one it can write against.
func (m Model) loadedItem(id string) (events.Item, bool) {
	for _, item := range m.loops.items {
		if item.Id == id {
			return item, true
		}
	}
	return events.Item{}, false
}

// applyItem replaces the loaded copy of an item with what a write just made of it, moving the badge
// and pulling the cursor back into range in case the row left the list. The slice is rebuilt rather
// than written through: Model is copied by value through the update loop, and two copies sharing a
// backing array would see each other's edits.
func (m Model) applyItem(item events.Item) Model {
	items := make([]events.Item, len(m.loops.items))
	copy(items, m.loops.items)

	found := false
	for i := range items {
		if items[i].Id == item.Id {
			items[i], found = item, true
		}
	}
	if !found {
		return m
	}

	m.loops.items = items
	m.counts[tabLoops] = len(m.owed())

	// The row carries a fresh timestamp and sorts elsewhere for it, so the cursor follows the item the
	// write acted on rather than the place it was standing in. A row that left the list has nothing to
	// follow, and the clamp puts the cursor back in range.
	tree := m.loopsTree(m.viewWidth())
	if !tree.Focus(func(candidate events.Item) bool { return candidate.Id == item.Id }) {
		tree.clamp()
	}

	m.loops.cursor = tree.Cursor
	return m
}
