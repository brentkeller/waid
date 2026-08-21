package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/brentkeller/waid/internal/detect"
)

// key builds the message a keypress arrives as, so the tests press keys the way a terminal sends
// them rather than by naming the branch they want.
func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// press feeds keys through Update in order and hands back the model they left, with the command the
// last of them produced.
func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()

	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		next, cmd = m.Update(key(k))
		m = next.(Model)
	}
	return m, cmd
}

// chrome is a model that has been told how big its terminal is, which every view assertion needs.
func chrome(t *testing.T, width int) Model {
	t.Helper()

	m, _ := New(Options{}).Update(tea.WindowSizeMsg{Width: width, Height: 24})
	return m.(Model)
}

var escapes = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

// plain strips the styling, so an assertion about what a view says does not depend on the colour
// profile of whatever the test binary is attached to.
func plain(view string) string { return escapes.ReplaceAllString(view, "") }

// The digits address the tabs directly, in the order the bar draws them (§4).
func TestDigitKeysSelectTheTabsDirectly(t *testing.T) {
	cases := map[string]tab{"1": tabLoops, "2": tabScan, "3": tabReview}

	for pressed, want := range cases {
		m, _ := press(t, New(Options{}), pressed)
		if m.tab != want {
			t.Errorf("%q selected %s, want %s", pressed, tabTitles[m.tab], tabTitles[want])
		}
	}
}

// tab walks the bar and wraps; shift+tab walks it the other way.
func TestTabCyclesThroughTheTabs(t *testing.T) {
	m := New(Options{})
	for _, want := range []tab{tabScan, tabReview, tabLoops} {
		m, _ = press(t, m, "tab")
		if m.tab != want {
			t.Fatalf("tab landed on %s, want %s", tabTitles[m.tab], tabTitles[want])
		}
	}

	for _, want := range []tab{tabReview, tabScan, tabLoops} {
		m, _ = press(t, m, "shift+tab")
		if m.tab != want {
			t.Fatalf("shift+tab landed on %s, want %s", tabTitles[m.tab], tabTitles[want])
		}
	}
}

// Both quit keys have to reach tea.Quit; ctrl-c especially, since it is the one a user reaches for
// when the app is misbehaving.
func TestQuitKeys(t *testing.T) {
	for _, pressed := range []string{"q", "ctrl+c"} {
		_, cmd := press(t, New(Options{}), pressed)
		if cmd == nil {
			t.Fatalf("%q produced no command, want quit", pressed)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%q produced %T, want tea.QuitMsg", pressed, cmd())
		}
	}
}

