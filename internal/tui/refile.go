package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/events"
)

// Moving an item under another is the one write that needs more than a line of text: a parent is an
// id, and an id is not something anyone types from memory. So the prompt searches instead — the
// fragment names an item the log already holds, and the footer lists what it matched so enter is
// never a guess (§6).

// parentMatchesShown is how many matches the footer lists. The rest are counted rather than drawn:
// the footer grows into the list above it, and a fragment matching a dozen items is a fragment to
// narrow rather than a list to read.
const parentMatchesShown = 5

// topLevel is what an item with no parent is called wherever one is named — the prompt's footer,
// the receipt, and the undo that puts it back.
const topLevel = "(top level)"

// parentPrompt opens the search on the title of the parent the item already sits under, since a move
// is read against where the item is. ctrl-u empties it, which is both how a search starts from
// nothing and how an item is moved back to the top level.
func (m Model) parentPrompt(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "P moves an item under another")
	if !ok {
		return m, nil, true
	}

	value := ""
	if parent, found := m.loadedItem(derefOr(item.Parent)); found {
		value = parent.Title
	}

	m.prompt = prompt{kind: promptParent, label: "parent", subject: item.Id, value: value}
	return m, nil, true
}

// parentCandidates are the items the search matches against: everything the log holds but the item
// being moved and what already hangs beneath it, since an item cannot become its own descendant's
// child. They are ordered by title, which is the order the footer offers them in.
func (m Model) parentCandidates(subject string) []events.Item {
	beneath := m.descendants(subject)

	candidates := make([]events.Item, 0, len(m.loops.items))
	for _, item := range m.loops.items {
		if item.Id == subject || slices.Contains(beneath, item.Id) {
			continue
		}
		candidates = append(candidates, item)
	}

	slices.SortFunc(candidates, func(a, b events.Item) int {
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})
	return candidates
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

// parentMatches are the candidates a fragment names, case-insensitively, in the order the footer
// lists them. An empty fragment matches nothing rather than everything: it is the answer that moves
// the item to the top level, and a list of candidates behind it would make enter mean the first of
// them instead.
func (m Model) parentMatches(subject, fragment string) []events.Item {
	needle := strings.ToLower(strings.TrimSpace(fragment))
	if needle == "" {
		return nil
	}

	matched := []events.Item{}
	for _, candidate := range m.parentCandidates(subject) {
		if strings.Contains(strings.ToLower(candidate.Title), needle) {
			matched = append(matched, candidate)
		}
	}
	return matched
}

// moveChoice walks the match the prompt is on, wrapping at both ends so a short list is circled
// rather than run off. A prompt with nothing to choose between is left alone, which is every prompt
// but the parent search — the others take a line of text and have no list under them.
func (m Model) moveChoice(by int) Model {
	if m.prompt.kind != promptParent {
		return m
	}

	count := len(m.parentMatches(m.prompt.subject, m.prompt.value))
	if count == 0 {
		return m
	}

	m.prompt.choice = ((m.prompt.choice+by)%count + count) % count
	return m
}

// chosenParent is the item the answer names: the match the prompt is on, or nothing at all for an
// empty answer, which is the top level. A fragment that matched nothing is an error rather than a
// write — there is no path to fall back on, since a path is no longer a place in the tree (§6).
func (m Model) chosenParent(subject, fragment string, choice int) (*string, error) {
	if strings.TrimSpace(fragment) == "" {
		return nil, nil
	}

	matches := m.parentMatches(subject, fragment)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no item matches %q", strings.TrimSpace(fragment))
	}

	return &matches[min(choice, len(matches)-1)].Id, nil
}

// refile writes the move in the frame the key was pressed in. An answer naming the parent the item
// already sits under is not a write — the log would hold a move that moved nothing (§6).
func (m Model) refile(id, fragment string, choice int) (Model, tea.Cmd) {
	item, loaded := m.loadedItem(id)
	if !loaded {
		m.hint = noItems
		return m, nil
	}

	chosen, err := m.chosenParent(id, fragment, choice)
	if err != nil {
		m.hint = err.Error()
		return m, nil
	}
	if sameRef(chosen, item.Parent) {
		return m, nil
	}

	ts, err := events.Append(m.opts.Cfg.EventsPath, events.ParentEvent{Ev: "update", Id: id, Parent: chosen}, m.now())
	if err != nil {
		m.hint = writeFailed("parent", err)
		return m, nil
	}

	moved := item
	moved.Parent, moved.Updated = chosen, ts

	m = m.applyItem(moved).pushUndo(undoRefile(item, m.parentLabel(item.Parent)))
	return m.record(receipt{verb: verbFiled, subject: id, detail: m.parentLabel(chosen)}), nil
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

// parentChoices is what the footer draws under the search while it is open, in place of the tab's
// key hints: the matches the fragment named, or the reason there are none to offer.
func (m Model) parentChoices() string {
	if strings.TrimSpace(m.prompt.value) == "" {
		return m.theme.Dim.Render(" enter moves " + m.prompt.subject + " to the top level")
	}

	matches := m.parentMatches(m.prompt.subject, m.prompt.value)
	if len(matches) == 0 {
		return m.theme.Dim.Render(" no item matches — narrow the search or esc to abandon it")
	}

	var lines []string
	for i, match := range matches[:min(len(matches), parentMatchesShown)] {
		if i == min(m.prompt.choice, len(matches)-1) {
			lines = append(lines, m.theme.RowFocused.Render(" › "+match.Title))
			continue
		}
		lines = append(lines, m.theme.Row.Render("   "+match.Title))
	}
	if hidden := len(matches) - len(lines); hidden > 0 {
		lines = append(lines, m.theme.Dim.Render("   … "+plural(hidden, "more match")))
	}
	return strings.Join(lines, "\n")
}

// sameRef reports whether two optional ids are the same one, counting the top level as a place two
// items can share.
func sameRef(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// derefOr is an optional id as a lookup takes it, where the empty string is an id nothing holds.
func derefOr(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}
