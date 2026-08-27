package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The cursor moves inside the answer, so a typo in the middle of a title is fixed where it sits rather
// than by deleting back to it.
func TestPromptCursorMovesBackToInsert(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "a", "a", "b", "c", "left", "X")

	if got, want := m.prompt.text(), "abXc"; got != want {
		t.Errorf("left then X left %q, want %q", got, want)
	}
}

// typing feeds a string into an open prompt a keystroke at a time, rendering between them the way the
// update loop does. The render matters: the input positions its view against what it last drew, so a
// prompt typed into without being drawn is not the prompt the app has on screen.
func typing(t *testing.T, m Model, text string) Model {
	t.Helper()

	for _, r := range strings.Split(text, "") {
		m, _ = press(t, m, r)
		m.View()
	}
	return m
}

// narrow is a working model on a terminal too small for a long answer to fit on one line.
func narrow(t *testing.T, width int) (Model, string) {
	t.Helper()

	m, path := working(t)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
	return resized.(Model), path
}

// An answer wider than the terminal wraps inside the frame rather than running past its right edge,
// so what is being typed stays on screen.
func TestPromptWrapsALongAnswerInsideTheFrame(t *testing.T) {
	const width = 60
	m, _ := narrow(t, width)

	note := "Deploy is blocked until the migration is approved"
	m = typing(t, press1(t, m, "n"), note)

	view := plain(m.View())
	for _, line := range strings.Split(view, "\n") {
		if over := lipgloss.Width(line) - width; over > 0 {
			t.Fatalf("a line runs %d columns past the %d-column frame:\n%s", over, width, view)
		}
	}
	if joined := strings.Join(strings.Fields(view), " "); !strings.Contains(joined, note) {
		t.Errorf("the wrapped answer is not all on screen:\n%s", view)
	}
}

// The prompt takes exactly the rows its answer wraps into: it grows a row when the answer does and
// reserves none it is not using, so the list above it keeps every row it can.
func TestPromptTakesOnlyTheRowsItsAnswerNeeds(t *testing.T) {
	m, _ := narrow(t, 60)

	m = press1(t, m, "n")
	if got := lines(plain(m.promptLine())); got != 1 {
		t.Errorf("an empty prompt took %d rows, want 1", got)
	}

	m = typing(t, m, "Deploy is blocked until the migration is approved")
	if got := lines(plain(m.promptLine())); got != 2 {
		t.Errorf("an answer wrapping onto a second row took %d rows, want 2", got)
	}
}

// press1 presses one key and hands back the model, for the keys opening the prompt a test then types
// into.
func press1(t *testing.T, m Model, k string) Model {
	t.Helper()

	m, _ = press(t, m, k)
	return m
}

// The keys that move and cut inside the answer, each pressed against the same starting text so the
// table reads as the edit it makes rather than as the branch it takes.
func TestPromptEditingKeys(t *testing.T) {
	const start = "one two three"

	for _, c := range []struct {
		name  string
		keys  []string
		after string
	}{
		{"left inserts before the last character", []string{"left", "X"}, "one two threXe"},
		{"right after a left lands back at the end", []string{"left", "right", "X"}, "one two threeX"},
		{"ctrl-a goes to the start", []string{"ctrl+a", "X"}, "Xone two three"},
		{"home goes to the start", []string{"home", "X"}, "Xone two three"},
		{"ctrl-e comes back to the end", []string{"ctrl+a", "ctrl+e", "X"}, "one two threeX"},
		{"end comes back to the end", []string{"home", "end", "X"}, "one two threeX"},
		{"ctrl-left jumps a word back", []string{"ctrl+left", "X"}, "one two Xthree"},
		{"ctrl-right jumps a word forward", []string{"home", "ctrl+right", "X"}, "oneX two three"},
		{"two ctrl-lefts clear two words", []string{"ctrl+left", "ctrl+left", "X"}, "one Xtwo three"},
		{"alt-left jumps a word back", []string{"alt+left", "X"}, "one two Xthree"},
		{"alt-b jumps a word back", []string{"alt+b", "X"}, "one two Xthree"},
		{"two word jumps clear two words", []string{"alt+left", "alt+left", "X"}, "one Xtwo three"},
		{"alt-right jumps a word forward", []string{"home", "alt+right", "X"}, "oneX two three"},
		{"alt-f jumps a word forward", []string{"home", "alt+f", "X"}, "oneX two three"},
		{"ctrl-w cuts the word behind the cursor", []string{"ctrl+w"}, "one two "},
		{"ctrl-k cuts to the end of the line", []string{"alt+left", "ctrl+k"}, "one two "},
		{"ctrl-u cuts back to the start", []string{"ctrl+u"}, ""},
		{"delete takes the character in front", []string{"home", "delete"}, "ne two three"},
		{"backspace still takes the one behind", []string{"backspace"}, "one two thre"},
	} {
		m, _ := working(t)
		m = typing(t, press1(t, m, "n"), start)

		m, _ = press(t, m, c.keys...)
		if got := m.prompt.text(); got != c.after {
			t.Errorf("%s: left %q, want %q", c.name, got, c.after)
		}
	}
}

