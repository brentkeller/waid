package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/sessions"
	// The Loops rows are a Tree[T] the writes take as a parameter named tree, so the package that
	// answers questions about the item hierarchy is aliased out of that name's way.
	itemtree "github.com/brentkeller/waid/internal/tree"
)

// The writes made against items: the four Loops binds to the row under the cursor — close an item,
// mark it waiting on someone, correct its title, note something against it — and the add every tab
// offers. They follow the rule Repos' triage already does (§3) — the write lands on the keypress with
// no confirm step, and the footer's receipt is what says a press did anything.
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
	// A heading closes only once the work beneath it is finished (§5). The guard is the one `waid
	// done` calls, so the two surfaces cannot disagree about when that is.
	if err := itemtree.GuardClose(events.State{Items: m.loops.items}, item.Id); err != nil {
		m.hint = err.Error()
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

// headingSelected marks the item under the cursor a heading, or takes the mark off one that already
// holds it (§6). The event is written whatever the item held, which is how `waid heading` writes it:
// the log is a history of what was asked for, the fold is idempotent, and a key that sometimes wrote
// and sometimes did not would make `u` guess.
func (m Model) headingSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "P marks an item a heading")
	if !ok {
		return m, nil, true
	}

	heading := !item.Heading
	event := events.HeadingEvent{Ev: "update", Id: item.Id, Heading: heading}
	ts, err := events.Append(m.opts.Cfg.EventsPath, event, m.now())
	if err != nil {
		m.hint = writeFailed("heading", err)
		return m, nil, true
	}

	marked := item
	marked.Heading, marked.Updated = heading, ts

	m = m.applyItem(marked).pushUndo(undoHeading(item))
	return m.record(headingReceipt(marked)), nil, true
}

// headingReceipt is what the toggle says it did, in the wording `waid heading` prints for the same
// write. It is read off the item as it stands afterwards, so the inverse can share it.
func headingReceipt(item events.Item) receipt {
	verb := "unmarked"
	if item.Heading {
		verb = "marked"
	}
	return receipt{verb: verb, subject: item.Id, detail: item.Title}
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

// editSelected corrects the title of the item under the cursor. Repos' `e` acts on the receipt of the
// promotion it just made, since the row it addresses has already left the list; here the row is on
// screen, so the cursor is what says which title is being corrected (§4).
func (m Model) editSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "e edits an item's title")
	if !ok {
		return m, nil, true
	}

	// The input opens on the title being corrected, since a correction is usually a word of a title
	// rather than a fresh one. ctrl-u empties it for the times it is a fresh one.
	m.prompt = prompt{kind: promptRename, label: "edit", subject: item.Id, prior: item.Title, value: item.Title}
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

// addTarget is where a declared item lands, and the two halves of the app answer that differently:
// Loops files into the item tree and yields a parent id, while Repos and Agents are keyed by path
// and yield an origin. Never both — the rule `-p` follows on the command line (§6).
//
// label is where the footer says the item is going: the parent's title or the project's directory,
// rather than the id or the whole path.
type addTarget struct {
	parent *string
	origin *string
	label  string
}

// addPrompt opens the input a declared item is titled in. `a` is global rather than a Loops key (§4):
// an item is declared as often while reading a signal or a session as while reading the list it lands
// in, and where it belongs is what the row the cursor is on already says.
//
// child is `A` rather than `a`: the item lands under the row instead of beside it, which is how a
// tier is created (§7.2). Repos and Agents file by path, where a row holds no tier to add under, so
// the two keys are one press there.
func (m Model) addPrompt(child bool) (Model, tea.Cmd) {
	target := m.addTarget(child)
	m.prompt = prompt{kind: promptAdd, label: "add", subject: target.label, target: target}
	return m, nil
}

