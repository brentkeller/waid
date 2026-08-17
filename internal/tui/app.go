// Package tui is the terminal app `waid ui` runs: a long-running view over the same data the
// commands print, with triage that writes as you go. Nothing outside this package holds a terminal
// concern, and nothing in it is imported by a domain package.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/ids"
)

// Options are what the app needs from the CLI: the resolved configuration, and the clock and id
// generator dispatch already resolved, so the app draws both from the same places every command does.
type Options struct {
	Cfg config.Config
	Now time.Time
	Ids ids.Generator
}

// Run starts the app on the alternate screen and blocks until it quits. Bubble Tea restores the
// terminal from a deferred call, so a panic mid-render still leaves the alt screen before the stack
// reaches the restored one.
func Run(opts Options) error {
	_, err := tea.NewProgram(New(opts), tea.WithAltScreen()).Run()
	return err
}

// tab is one of the three sections the app switches between. They are tabs rather than lazygit's
// fixed panes because the sections are modes of a workday, not a drill-down hierarchy (§1).
type tab int

const (
	tabLoops tab = iota
	tabScan
	tabReview
)

const numTabs = 3

var tabTitles = [numTabs]string{"Loops", "Scan", "Review"}

// tabFooters are the key hints each tab keeps in the footer — the keys worth having in front of you
// while working the tab, as opposed to the full table `?` opens.
var tabFooters = [numTabs]string{
	tabLoops:  "x done · w waiting · e edit · n note · a add · / filter · s status · ? keys",
	tabScan:   "p promote · d dismiss · o open in browser · r refresh · / filter · ? keys",
	tabReview: "space preview · R resume in claude · o open repo · y copy id · ? keys",
}

// tabEmpty is what a tab says when it has nothing to show.
var tabEmpty = [numTabs]string{"no items", "no signals", "no sessions"}

// binding is one row of the key table (§4): the keys that trigger it, how the table prints them,
// and what pressing one does. The keys are what decides whether a press is inert, so the table is
// the single source for both the help and the behaviour.
type binding struct {
	keys   []string
	label  string
	effect string
}

// globalBindings apply on every tab.
var globalBindings = []binding{
	{[]string{"1", "2", "3", "tab", "shift+tab"}, "1 2 3 / tab", "switch tab"},
	{[]string{"j", "k", "up", "down"}, "j k ↑ ↓", "move the cursor, skipping headings"},
	{[]string{"g", "G"}, "g / G", "first / last row"},
	{[]string{"enter"}, "enter", "expand or collapse the fold under the cursor"},
	{[]string{"/", "esc"}, "/", "filter; esc clears"},
	{[]string{"a"}, "a", "add an item, from any tab"},
	{[]string{"r"}, "r", "refresh the current tab"},
	{[]string{"u"}, "u", "undo the last write"},
	{[]string{"?"}, "?", "key table"},
	{[]string{"q", "ctrl+c"}, "q / ctrl-c", "quit"},
}

// tabBindings are the keys that mean something on one tab only. A key listed here and pressed
// elsewhere is inert, and the footer names the tab that owns it.
var tabBindings = [numTabs][]binding{
	tabLoops: {
		{[]string{"x"}, "x", "done"},
		{[]string{"w"}, "w", "waiting"},
		{[]string{"e"}, "e", "edit title"},
		{[]string{"n"}, "n", "note"},
		{[]string{"s"}, "s", "cycle the status filter"},
		{[]string{"p"}, "p", "toggle the detail pane"},
	},
	tabScan: {
		{[]string{"p"}, "p", "promote"},
		{[]string{"d"}, "d", "dismiss"},
		{[]string{"o"}, "o", "open in browser"},
	},
	tabReview: {
		{[]string{" "}, "space", "preview"},
		{[]string{"R"}, "R", "resume in claude"},
		{[]string{"o"}, "o", "open repo"},
		{[]string{"y"}, "y", "copy the session id"},
	},
}

