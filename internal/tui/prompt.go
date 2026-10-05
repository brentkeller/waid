package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// promptKind is what an open prompt is collecting, and what its answer does — a write, for all but
// promptDate, which moves the Agents window instead. promptNone is the zero value, so a Model with
// nothing to ask has no prompt.
type promptKind int

const (
	promptNone promptKind = iota
	promptRename
	promptWaiting
	promptNote
	promptTags
	promptAdd
	promptDate
)

// promptMaxHeight caps the rows a wrapping answer takes from the list above it. Past the cap the
// input scrolls, keeping the cursor in view, so a long note stays typeable on a short terminal.
const promptMaxHeight = 6

// prompt is the inline input the footer takes text in. It holds the keyboard the way the filter does,
// so a title containing `q` cannot quit the app out from under the person typing it.
type prompt struct {
	kind promptKind

	// label is the verb the footer prints, and subject is what the answer is about — the id or key,
	// so the line reads `rename 7k3m`.
	label   string
	subject string

	// subjects are the items a waiting or a tags answer is written to: one for a prompt opened on the
	// row under the cursor, and every marked item otherwise. subject is what the footer prints for
	// them, which is an id for one and a count for several.
	subjects []string

	// prior is what the answer replaces, kept for the inverse the write pushes onto the undo stack.
	// It is also what the input opens on, since a prompt that replaces something opens on it.
	prior string

	// target is where an added item lands, resolved from the row the prompt was opened on. The
	// subject names it for the footer; this is what the write carries, since the two differ.
	target addTarget

	input textarea.Model
}

// text is the answer as it stands.
func (p prompt) text() string { return p.input.Value() }

// open fits a prompt with the input that takes its answer, seeded with what the answer replaces and
// with the cursor at the end of it, so a prefilled prompt is ready to be appended to or backed over.
func (m Model) open(p prompt) prompt {
	input := textarea.New()
	input.ShowLineNumbers = false
	input.MaxHeight = promptMaxHeight

	// A static cursor asks for no blink ticks, so the app still wakes only for the work it is
	// doing rather than on a timer while a prompt sits open.
	input.Cursor.SetMode(cursor.CursorStatic)

	input.FocusedStyle = promptStyles(m.theme)
	input.BlurredStyle = input.FocusedStyle

	// enter commits the answer, so it cannot also open a line: every prompt collects one line, and
	// the writes reading the answer back expect one.
	input.KeyMap.InsertNewline.SetEnabled(false)

	// ctrl-arrow moves by word beside the readline keys, since ctrl-arrow is what the editors and
	// browsers around the app use for the same move. The whole binding is named rather than appended
	// to, so it does not depend on what the component happens to bind by default.
	input.KeyMap.WordForward.SetKeys("ctrl+right", "alt+right", "alt+f")
	input.KeyMap.WordBackward.SetKeys("ctrl+left", "alt+left", "alt+b")

	input.SetValue(p.prior)
	input.CursorEnd()
	input.Focus()

	p.input = input
	return p.fit(m.viewWidth())
}

// promptResized is the no-op the input is handed after it is resized, so it scrolls the cursor back
// into view: the component repositions itself only while handling a message.
type promptResized struct{}

// fit sizes the input to the terminal and to what has been typed into it. The label is the input's
// own per-line prompt, which is what makes a wrapped answer hang under the first line's text instead
// of under the label; the height follows the wrap, so the footer grows by the rows the answer takes
// and the list above it gives up exactly that many.
func (p prompt) fit(width int) prompt {
	prefix := " " + strings.TrimSpace(p.label+" "+p.subject) + " "
	p.input.SetPromptFunc(lipgloss.Width(prefix), func(line int) string {
		if line == 0 {
			return prefix
		}
		return ""
	})
	p.input.SetWidth(width)
	p.input.SetHeight(p.input.LineInfo().Height)
	// The scroll is measured against what the input last drew, so the rewrap has to be drawn before it
	// can be scrolled to: at the old width the answer took fewer rows than there are to move through.
	p.input.View()
	p.input, _ = p.input.Update(promptResized{})
	return p
}

// promptStyles dresses the input in the footer's own colour, cursor line included: the prompt is one
// line that happens to wrap, so no row of it is lit differently from the rest.
func promptStyles(theme Theme) textarea.Style {
	return textarea.Style{
		Base:       lipgloss.NewStyle(),
		Text:       theme.FilterActive,
		Prompt:     theme.FilterActive,
		CursorLine: theme.FilterActive,
	}
}

// promptKey edits the answer. esc abandons it and enter commits it; everything else is text or a
// move within it, which is the input's own business (§4).
func (m Model) promptKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.prompt = prompt{}
		return m, nil
	case tea.KeyEnter:
		return m.commitPrompt()
	}

	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.edit(msg, m.viewWidth())
	return m, cmd
}

// edit hands the key to the input and refits the prompt around what it did. The input is given the
// full height it may take before the key lands: it repositions its own view only while handling a
// message, and a height momentarily too short for the answer leaves it scrolled past the first line.
func (p prompt) edit(msg tea.KeyMsg, width int) (prompt, tea.Cmd) {
	p.input.SetHeight(promptMaxHeight)

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p.fit(width), cmd
}

// commitPrompt closes the prompt and makes the write its answer asked for.
func (m Model) commitPrompt() (Model, tea.Cmd) {
	answered := m.prompt
	m.prompt = prompt{}

	// An empty answer is an abandoned edit rather than a write of nothing — except for the tags of one
	// item, where it is a write of nothing in the literal sense: an item with no tags is a state to
	// want, and esc is what abandons an edit. A tags answer for several items adds to each rather than
	// replacing, so a blank one adds nothing and is abandoned like the rest.
	value := strings.TrimSpace(answered.text())
	clears := answered.kind == promptTags && len(answered.subjects) == 1
	if value == "" && !clears {
		return m, nil
	}

	switch answered.kind {
	case promptRename:
		return m.rename(answered.subject, answered.prior, value)
	case promptWaiting:
		return m.waitOn(answered.subjects, value)
	case promptNote:
		return m.addNote(answered.subject, value)
	case promptTags:
		return m.setTags(answered.subjects, value)
	case promptAdd:
		return m.addItem(answered.target, value)
	case promptDate:
		return m.pickDate(value), nil
	}
	return m, nil
}

// promptLine is the prompt as the footer shows it — the label, the answer wrapped under it, and the
// cursor sitting where the next keystroke lands.
func (m Model) promptLine() string {
	return strings.TrimRight(m.prompt.input.View(), "\n")
}