// addItem declares the item the prompt was answered with, writing the same event `waid add` writes
// for it. The id is drawn against the whole log rather than the loaded list, since an id another
// session declared is taken whether or not this one has read it.
func (m Model) addItem(target addTarget, title string) (Model, tea.Cmd) {
	id, err := events.Load(m.opts.Cfg.EventsPath).NewId(m.opts.Ids)
	if err != nil {
		m.hint = writeFailed("add", err)
		return m, nil
	}

	// The log holds an empty array rather than null when no tags were given, which is what the command
	// writes for an item declared with none.
	event := events.AddEvent{
		Ev: "add", Id: id, Title: title, Status: events.StatusOpen,
		Parent: target.parent, Origin: target.origin, Session: nil, Tags: []string{}, WaitingOn: nil,
	}
	ts, err := events.Append(m.opts.Cfg.EventsPath, event, m.now())
	if err != nil {
		m.hint = writeFailed("add", err)
		return m, nil
	}

	item := events.Item{
		Id: id, Title: title, Status: events.StatusOpen, Parent: target.parent, Origin: target.origin,
		Tags: []string{}, Notes: []events.Note{}, Created: ts, Updated: ts,
	}

	m = m.insertItem(item).pushUndo(undoAdd(item))
	return m.record(receipt{verb: "added", subject: id, detail: title}), nil
}

// addTarget is where the row under the cursor sends a declared item. On Loops that is a place in the
// item tree: the row's own id under `A`, and the id of whatever it hangs beneath otherwise, since a
// sibling is a child of the same parent. The `(unassigned)` bucket is not an item and so is not a
// parent either — an add against it lands under the fold whose leaves it gathers.
//
// On Repos and Agents the row is a project, and a project is a path: the item records it as its
// origin and lands at the top level. A row belonging to none — a signal against a repository with no
// checkout, a session whose transcript recorded no cwd — records none either.
func (m Model) addTarget(child bool) addTarget {
	width := m.viewWidth()
	switch m.tab {
	case tabLoops:
		tree := m.loopsTree(width)
		if child {
			if item, ok := tree.SelectedItem(); ok {
				return parentTarget(item)
			}
		}
		if item, ok := tree.SelectedParent(); ok {
			return parentTarget(item)
		}
		return addTarget{label: topLevel}
	case tabScan:
		return originTarget(rowProject(m.scanTree(width), func(signal detect.Signal) *string { return signal.Project }))
	case tabReview:
		return originTarget(rowProject(m.reviewTree(width), func(s sessions.Session) *string { return s.Project }))
	}
	return addTarget{label: topLevel}
}

// parentTarget files the item under an item, which is what Loops means by a place.
func parentTarget(item events.Item) addTarget {
	return addTarget{parent: &item.Id, label: item.Title}
}

// originTarget records a path as provenance and leaves the item at the top level, which is where a
// promotion lands too (§6).
func originTarget(path *string) addTarget {
	return addTarget{origin: path, label: projectLabel(path)}
}

// rowProject reads the project off the row under the cursor, falling back to the group the cursor is
// in when the row is a fold: a project collapsed to one line is still a project to file against, and
// every row folded under it belongs to it.
func rowProject[T any](tree Tree[T], of func(T) *string) *string {
	if item, ok := tree.SelectedItem(); ok {
		return of(item)
	}
	if group, ok := tree.SelectedGroup(); ok && len(group.Items) > 0 {
		return of(group.Items[0])
	}
	return nil
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

// applyItem replaces the loaded copy of an item with what a write just made of it. The slice is
// rebuilt rather than written through: Model is copied by value through the update loop, and two
// copies sharing a backing array would see each other's edits.
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
	return m.setItems(items, item.Id)
}

// insertItem puts a freshly declared item into the loaded list, so it is on the tab in the frame the
// add was made in rather than waiting for the next read to bring it in. The append is capped to the
// list's own length so it allocates a fresh array, for the reason applyItem rebuilds one.
func (m Model) insertItem(item events.Item) Model {
	return m.setItems(append(m.loops.items[:len(m.loops.items):len(m.loops.items)], item), item.Id)
}

// setItems takes the list a write left behind, moving the badge with it and leaving the cursor on the
// item the write addressed.
func (m Model) setItems(items []events.Item, focus string) Model {
	m.loops.items = items
	m.counts[tabLoops] = len(m.owed())

	// The row carries a fresh timestamp and sorts elsewhere for it, so the cursor follows the item the
	// write acted on rather than the place it was standing in. A row that left the list has nothing to
	// follow, and the clamp puts the cursor back in range.
	tree := m.loopsTree(m.viewWidth())
	if !tree.Focus(func(candidate events.Item) bool { return candidate.Id == focus }) {
		tree.clamp()
	}

	m.loops.cursor = tree.Cursor
	return m
}
