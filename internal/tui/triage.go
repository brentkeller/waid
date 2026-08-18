package tui

import (
	"fmt"
	"maps"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/triage"
)

// The two writes Repos makes go through internal/triage, the same code the promote and dismiss
// commands write through, and they land on the keypress with no confirm step: in an app you live in,
// "press Enter to commit" is a modal interruption repeated all day (§3).
//
// The log is a local append, so the write is made inside the update loop rather than off it. The row
// has to leave the list and the receipt has to reach the footer in the frame the key was pressed in,
// which a round trip through a tea.Cmd could not promise.

// promoteSelected turns the signal under the cursor into a declared item carrying the signal's own
// title, which is the common case §3 costs one key. A project heading is not a signal, so the action
// is inert there and the footer says why (§4).
func (m Model) promoteSelected(tree Tree[detect.Signal]) (Model, tea.Cmd, bool) {
	signal, ok := tree.SelectedItem()
	if !ok {
		m.hint = inertOn(tree, noSignals, "p promotes a signal — this row is a project")
		return m, nil, true
	}

	path := m.opts.Cfg.EventsPath
	promoted, err := triage.Promote(path, events.Load(path), m.opts.Ids, signal, signal.Title, m.now())
	if err != nil {
		m.hint = writeFailed("promote", err)
		return m, nil, true
	}

	m = m.hide(signal.Key).pushUndo(undoPromote(promoted))
	return m.record(receipt{verb: verbPromoted, subject: promoted.Id, detail: promoted.Title}), nil, true
}

// renameSelected corrects the title a promotion was written with. It acts on the receipt rather than
// on the cursor: the row it addresses has already left the list, and the promoted item is not a
// signal to point at (§3).
func (m Model) renameSelected() (Model, tea.Cmd, bool) {
	target, ok := m.renameTarget()
	if !ok {
		m.hint = "e renames an item just promoted"
		return m, nil, true
	}

	// The input opens on the title being corrected, for the reason Loops' edit does.
	m.prompt = prompt{kind: promptRename, label: "rename", subject: target.subject, prior: target.detail, value: target.detail}
	return m, nil, true
}

// rename writes the corrected title and leaves its own receipt, since it is a write of its own and
// the write before it is already on the undo stack.
func (m Model) rename(id, prior, title string) (Model, tea.Cmd) {
	if title == prior {
		return m, nil
	}

	event := events.UpdateEvent{Ev: "update", Id: id, Title: &title}
	ts, err := events.Append(m.opts.Cfg.EventsPath, event, m.now())
	if err != nil {
		m.hint = writeFailed("rename", err)
		return m, nil
	}

	// A title corrected on Loops is a row on screen; one corrected on Repos belongs to an item the log
	// has not been re-read for, so the loaded list is moved only where it holds the item.
	inverse := undoRetitle(id, prior)
	if item, loaded := m.loadedItem(id); loaded {
		retitled := item
		retitled.Title, retitled.Updated = title, ts
		m, inverse = m.applyItem(retitled), inverse.restoring(item)
	}

	m = m.pushUndo(inverse)
	return m.record(receipt{verb: verbRetitled, subject: id, detail: title}), nil
}

// renameTarget is the item the footer is offering `e` for: the promotion the last write made, or the
// correction of one, so a title can be corrected twice.
func (m Model) renameTarget() (receipt, bool) {
	// Loops binds `e` to the item under the cursor, so the receipt's rename is offered only where the
	// key is free to mean it.
	if m.tab != tabScan {
		return receipt{}, false
	}

	last, written := m.lastReceipt()
	if !written || (last.verb != verbPromoted && last.verb != verbRetitled) {
		return receipt{}, false
	}
	return last, true
}

// dismissSelected hides the signal under the cursor for good.
func (m Model) dismissSelected(tree Tree[detect.Signal]) (Model, tea.Cmd, bool) {
	signal, ok := tree.SelectedItem()
	if !ok {
		m.hint = inertOn(tree, noSignals, "d dismisses a signal — this row is a project")
		return m, nil, true
	}

	if err := triage.Dismiss(m.opts.Cfg.EventsPath, signal.Key, m.now()); err != nil {
		m.hint = writeFailed("dismiss", err)
		return m, nil, true
	}

	m = m.hide(signal.Key).pushUndo(undoDismiss(signal.Key))
	return m.record(receipt{verb: "dismissed", subject: signal.Key}), nil, true
}

// hide drops a triaged signal from the list the loaded pass is showing. Detection is not re-run — the
// pass is only re-read on r, and a written key is settled either way. The set is replaced rather than
// added to because Model is copied by value through the update loop, and two copies sharing one map
// would see each other's writes.
func (m Model) hide(key string) Model {
	hidden := make(map[string]bool, len(m.scan.triaged)+1)
	maps.Copy(hidden, m.scan.triaged)
	hidden[key] = true
	m.scan.triaged = hidden

	m.counts[tabScan] = m.scanCount()

	// The row that left may have been the last one, so the cursor is pulled back into range.
	m.scan.cursor = m.scanTree(m.viewWidth()).Cursor
	return m
}

// unhide puts a triaged signal back in the list an undo restored it to.
func (m Model) unhide(key string) Model {
	if !m.scan.triaged[key] {
		return m
	}

	restored := make(map[string]bool, len(m.scan.triaged))
	maps.Copy(restored, m.scan.triaged)
	delete(restored, key)
	m.scan.triaged = restored

	m.counts[tabScan] = m.scanCount()
	return m
}

// The two reasons an action found nothing at all to act on, which is not the same as finding the
// wrong kind of row.
const (
	noItems    = "there are no items here"
	noSignals  = "there are no signals to triage"
	noSessions = "there are no sessions here"
)

// inertOn is the reason an action needing a row did nothing. A list with nothing in it has no row to
// describe, so the heading's reason would be a lie there (§4).
func inertOn[T any](tree Tree[T], empty, hint string) string {
	if tree.Len() == 0 {
		return empty
	}
	return hint
}

// writeFailed is what the footer says when the log could not be appended to.
func writeFailed(action string, err error) string {
	return fmt.Sprintf("%s failed: %v", action, err)
}