// Model is the root model: the tabs, the focus, the global keys and the undo stack all hang off it.
type Model struct {
	opts   Options
	theme  Theme
	width  int
	height int

	// tab is the focus: exactly one section is live, and the global keys act on it.
	tab tab

	// counts are the badges on the tab labels, and progress is the bar's right-hand slot — the one
	// place slow work is visible, since detection runs off the update loop (§1).
	counts   [numTabs]int
	progress string

	// filter is the query typed after `/`; filtering is true only while it is being edited, so the
	// keys arriving in between are text rather than commands.
	filter    string
	filtering bool

	// showKeys is the key table `?` opens over the list, and hint is the reason an inert key left
	// behind for the footer.
	showKeys bool
	hint     string
}

// New builds the root model.
func New(opts Options) Model { return Model{opts: opts, theme: NewTheme()} }

func (m Model) Init() tea.Cmd { return nil }

// Update is pure: every read runs as a tea.Cmd off the update loop and returns a message, so
// Update never waits on the filesystem or the network.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

// key routes a keypress. While the filter is being edited every key but the two that end the edit
// is text, so a query containing `q` cannot quit the app out from under the person typing it.
func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering && msg.Type != tea.KeyCtrlC {
		return m.filterKey(msg), nil
	}

	m.hint = ""
	pressed := msg.String()
	switch pressed {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "1", "2", "3":
		m.tab, m.showKeys = tab(pressed[0]-'1'), false
	case "tab":
		m.tab, m.showKeys = (m.tab+1)%numTabs, false
	case "shift+tab":
		m.tab, m.showKeys = (m.tab+numTabs-1)%numTabs, false
	case "?":
		m.showKeys = !m.showKeys
	case "/":
		m.filtering = true
	case "esc":
		m.showKeys, m.filter = false, ""
	default:
		if !boundOn(m.tab, pressed) {
			m.hint = inertHint(m.tab, pressed)
		}
	}
	return m, nil
}

