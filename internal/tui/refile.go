package tui

import (
	"os"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
)

// Filing an item under a project is the one write that needs more than a line of text: a project is
// an absolute path, and typing one out is not something to do at a prompt. So the prompt searches
// instead — the fragment names a project the log or the disk already knows, and the footer lists what
// it matched so enter is never a guess (§4).

// projectMatchesShown is how many matches the footer lists. The rest are counted rather than drawn:
// the footer grows into the list above it, and a fragment matching a dozen projects is a fragment to
// narrow rather than a list to read.
const projectMatchesShown = 5

// projectPrompt opens the search on the project the item is already filed under, since a move is read
// against where the item sits. ctrl-u empties it, which is both how a search starts from nothing and
// how an item is filed under no project at all.
func (m Model) projectPrompt(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(tree, "P files an item under a project")
	if !ok {
		return m, nil, true
	}

	m.prompt = prompt{kind: promptProject, label: "project", subject: item.Id, value: pathOrEmpty(item.Project)}
	return m, nil, true
}

// projectCandidates are the paths the search matches against: every project the log or the harvested
// sessions name, and every repository the last detection pass discovered. The pass walks the scan
// roots anyway, so a checkout with nothing to report is a candidate at no cost — and a loop is as
// often filed under a repo not touched in weeks as under one that is.
func (m Model) projectCandidates() []string {
	recorded := make([]string, 0, len(m.review.sessions))
	for _, session := range m.review.sessions {
		if session.Project != nil {
			recorded = append(recorded, *session.Project)
		}
	}

	candidates := project.KnownProjects(m.loops.items, recorded)
	for _, repo := range m.scan.result.Repos {
		if repo != "" && !slices.Contains(candidates, repo) {
			candidates = append(candidates, repo)
		}
	}

	slices.SortFunc(candidates, func(a, b string) int { return project.CompareNames(&a, &b) })
	return candidates
}

// projectMatches are the candidates a fragment names, case-insensitively, in the order the footer
// lists them. An empty fragment matches nothing rather than everything: it is the answer that files
// the item under no project, and a list of candidates behind it would make enter mean the first of
// them instead.
func (m Model) projectMatches(fragment string) []string {
	needle := strings.ToLower(strings.TrimSpace(fragment))
	if needle == "" {
		return nil
	}

	matched := []string{}
	for _, candidate := range m.projectCandidates() {
		if strings.Contains(strings.ToLower(candidate), needle) {
			matched = append(matched, candidate)
		}
	}
	return matched
}

// moveChoice walks the match the prompt is on, wrapping at both ends so a short list is circled
// rather than run off. A prompt with nothing to choose between is left alone, which is every prompt
// but the project search — the others take a line of text and have no list under them.
func (m Model) moveChoice(by int) Model {
	if m.prompt.kind != promptProject {
		return m
	}

	count := len(m.projectMatches(m.prompt.value))
	if count == 0 {
		return m
	}

	m.prompt.choice = ((m.prompt.choice+by)%count + count) % count
	return m
}

// chosenProject is the project the answer names: the match the prompt is on, or — when a fragment
// matched nothing — whatever `waid add -p` would make of the same text, so a checkout outside the
// scan roots can still be named by its path. An empty answer is no project.
func (m Model) chosenProject(fragment string, choice int) (*string, error) {
	if strings.TrimSpace(fragment) == "" {
		return nil, nil
	}
	if matches := m.projectMatches(fragment); len(matches) > 0 {
		return &matches[min(choice, len(matches)-1)], nil
	}

	// The known set is empty here by construction: the matches above are drawn from it, and none
	// matched. What is left for Resolve to recognise is a path — absolute, or `.` for the directory
	// the app was started in.
	cwd, _ := os.Getwd()
	return project.Resolve(fragment, nil, cwd)
}

// refile writes the move and puts the row in its new group in the frame the key was pressed in. An
// answer naming the project the item is already filed under is not a write — the log would hold a
// move that moved nothing (§4).
func (m Model) refile(id, fragment string, choice int) (Model, tea.Cmd) {
	item, loaded := m.loadedItem(id)
	if !loaded {
		m.hint = noItems
		return m, nil
	}

	chosen, err := m.chosenProject(fragment, choice)
	if err != nil {
		m.hint = err.Error()
		return m, nil
	}
	if samePath(chosen, item.Project) {
		return m, nil
	}

	ts, err := events.Append(m.opts.Cfg.EventsPath, events.ProjectEvent{Ev: "update", Id: id, Project: chosen}, m.now())
	if err != nil {
		m.hint = writeFailed("project", err)
		return m, nil
	}

	filed := item
	filed.Project, filed.Updated = chosen, ts

	m = m.applyItem(filed).pushUndo(undoRefile(item))
	return m.record(receipt{verb: verbFiled, subject: id, detail: projectLabel(chosen)}), nil
}

// projectChoices is what the footer draws under the search while it is open, in place of the tab's
// key hints: the matches the fragment named, or the reason there are none to offer.
func (m Model) projectChoices() string {
	if strings.TrimSpace(m.prompt.value) == "" {
		return m.theme.Dim.Render(" enter files " + m.prompt.subject + " under no project")
	}

	matches := m.projectMatches(m.prompt.value)
	if len(matches) == 0 {
		return m.theme.Dim.Render(" no project matches — enter takes an absolute path as typed")
	}

	var lines []string
	for i, match := range matches[:min(len(matches), projectMatchesShown)] {
		if i == min(m.prompt.choice, len(matches)-1) {
			lines = append(lines, m.theme.RowFocused.Render(" › "+match))
			continue
		}
		lines = append(lines, m.theme.Row.Render("   "+match))
	}
	if hidden := len(matches) - len(lines); hidden > 0 {
		lines = append(lines, m.theme.Dim.Render("   … "+plural(hidden, "more match")))
	}
	return strings.Join(lines, "\n")
}

// samePath reports whether two projects are the same one, counting no project as a project two items
// can share.
func samePath(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// pathOrEmpty is a project as a prompt opens on it, which is nothing at all for an item filed under
// none.
func pathOrEmpty(path *string) string {
	if path == nil {
		return ""
	}
	return *path
}
