package tui

import (
	"fmt"
	"maps"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/triage"
)

// The two writes Scan makes go through internal/triage, the same code the promote and dismiss
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
		m.hint = inertOn(tree, "p promotes a signal — this row is a project")
		return m, nil, true
	}

	path := m.opts.Cfg.EventsPath
	promoted, err := triage.Promote(path, events.Load(path), m.opts.Ids, signal, signal.Title, m.now())
	if err != nil {
		m.hint = writeFailed("promote", err)
		return m, nil, true
	}

	m = m.hide(signal.Key)
	return m.record(receipt{verb: "promoted", subject: promoted.Id, detail: promoted.Title}), nil, true
}

// dismissSelected hides the signal under the cursor for good.
func (m Model) dismissSelected(tree Tree[detect.Signal]) (Model, tea.Cmd, bool) {
	signal, ok := tree.SelectedItem()
	if !ok {
		m.hint = inertOn(tree, "d dismisses a signal — this row is a project")
		return m, nil, true
	}

	if err := triage.Dismiss(m.opts.Cfg.EventsPath, signal.Key, m.now()); err != nil {
		m.hint = writeFailed("dismiss", err)
		return m, nil, true
	}

	m = m.hide(signal.Key)
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

// inertOn is the reason an action needing a signal did nothing. A list with nothing in it has no row
// to describe, so the heading's reason would be a lie there (§4).
func inertOn(tree Tree[detect.Signal], hint string) string {
	if tree.Len() == 0 {
		return "there are no signals to triage"
	}
	return hint
}

// writeFailed is what the footer says when the log could not be appended to.
func writeFailed(action string, err error) string {
	return fmt.Sprintf("%s failed: %v", action, err)
}