// The key table advertises what a prompt can do with the text in it, since the keys that move and cut
// inside an answer are bound exactly while nothing else is.
func TestKeyTableListsThePromptEditingKeys(t *testing.T) {
	m, _ := working(t)

	table := plain(press1(t, m, "?").View())
	typing := table[strings.Index(table, "while typing"):]
	for _, want := range []string{"← →", "ctrl-a / ctrl-e", "ctrl-← / ctrl-→", "ctrl-w", "ctrl-k", "delete"} {
		if !strings.Contains(typing, want) {
			t.Errorf("the key table does not offer %q while typing:\n%s", want, table)
		}
	}
}

// The first line of a wrapping answer stays in view as the answer grows past it. The input scrolls
// its own view only while handling a key, so it is given the height the answer may take before the
// key lands rather than after.
func TestPromptKeepsTheFirstLineOfAWrappedAnswerInView(t *testing.T) {
	m, _ := narrow(t, 60)

	m = typing(t, press1(t, m, "n"), "Deploy is blocked until the migration is approved")

	first := strings.Split(plain(m.promptLine()), "\n")[0]
	if !strings.Contains(first, "note") || !strings.Contains(first, "Deploy is blocked") {
		t.Errorf("the answer wrapped its own first line out of view, leaving %q", first)
	}
}

// An answer longer than the cap scrolls inside it rather than eating the list: the prompt stops at
// promptMaxHeight rows and keeps the cursor on screen.
func TestPromptStopsGrowingAtItsCap(t *testing.T) {
	m, _ := narrow(t, 40)

	m = typing(t, press1(t, m, "n"), strings.Repeat("blocked on the platform team ", 12))

	if got := lines(plain(m.promptLine())); got != promptMaxHeight {
		t.Errorf("an answer past the cap took %d rows, want it held to %d", got, promptMaxHeight)
	}

	m = typing(t, m, "END")
	if last := plain(m.promptLine()); !strings.Contains(last, "END") {
		t.Errorf("the cursor scrolled out of the capped prompt:\n%s", last)
	}
}

// Narrowing the terminal rewraps an answer already typed, and the cursor stays with it: a resize can
// push the end of a long answer past the cap, which is a scroll the input has to be asked to make.
func TestPromptFollowsTheCursorWhenTheTerminalNarrows(t *testing.T) {
	m, _ := narrow(t, 110)

	m = typing(t, press1(t, m, "n"), strings.Repeat("blocked on the platform team ", 8)+"END")
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 34, Height: 24})
	m = resized.(Model)

	if view := plain(m.promptLine()); !strings.Contains(view, "END") {
		t.Errorf("narrowing left the cursor scrolled out of the prompt:\n%s", view)
	}
}
