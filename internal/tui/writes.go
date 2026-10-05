package tui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/sessions"
	// The Loops rows are a Tree[T] the writes take as a parameter named tree, so the package that
	// answers questions about the item hierarchy is aliased out of that name's way.
	itemtree "github.com/brentkeller/waid/internal/tree"
)

// The writes made against items: the ones Loops binds to the row under the cursor or to the rows carrying a mark — close an item,
// mark it waiting on someone, correct its title, note something against it — and the add every tab
// offers. They follow the rule Repos' triage already does (§3) — the write lands on the keypress with
// no confirm step, and the footer's receipt is what says a press did anything.
//
// Each one edits the loaded copy of the item beside appending to the log, so the row moves in the
// frame the key was pressed in rather than waiting for the next read to catch up with it.

// itemWrite is one item's share of a write: the event appended for it, the item as the write leaves
// it, the receipt it is recorded by, and the entry that reverses it.
type itemWrite struct {
	event   events.WaidEvent
	after   events.Item
	receipt receipt
	inverse undoEntry
}

// commit lands a write against one item or several. Each item's event is appended in turn, its
// loaded copy replaced and its receipt recorded, and the inverses are pushed as a single entry so one
// undo reverses the whole of it. A write against several also records summary, which is what the
// footer shows in place of the last item's own receipt.
//
// An append that fails stops the write there. What landed is kept and can be undone, the hint names
// the failure, and the marks are left standing so the rest can be tried again.
func (m Model) commit(action string, writes []itemWrite, summary receipt) (Model, tea.Cmd) {
	var inverses []undoEntry
	failed := ""
	for _, write := range writes {
		ts, err := events.Append(m.opts.Cfg.EventsPath, write.event, m.now())
		if err != nil {
			failed = writeFailed(action, err)
			break
		}

		landed := write.after
		landed.Updated = ts
		m = m.applyItem(landed).record(write.receipt)
		inverses = append(inverses, write.inverse)
	}

	if len(inverses) > 0 {
		m = m.pushUndo(mergeUndo(inverses))
	}
	if failed != "" {
		m.hint = failed
		return m, nil
	}

	if len(writes) > 1 {
		summary.summary = true
		m = m.record(summary)
	}
	m.loops.marked = nil
	return m, nil
}

// doneSelected closes the marked items, or the item under the cursor when nothing is marked — which
// is the only way out of the list (§1.1).
func (m Model) doneSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, items, ok := m.targets(tree, "d closes an item")
	if !ok {
		return m, nil, true
	}

	// The done and all segments show items that are already closed, and a second close would be a log
	// line and a receipt for nothing (§4). A set holding some is closed around them.
	var open []events.Item
	for _, item := range items {
		if item.Status != events.StatusDone {
			open = append(open, item)
		}
	}
	if len(open) == 0 {
		m.hint = "every marked item is already closed"
		if len(items) == 1 {
			m.hint = items[0].Id + " is already closed"
		}
		return m, nil, true
	}

	// A heading closes only once the work beneath it is finished (§5). The guard is the one `waid
	// done` calls, so the two surfaces cannot disagree about when that is, and it judges the set as
	// one: a heading marked alongside everything open beneath it closes with it.
	if err := itemtree.GuardCloses(events.State{Items: m.loops.items}, idsOf(open)); err != nil {
		m.hint = err.Error()
		return m, nil, true
	}

	writes := make([]itemWrite, 0, len(open))
	for _, item := range open {
		closed := item
		closed.Status = events.StatusDone
		writes = append(writes, itemWrite{
			event:   events.CloseEvent{Ev: "close", Id: item.Id},
			after:   closed,
			receipt: receipt{verb: "closed", subject: item.Id, detail: item.Title},
			inverse: undoDone(item),
		})
	}

	m, cmd := m.commit("done", writes, receipt{verb: "closed", subject: plural(len(writes), "item")})
	return m, cmd, true
}

// headingSelected marks the item under the cursor a heading, or takes the mark off one that already
// holds it (§6). The event is written whatever the item held, which is how `waid heading` writes it:
// the log is a history of what was asked for, the fold is idempotent, and a key that sometimes wrote
// and sometimes did not would make `u` guess.
func (m Model) headingSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "H marks an item a heading")
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

// promptSubject is what a prompt's label names: the id of the one item it is asking about, or how
// many it is asking about at once.
func promptSubject(items []events.Item) string {
	if len(items) == 1 {
		return items[0].Id
	}
	return "(" + plural(len(items), "item") + ")"
}