// filterKey edits the query. esc abandons it, enter applies it and hands the keyboard back.
func (m Model) filterKey(msg tea.KeyMsg) Model {
	switch msg.Type {
	case tea.KeyEsc:
		m.filtering, m.filter = false, ""
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyBackspace:
		if runes := []rune(m.filter); len(runes) > 0 {
			m.filter = string(runes[:len(runes)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		m.filter += msg.String()
	}
	return m
}

// boundOn reports whether the key does something where it was pressed.
func boundOn(t tab, pressed string) bool {
	return bound(globalBindings, pressed) || bound(tabBindings[t], pressed)
}

func bound(bindings []binding, pressed string) bool {
	for _, b := range bindings {
		for _, k := range b.keys {
			if k == pressed {
				return true
			}
		}
	}
	return false
}

// inertHint is what the footer says about a key that did nothing. A key another tab owns names that
// tab and how to reach it; anything else names the tab it was pressed on, so the answer is never
// just silence (§4).
func inertHint(current tab, pressed string) string {
	for t := range numTabs {
		if bound(tabBindings[t], pressed) {
			return fmt.Sprintf("%s is a %s key — press %d to switch", keyLabel(pressed), tabTitles[t], t+1)
		}
	}
	return fmt.Sprintf("%s is not bound on %s", keyLabel(pressed), tabTitles[current])
}

// keyLabel prints a key the way the key table does, so a hint about the space bar does not read as
// a hint about nothing.
func keyLabel(pressed string) string {
	if pressed == " " {
		return "space"
	}
	return pressed
}

func (m Model) View() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}

	bar := m.tabBar(width)
	divider := m.theme.Divider.Render(strings.Repeat("─", width))
	footer := m.footer()

	body := m.body()
	if height > 0 {
		body = padLines(body, height-lines(bar)-lines(divider)-lines(footer))
	}

	return strings.Join([]string{bar, body, divider, footer}, "\n")
}

// tabBar is the three lines at the top: the cap over the active tab, the labels, and the rule the
// list hangs from. The right-hand slot rides on the label line (§1).
func (m Model) tabBar(width int) string {
	var bar strings.Builder
	bar.WriteString(m.theme.TabRule.Render("│"))

	separators := []int{0}
	column := 1
	activeStart, activeEnd := 0, 0
	for i := range numTabs {
		cell := m.tabCell(tab(i))

		style := m.theme.TabInactive
		if tab(i) == m.tab {
			style, activeStart = m.theme.TabActive, column-1
		}
		bar.WriteString(style.Render(cell))
		column += lipgloss.Width(cell)

		bar.WriteString(m.theme.TabRule.Render("│"))
		separators = append(separators, column)
		if tab(i) == m.tab {
			activeEnd = column
		}
		column++
	}

	labels := bar.String()
	if gap := width - column - lipgloss.Width(m.progress); m.progress != "" && gap >= 2 {
		labels += strings.Repeat(" ", gap) + m.theme.Progress.Render(m.progress)
	}

	top := strings.Repeat(" ", activeStart) + "╭" + strings.Repeat("─", activeEnd-activeStart-1) + "╮"

	rule := []rune(strings.Repeat("─", width))
	for _, at := range separators {
		if at < len(rule) {
			rule[at] = '┴'
		}
	}

	return m.theme.TabRule.Render(top) + "\n" + labels + "\n" + m.theme.TabRule.Render(string(rule))
}

// tabCell is one label, with the count the tab is carrying when it has one.
func (m Model) tabCell(t tab) string {
	if n := m.counts[t]; n > 0 {
		return fmt.Sprintf(" %s  %d ", tabTitles[t], n)
	}
	return " " + tabTitles[t] + " "
}

// body is what sits between the bar and the footer: the key table when it is open, otherwise the
// active tab's list.
func (m Model) body() string {
	if m.showKeys {
		return m.keyTable()
	}
	return m.theme.Dim.Render("  " + tabEmpty[m.tab])
}

// keyTable prints the globals and the active tab's keys in two labelled sections, so the keys that
// vary with the tab are visibly the ones that vary.
func (m Model) keyTable() string {
	sections := []struct {
		title    string
		bindings []binding
	}{
		{"global", globalBindings},
		{tabTitles[m.tab], tabBindings[m.tab]},
	}

	label := 0
	for _, section := range sections {
		for _, b := range section.bindings {
			label = max(label, lipgloss.Width(b.label))
		}
	}

	var out strings.Builder
	for i, section := range sections {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString("  " + m.theme.Heading.Render(section.title) + "\n")
		for _, b := range section.bindings {
			gap := strings.Repeat(" ", label-lipgloss.Width(b.label)+2)
			out.WriteString("    " + m.theme.RowFocused.Render(b.label) + gap + m.theme.Row.Render(b.effect) + "\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

// footer is two lines: the status line, where the filter being typed and the reason for an inert
// key take turns, and the tab's key hints. It keeps its height either way so the list above it does
// not shift as messages come and go.
func (m Model) footer() string {
	status := ""
	switch {
	case m.filtering:
		status = m.theme.FilterActive.Render(" /" + m.filter + "▏")
	case m.filter != "":
		status = m.theme.FilterActive.Render(" /"+m.filter) + m.theme.Dim.Render("  esc clears")
	case m.hint != "":
		status = m.theme.Dim.Render(" " + m.hint)
	}
	return status + "\n" + m.theme.Footer.Render(" "+tabFooters[m.tab])
}

// lines counts the printed lines in a rendered block.
func lines(s string) int { return strings.Count(s, "\n") + 1 }

// padLines grows a block to fill the rows it has been given, which is what keeps the footer on the
// bottom row of the alternate screen.
func padLines(s string, want int) string {
	if grow := want - lines(s); grow > 0 {
		return s + strings.Repeat("\n", grow)
	}
	return s
}
