package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// promptKind is what an open prompt is collecting, and which write its answer makes. promptNone is
// the zero value, so a Model with nothing to ask has no prompt.
type promptKind int

const (
	promptNone promptKind = iota
	promptRename
)

// prompt is the inline input the footer takes text in. It holds the keyboard the way the filter does,
// so a title containing `q` cannot quit the app out from under the person typing it.
type prompt struct {
	kind promptKind

	// label is the verb the footer prints, and subject is what the answer is about — the id or key,
	// so the line reads `rename 7k3m`.
	label   string
	subject string

	// prior is what the answer replaces, kept for the inverse the write pushes onto the undo stack.
	prior string

	value string
}

// promptKey edits the answer. esc abandons it, enter commits it.
func (m Model) promptKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.prompt = prompt{}
	case tea.KeyEnter:
		return m.commitPrompt()
	case tea.KeyBackspace:
		if runes := []rune(m.prompt.value); len(runes) > 0 {
			m.prompt.value = string(runes[:len(runes)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		m.prompt.value += msg.String()
	}
	return m, nil
}

// commitPrompt closes the prompt and makes the write its answer asked for. An empty answer is an
// abandoned edit rather than a write of nothing.
func (m Model) commitPrompt() (Model, tea.Cmd) {
	answered := m.prompt
	m.prompt = prompt{}

	value := strings.TrimSpace(answered.value)
	if value == "" {
		return m, nil
	}

	switch answered.kind {
	case promptRename:
		return m.rename(answered.subject, answered.prior, value)
	}
	return m, nil
}

// promptLine is the prompt as the footer shows it, with the cursor the filter uses.
func (m Model) promptLine() string {
	return m.theme.FilterActive.Render(" " + m.prompt.label + " " + m.prompt.subject + " " + m.prompt.value + "▏")
}