// waitingSelected asks who the marked items — or the item under the cursor — are waiting on. The
// name is what the status is for, since an item waiting on nobody is just open, so it is collected
// before anything is written.
func (m Model) waitingSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, items, ok := m.targets(tree, "w marks an item waiting")
	if !ok {
		return m, nil, true
	}

	m.prompt = m.open(prompt{
		kind: promptWaiting, label: "waiting on", subject: promptSubject(items), subjects: idsOf(items),
	})
	return m, nil, true
}

// waitOn writes the status and the name together to each item, since an update carrying one without
// the other would leave the item describing half a state.
func (m Model) waitOn(ids []string, who string) (Model, tea.Cmd) {
	var writes []itemWrite
	for _, id := range ids {
		item, loaded := m.loadedItem(id)
		if !loaded {
			continue
		}

		waiting := item
		waiting.Status, waiting.WaitingOn = events.StatusWaiting, &who
		writes = append(writes, itemWrite{
			event:   events.UpdateEvent{Ev: "update", Id: id, Status: events.StatusWaiting, WaitingOn: &who},
			after:   waiting,
			receipt: receipt{verb: "waiting", subject: id, detail: who},
			inverse: undoWaiting(item),
		})
	}
	if len(writes) == 0 {
		m.hint = noItems
		return m, nil
	}

	return m.commit("waiting", writes, receipt{verb: "waiting", subject: plural(len(writes), "item"), detail: who})
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
	m.prompt = m.open(prompt{kind: promptRename, label: "edit", subject: item.Id, prior: item.Title})
	return m, nil, true
}

// tagSelected edits the tags on the item under the cursor, or adds tags to the marked items.
//
// For one item the input opens on the set it already carries, so the whole set is in front of
// whoever is editing it — which is what makes a replacement safe here and not on the command line,
// where `waid tag` names a change instead (§7). Several items carry several sets and no one line can
// stand for them, so that input opens empty and what is typed is added to each.
func (m Model) tagSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, items, ok := m.targets(tree, "t tags an item")
	if !ok {
		return m, nil, true
	}

	asked := prompt{kind: promptTags, label: "add tags", subject: promptSubject(items), subjects: idsOf(items)}
	if len(items) == 1 {
		// The set is seeded with a space after each comma, since it is being read as well as typed. The
		// spaces are trimmed back off on the way to the log.
		asked.label, asked.prior = "tags", strings.Join(items[0].Tags, ", ")
	}

	m.prompt = m.open(asked)
	return m, nil, true
}

// setTags writes the typed tags: one item has its set replaced by them, and several each have them
// added to the set they carry. Either way the event carries the item's whole set rather than what
// changed, so the fold reads one line and `waid tag` writes the same shape (§1).
//
// The write is made whatever the item already held, for the reason the heading toggle is: the log is
// a history of what was asked for, and a key that sometimes wrote and sometimes did not would leave
// `u` guessing which press it was reversing. An answer that adds nothing to several items is the
// exception, since it asked for nothing.
func (m Model) setTags(ids []string, answer string) (Model, tea.Cmd) {
	typed := events.NormalizeTags(strings.Split(answer, ","))
	adding := len(ids) > 1
	if adding && len(typed) == 0 {
		return m, nil
	}

	var writes []itemWrite
	for _, id := range ids {
		item, loaded := m.loadedItem(id)
		if !loaded {
			continue
		}

		tags := typed
		if adding {
			tags = events.NormalizeTags(slices.Concat(item.Tags, typed))
		}

		tagged := item
		tagged.Tags = tags
		writes = append(writes, itemWrite{
			event:   events.TagsEvent{Ev: "update", Id: id, Tags: tags},
			after:   tagged,
			receipt: tagsReceipt(tagged),
			inverse: undoTags(item),
		})
	}
	if len(writes) == 0 {
		m.hint = noItems
		return m, nil
	}

	summary := receipt{verb: "tagged", subject: plural(len(writes), "item"), detail: strings.Join(typed, ", ")}
	return m.commit("tags", writes, summary)
}

// tagsReceipt is what a tag write says it did, read off the item as it stands afterwards so the
// inverse can share it. An item left carrying none has no set to name, so the receipt names the item
// instead — the wording `waid tag` prints for the same write.
func tagsReceipt(item events.Item) receipt {
	if len(item.Tags) == 0 {
		return receipt{verb: "untagged", subject: item.Id, detail: item.Title}
	}
	return receipt{verb: "tagged", subject: item.Id, detail: strings.Join(item.Tags, ", ")}
}

// noteSelected collects a note against the item under the cursor.
func (m Model) noteSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "n notes against an item")
	if !ok {
		return m, nil, true
	}

	m.prompt = m.open(prompt{kind: promptNote, label: "note", subject: item.Id})
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
	m.prompt = m.open(prompt{kind: promptAdd, label: "add", subject: target.label, target: target})
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
