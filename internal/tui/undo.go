package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/triage"
)

// Undo is a bounded stack of inverse commands rather than a state snapshot, which is what the event
// log makes safe: reversing a write is another write, so an undo appends rather than erases and the
// reversal stays visible in the log. For an append-only design that is the correct behaviour (§3).

// undoLimit is how many writes back the stack reaches. It is bounded because a triage pass is long
// and an unbounded stack would hold every write of a session for an affordance that is only ever
// reached for the last few.
const undoLimit = 20

// undoEntry is one write's inverse: the events that reverse it, the receipt the reversal leaves, and
// the signal it puts back on Scan.
type undoEntry struct {
	events []events.WaidEvent

	// receipt is what the undo shows in the footer and prints in the replay, since an undo is itself a
	// write (§3.1).
	receipt receipt

	// key is the signal a retired row is restored by, empty for a write no signal was retired by.
	key string
}

// undoDismiss restores a hidden key.
func undoDismiss(key string) undoEntry {
	return undoEntry{
		events:  []events.WaidEvent{events.UndismissEvent{Ev: "undismiss", Key: key}},
		receipt: receipt{verb: "undismissed", subject: key},
		key:     key,
	}
}

// undoPromote reverses both halves of a promotion: the item the signal became is closed, and the key
// the promotion dismissed is restored so the signal is reported again.
func undoPromote(promotion triage.Promotion) undoEntry {
	return undoEntry{
		events: []events.WaidEvent{
			events.CloseEvent{Ev: "close", Id: promotion.Id},
			events.UndismissEvent{Ev: "undismiss", Key: promotion.Key},
		},
		receipt: receipt{verb: "unpromoted", subject: promotion.Id, detail: promotion.Title},
		key:     promotion.Key,
	}
}

// undoDone reopens a closed item. item is the item as it stood before the write.
func undoDone(item events.Item) undoEntry {
	return undoEntry{
		events:  []events.WaidEvent{events.ReopenEvent{Ev: "reopen", Id: item.Id}},
		receipt: receipt{verb: "reopened", subject: item.Id, detail: item.Title},
	}
}

// undoWaiting puts back the status and the waiting-on the item held before the write. reopen leads
// because it is the only event that clears waitingOn — an update omits the field when it carries no
// name, which would leave the name the undone write set. Anything the reopen then overshoots is put
// back by the update behind it.
func undoWaiting(item events.Item) undoEntry {
	inverse := []events.WaidEvent{events.ReopenEvent{Ev: "reopen", Id: item.Id}}
	if item.Status != events.StatusOpen || item.WaitingOn != nil {
		inverse = append(inverse, events.UpdateEvent{
			Ev:        "update",
			Id:        item.Id,
			Status:    item.Status,
			WaitingOn: item.WaitingOn,
		})
	}

	return undoEntry{events: inverse, receipt: receipt{verb: "restored", subject: item.Id, detail: item.Title}}
}

// undoRetitle puts back the title an item carried before it was edited.
func undoRetitle(id, title string) undoEntry {
	return undoEntry{
		events:  []events.WaidEvent{events.UpdateEvent{Ev: "update", Id: id, Title: &title}},
		receipt: receipt{verb: verbRetitled, subject: id, detail: title},
	}
}

// pushUndo records the inverse of a write just made. The append is capped to the stack's own length
// so it allocates a fresh array: Model is copied by value through the update loop, and two copies
// sharing a backing array would overwrite each other's last entry.
func (m Model) pushUndo(entry undoEntry) Model {
	stack := append(m.undos[:len(m.undos):len(m.undos)], entry)
	if len(stack) > undoLimit {
		stack = stack[len(stack)-undoLimit:]
	}

	m.undos = stack
	return m
}

// undo reverses the last write. The entry leaves the stack before its events are written, so a write
// that fails part way through is not repeated by a second press over the events that did land.
func (m Model) undo() (Model, tea.Cmd) {
	if len(m.undos) == 0 {
		m.hint = "nothing to undo"
		return m, nil
	}

	entry := m.undos[len(m.undos)-1]
	m.undos = m.undos[:len(m.undos)-1]

	for _, event := range entry.events {
		if _, err := events.Append(m.opts.Cfg.EventsPath, event, m.now()); err != nil {
			m.hint = writeFailed("undo", err)
			return m, nil
		}
	}

	if entry.key != "" {
		m = m.unhide(entry.key)
	}
	return m.record(entry.receipt), nil
}

// undoAffordance is the keys the footer offers against the write that just landed. Promote is the one
// action carrying text — an item title read for weeks afterwards — so the rename sits beside the undo
// and the correction stays one key away (§3).
func (m Model) undoAffordance() string {
	if len(m.undos) == 0 {
		return ""
	}
	if _, ok := m.renameTarget(); ok {
		return "u undo · e rename"
	}
	return "u undo"
}