// The key table covers the globals and the keys of the tab it is opened on, and the same key closes
// it again.
func TestQuestionTogglesTheKeyTable(t *testing.T) {
	m := chrome(t, 100)
	m, _ = press(t, m, "2")

	if got := plain(m.View()); strings.Contains(got, "switch tab") {
		t.Fatalf("the key table is showing before ? was pressed:\n%s", got)
	}

	m, _ = press(t, m, "?")
	view := plain(m.View())
	for _, want := range []string{"switch tab", "quit", "promote", "dismiss"} {
		if !strings.Contains(view, want) {
			t.Errorf("the key table does not mention %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "edit title") {
		t.Errorf("the key table on Scan lists a Loops key:\n%s", view)
	}

	m, _ = press(t, m, "?")
	if got := plain(m.View()); strings.Contains(got, "switch tab") {
		t.Errorf("? did not close the key table:\n%s", got)
	}
}

// / opens the filter, the keys that follow are text rather than commands, and esc clears it (§4).
func TestSlashFiltersAndEscClears(t *testing.T) {
	m := chrome(t, 100)
	m, _ = press(t, m, "/", "t", "u", "i")

	if !m.filtering || m.filter != "tui" {
		t.Fatalf("after /tui the filter is %q (editing: %v), want %q while editing", m.filter, m.filtering, "tui")
	}
	if got := plain(m.View()); !strings.Contains(got, "/tui") {
		t.Errorf("the filter is not visible in the view:\n%s", got)
	}

	typed, cmd := press(t, m, "q")
	if cmd != nil {
		t.Errorf("q typed into the filter produced a command, want it treated as text")
	}
	if typed.filter != "tuiq" {
		t.Errorf("filter = %q after typing q, want %q", typed.filter, "tuiq")
	}

	backspaced, _ := press(t, typed, "backspace")
	if backspaced.filter != "tui" {
		t.Errorf("filter = %q after backspace, want %q", backspaced.filter, "tui")
	}

	accepted, _ := press(t, backspaced, "enter")
	if accepted.filtering || accepted.filter != "tui" {
		t.Errorf("enter left filter %q (editing: %v), want %q applied and no longer editing",
			accepted.filter, accepted.filtering, "tui")
	}

	cleared, _ := press(t, accepted, "esc")
	if cleared.filtering || cleared.filter != "" {
		t.Errorf("esc left filter %q (editing: %v), want it cleared", cleared.filter, cleared.filtering)
	}
}

// A key with no meaning where it was pressed is inert and says why in the footer, rather than doing
// nothing silently (§4).
func TestInertKeyLeavesAFooterHint(t *testing.T) {
	m := chrome(t, 100)
	m, _ = press(t, m, "2")

	owned, _ := press(t, m, "w")
	if owned.hint == "" {
		t.Fatal("w on Scan left no hint, want one naming the tab that owns it")
	}
	if !strings.Contains(owned.hint, "Loops") {
		t.Errorf("hint for w on Scan is %q, want it to name Loops", owned.hint)
	}
	if got := plain(owned.View()); !strings.Contains(got, owned.hint) {
		t.Errorf("the hint is not in the footer:\n%s", got)
	}

	unbound, _ := press(t, owned, "z")
	if !strings.Contains(unbound.hint, "Repos") {
		t.Errorf("hint for an unbound key is %q, want it to name the tab it was pressed on", unbound.hint)
	}

	acted, _ := press(t, unbound, "s")
	if acted.hint != "" {
		t.Errorf("a key bound on the tab left the hint %q, want it cleared", acted.hint)
	}
}

// The bar carries the tab names, the count each tab has, and the progress slot that is the only
// place slow work is ever visible (§1).
func TestTabBarShowsTheTabsTheirCountsAndProgress(t *testing.T) {
	m := chrome(t, 140)
	m.counts[tabLoops], m.counts[tabScan] = 12, 8
	m.progress = "scanning  5/9 repos"

	bar := strings.Split(plain(m.View()), "\n")[1]
	for _, want := range []string{"Loops  12", "Repos  8", "Agents", m.progress} {
		if !strings.Contains(bar, want) {
			t.Errorf("the tab bar does not carry %q:\n%s", want, bar)
		}
	}
	if got := lipgloss.Width(bar); got > 140 {
		t.Errorf("the tab bar is %d columns wide, want no more than 140", got)
	}
}

// Golden views of the chrome at the two widths §8 names, driven through a real program so the
// snapshot is of what a terminal would have been handed.
func TestGoldenChromeAt80Columns(t *testing.T) { goldenChrome(t, 80) }

func TestGoldenChromeAt140Columns(t *testing.T) { goldenChrome(t, 140) }

func goldenChrome(t *testing.T, width int) {
	t.Helper()

	// The detection seam is stubbed and the pass is delivered explicitly, so the snapshot does not
	// depend on whether the one Init issued landed before the program was told to quit.
	tm := teatest.NewTestModel(t, offline(New(Options{}), detect.Result{}), teatest.WithInitialTermSize(width, 24))
	tm.Send(key("2"))
	tm.Send(scanLoadedMsg{at: scannedAt})
	tm.Send(key("q"))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(Model)
	teatest.RequireEqualOutput(t, []byte(plain(final.View())))
}

// The window title names the live tab, so a `waid ui` window is identifiable from a terminal's tab
// strip without switching to it.
func TestSwitchingTabsRetitlesTheWindow(t *testing.T) {
	_, cmd := press(t, New(Options{}), "2")

	if cmd == nil {
		t.Fatal("switching tabs set no window title")
	}
	if got, want := fmt.Sprint(cmd()), "waid · Repos"; got != want {
		t.Errorf("window title = %q, want %q", got, want)
	}
}

// A key that leaves the tab where it was leaves the title alone, so the terminal is not retitled on
// every press.
func TestKeysThatDoNotSwitchTabsLeaveTheTitleAlone(t *testing.T) {
	if _, cmd := press(t, New(Options{}), "2", "2"); cmd != nil {
		t.Errorf("pressing the live tab's digit produced %v, want no command", cmd())
	}
}

// The title is set from Init, so the window is named before the first keypress.
func TestTheAppTitlesTheWindowOnStart(t *testing.T) {
	batch, ok := New(Options{}).Init()().(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init did not batch its commands")
	}
	if got, want := fmt.Sprint(batch[0]()), "waid · Loops"; got != want {
		t.Errorf("window title = %q, want %q", got, want)
	}
}

// A terminal too narrow for every hint loses them from the end rather than spilling the line past the
// frame, and the way to the full key table survives whatever else goes.
func TestFooterHintsFitTheTerminal(t *testing.T) {
	m := chrome(t, 80)

	line := plain(m.footer())
	hints := strings.Split(line, "\n")[1]

	if got := lipgloss.Width(hints); got > 80 {
		t.Errorf("the hints are %d columns wide at 80, want them inside the frame:\n%s", got, hints)
	}
	if !strings.HasSuffix(strings.TrimRight(hints, " "), "? keys") {
		t.Errorf("the hints are %q, want the key table still offered", hints)
	}
	if !strings.Contains(hints, "x done") {
		t.Errorf("the hints are %q, want the first of them kept", hints)
	}
}

// A terminal with room for every hint keeps every hint.
func TestFooterHintsAreWholeWhenTheyFit(t *testing.T) {
	m := chrome(t, 140)

	hints := strings.Split(plain(m.footer()), "\n")[1]
	for _, want := range []string{"x done", "P parent", "s status", "? keys"} {
		if !strings.Contains(hints, want) {
			t.Errorf("the hints are %q, want %q kept at 140 columns", hints, want)
		}
	}
}
