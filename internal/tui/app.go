// Package tui is the terminal app `waid ui` runs: a long-running view over the same data the
// commands print, with triage that writes as you go. Nothing outside this package holds a terminal
// concern, and nothing in it is imported by a domain package.
package tui

import (
	"fmt"
	"os"
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
//
// The receipts are replayed once the terminal is back, which is what leaves a triage pass in real
// scrollback rather than losing it with the screen (§3.1).
func Run(opts Options) error {
	// The app titles the window after the tab it is on, so the title the shell left is pushed to the
	// terminal's title stack and popped back on the way out. Terminals without the stack ignore both
	// and keep the last title the app set, which is what they would have done anyway.
	fmt.Fprint(os.Stdout, "\x1b[22;2t")
	defer fmt.Fprint(os.Stdout, "\x1b[23;2t")

	final, err := start(New(opts))
	if m, ok := final.(Model); ok {
		replayTo(os.Stdout, m.receipts)
	}
	return err
}

// start runs a model on the alternate screen until it quits. The extra options are the seam the
// panic test drives the same program through without a terminal to attach to.
func start(model tea.Model, extra ...tea.ProgramOption) (tea.Model, error) {
	return tea.NewProgram(model, append([]tea.ProgramOption{tea.WithAltScreen()}, extra...)...).Run()
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

var tabTitles = [numTabs]string{"Loops", "Repos", "Agents"}

// tabFooters are the key hints each tab keeps in the footer — the keys worth having in front of you
// while working the tab, as opposed to the full table `?` opens.
var tabFooters = [numTabs]string{
	tabLoops:  "x done · w waiting · e edit · P project · n note · a add · / filter · s status · ? keys",
	tabScan:   "p promote · d dismiss · o open in browser · r refresh · / filter · ? keys",
	tabReview: "space preview · R resume · o open repo · y copy id · s range · d date · ? keys",
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
	// The digits are the bar's on every tab. §1.1 also offers them to the Loops status row, but a digit
	// that switches tabs on two tabs and jumps a filter on the third is a mode to keep track of, and the
	// segmented rows Repos and Agents draw have no digits either — `s` cycles all three.
	{[]string{"1", "2", "3", "tab", "shift+tab"}, "1 2 3 / tab", "switch tab, from any tab"},
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

// promptBindings are the keys that mean something only while a prompt is taking text. They are a
// section of their own rather than globals: a prompt holds the keyboard, so these are the keys that
// are bound exactly when nothing else is.
var promptBindings = []binding{
	{[]string{"enter"}, "enter", "commit the answer"},
	{[]string{"esc"}, "esc", "abandon it"},
	{[]string{"ctrl+u"}, "ctrl-u", "empty the input, leaving the prompt open"},
	{[]string{"tab", "shift+tab"}, "tab / shift-tab", "move through a project search's matches"},
}

// tabBindings are the keys that mean something on one tab only. A key listed here and pressed
// elsewhere is inert, and the footer names the tab that owns it.
var tabBindings = [numTabs][]binding{
	tabLoops: {
		{[]string{"x"}, "x", "done"},
		{[]string{"w"}, "w", "waiting"},
		{[]string{"e"}, "e", "edit title"},
		{[]string{"n"}, "n", "note"},
		{[]string{"P"}, "P", "file under a project"},
		{[]string{"s"}, "s", "cycle the status filter — open, waiting, done, all"},
		{[]string{"p"}, "p", "toggle the detail pane"},
	},
	tabScan: {
		{[]string{"p"}, "p", "promote"},
		{[]string{"d"}, "d", "dismiss"},
		{[]string{"o"}, "o", "open in browser"},
		{[]string{"s"}, "s", "cycle the kind filter"},
		{[]string{"e"}, "e", "rename the item just promoted"},
	},
	tabReview: {
		{[]string{" "}, "space", "preview"},
		{[]string{"pgup", "pgdown"}, "pgup / pgdn", "scroll the open preview"},
		{[]string{"home", "end"}, "home / end", "top / bottom of the transcript"},
		{[]string{"R"}, "R", "resume in claude"},
		{[]string{"o"}, "o", "open repo"},
		{[]string{"y"}, "y", "copy the session id"},
		{[]string{"d"}, "d", "pick a calendar day"},
		{[]string{"s"}, "s", "cycle the range"},
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

	// counts are the badges on the tab labels, and progress is the work in flight — the one place slow
	// work is visible, since detection runs off the update loop (§1). It is what the bar's right-hand
	// slot shows while there is any; barSlot decides what shows there otherwise.
	counts   [numTabs]int
	progress string

	// spinner is the frame the progress slot is showing. It is the root model's rather than a tab's,
	// since detection and the history sync turn the same one.
	spinner int

	// filter is the query typed after `/`; filtering is true only while it is being edited, so the
	// keys arriving in between are text rather than commands.
	filter    string
	filtering bool

	// showKeys is the key table `?` opens over the list, and hint is the reason an inert key left
	// behind for the footer.
	showKeys bool
	hint     string

	// prompt is the inline input open in the footer, if any; its zero value is no prompt.
	prompt prompt

	// undos are the inverses of the writes this session made, newest last and bounded (§3).
	undos []undoEntry

	// reversible is whether the last write left an inverse behind. A note does not — the log holds no
	// event that removes one — so the footer stops offering the undo rather than offering one that
	// would reverse the write before it.
	reversible bool

	// receipts are the writes this session made, in order. The last one is what the footer shows,
	// and the whole log is replayed to the restored terminal on quit (§3.1).
	receipts []receipt

	// loops is the Loops tab's own state: the folded log and the tree over it.
	loops loopsModel

	// scan is the Repos tab's own state: the last good detection pass and the filters over it.
	scan scanModel

	// review is the Agents tab's own state: the harvested history and the window over it.
	review reviewModel

	// clock is the app's present. It advances while the app runs, so the age of what is on screen
	// keeps counting; Options.Now is only the instant the app started.
	clock func() time.Time
}

// New builds the root model with the first detection pass already claimed, so the tab bar is turning
// from the moment the program starts rather than from the moment the pass is issued.
func New(opts Options) Model {
	m := Model{opts: opts, theme: NewTheme(), clock: time.Now}
	m.loops.load = loopsLoader(opts)
	m.scan.load = scanLoader(opts)
	m.scan.loading = true
	m.progress = m.spinnerText()
	m.review.load = reviewLoader(opts)
	m.review.readTurns = readTranscript
	return m
}

// Init issues the first reads. Nothing loads inside New, so a model can be built and inspected
// without touching the filesystem, git or the network.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.SetWindowTitle(m.windowTitle())}
	if m.loops.load != nil {
		cmds = append(cmds, m.loops.load)
	}
	if m.scan.load != nil {
		cmds = append(cmds, m.scan.load, spinnerTick())
	}
	if m.review.load != nil {
		// The first history read is incremental: it re-parses the transcripts written since the last sync,
		// which is what makes the tab open on the sessions run since a shell last ran a command.
		cmds = append(cmds, reviewCmd(m.review.load, false))
	}
	return tea.Batch(cmds...)
}

// now is the instant the views measure ages against.
func (m Model) now() time.Time {
	if m.clock == nil {
		return time.Now()
	}
	return m.clock()
}

// viewWidth is the columns the views lay out in, standing in a default until the first resize
// arrives.
func (m Model) viewWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width
}

// Update is pure: every read runs as a tea.Cmd off the update loop and returns a message, so
// Update never waits on the filesystem or the network.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.key(msg)
	case loopsLoadedMsg:
		return m.loopsLoaded(msg), nil
	case scanLoadedMsg:
		return m.scanLoaded(msg), nil
	case reviewLoadedMsg:
		return m.reviewLoaded(msg)
	case previewLoadedMsg:
		return m.previewLoaded(msg), nil
	case resumedMsg:
		return m.resumed(msg)
	case copiedMsg:
		return m.copied(msg), nil
	case spinnerTickMsg:
		// The spinner stops with the work it is reporting, so nothing ticks while the app is idle.
		if m.progressText() == "" {
			return m, nil
		}
		m.spinner++
		m.progress = m.spinnerText()
		return m, spinnerTick()
	}
	return m, nil
}

// key routes a keypress. While text is being edited — a filter query or a prompt's answer — every key
// but the two that end the edit is text, so a title containing `q` cannot quit the app out from under
// the person typing it. ctrl-c is the exception and quits from anywhere.
func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type != tea.KeyCtrlC {
		if m.prompt.kind != promptNone {
			return m.promptKey(msg)
		}
		if m.filtering {
			return m.filterKey(msg), nil
		}
	}

	m.hint = ""
	pressed := msg.String()
	was := m.tab
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
	case "a":
		return m.addPrompt()
	case "u":
		return m.undo()
	case "esc":
		m.showKeys, m.filter = false, ""
	default:
		if next, cmd, handled := m.tabKey(pressed); handled {
			return next, cmd
		}
		if !boundOn(m.tab, pressed) {
			m.hint = inertHint(m.tab, pressed)
		}
	}
	if m.tab != was {
		return m, tea.SetWindowTitle(m.windowTitle())
	}
	return m, nil
}

// windowTitle names the live tab, which is what a terminal shows in a tab strip or window list.
func (m Model) windowTitle() string { return "waid · " + tabTitles[m.tab] }

// tabKey offers a press to the live tab. It reports whether the tab dealt with the key, which
// includes a key that was inert for a reason the tab left in the footer (§4).
func (m Model) tabKey(pressed string) (Model, tea.Cmd, bool) {
	switch m.tab {
	case tabLoops:
		return m.loopsKey(pressed)
	case tabScan:
		return m.scanKey(pressed)
	case tabReview:
		return m.reviewKey(pressed)
	}
	return m, nil, false
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
	width := m.viewWidth()

	bar := m.tabBar(width)
	divider := m.theme.Divider.Render(strings.Repeat("─", width))
	footer := m.footer()

	rows := m.bodyRows()
	body := m.body(width, rows)
	if rows > 0 {
		body = padLines(body, rows)
	}

	return strings.Join([]string{bar, body, divider, footer}, "\n")
}

// bodyRows is the rows the chrome leaves between the tab bar and the footer. They are measured rather
// than assumed, since a view that fills them — the preview — has to know how many it has before it
// draws, and the keys that page it have to agree with what was drawn. It is zero until the terminal
// says how tall it is.
func (m Model) bodyRows() int {
	if m.height <= 0 {
		return 0
	}
	return m.height - lines(m.tabBar(m.viewWidth())) - 1 - lines(m.footer())
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
	if slot := m.barSlot(); slot != "" {
		if gap := width - column - lipgloss.Width(slot); gap >= 2 {
			labels += strings.Repeat(" ", gap) + m.theme.Progress.Render(slot)
		}
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
// active tab's list, in the rows the chrome left it.
func (m Model) body(width, height int) string {
	if m.showKeys {
		return m.keyTable()
	}
	switch m.tab {
	case tabLoops:
		return m.loopsBody(width, height)
	case tabScan:
		return m.scanBody(width)
	case tabReview:
		return m.reviewBody(width, height)
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
		{"while typing", promptBindings},
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

// footer is two lines: the status line and the tab's key hints. It keeps its height whether or not
// it has anything to say, so the list above it does not shift as messages come and go.
func (m Model) footer() string {
	// A project search puts its matches where the hints go, since the hints are for keys and the
	// search is asking which of several projects the answer means. It is the one prompt that takes
	// more than the status line, and the body is measured off this, so the list simply gives way.
	if m.prompt.kind == promptProject {
		return m.status() + "\n" + m.projectChoices()
	}
	return m.status() + "\n" + m.theme.Footer.Render(" "+fitHints(tabFooters[m.tab], m.viewWidth()-1))
}

// fitHints drops hints from a line too long for the terminal, so a narrow window loses the tail of
// the list rather than spilling it past the frame. The last hint is the way to the full key table and
// is kept whatever else goes, since it is what makes the dropped ones reachable.
func fitHints(hints string, width int) string {
	parts := strings.Split(hints, hintSeparator)
	if len(parts) < 2 {
		return hints
	}

	kept, last := parts[:len(parts)-1], parts[len(parts)-1]
	for len(kept) > 1 {
		line := strings.Join(append(kept, last), hintSeparator)
		if lipgloss.Width(line) <= width {
			return line
		}
		kept = kept[:len(kept)-1]
	}
	return strings.Join([]string{kept[0], last}, hintSeparator)
}

// hintSeparator is what the footer puts between two key hints.
const hintSeparator = " · "

// status is the footer's upper line. A query being typed holds it alone, since the cursor is in it;
// otherwise the filter in force sits beside the freshest thing the app has to say — the reason a key
// was inert, or the receipt for the last write (§3.1).
func (m Model) status() string {
	if m.prompt.kind != promptNone {
		return m.promptLine()
	}
	if m.filtering {
		return m.theme.FilterActive.Render(" /" + m.filter + "▏")
	}

	var parts []string
	if m.filter != "" {
		parts = append(parts, m.theme.FilterActive.Render("/"+m.filter))
	}

	last, written := m.lastReceipt()
	switch {
	case m.hint != "":
		parts = append(parts, m.theme.Dim.Render(m.hint))
	case written:
		parts = append(parts, m.theme.Receipt.Render(last.line()))
	case m.filter != "":
		parts = append(parts, m.theme.Dim.Render("esc clears"))
	}

	line := ""
	if len(parts) > 0 {
		line = " " + strings.Join(parts, "  ")
	}
	return m.withAffordance(line)
}

// withAffordance hangs the keys that act on the last write against the right edge of the status line,
// which is where the undo is offered rather than in the tab's fixed hints (§1.2).
func (m Model) withAffordance(line string) string {
	keys := m.undoAffordance()
	if keys == "" {
		return line
	}

	gap := m.viewWidth() - lipgloss.Width(line) - lipgloss.Width(keys) - 1
	if gap < 2 {
		return line
	}
	return line + strings.Repeat(" ", gap) + m.theme.Dim.Render(keys) + " "
}

// headerLine is the line above every tab's rule: whatever segmented row the tab draws on the left,
// and its counts hung against the right edge. The row's printed width is passed rather than measured
// because the styling makes the string longer than the columns it occupies.
func (m Model) headerLine(row string, rowWidth int, counts string, width int) string {
	gap := width - rowWidth - lipgloss.Width(counts) - 1
	if gap < 1 {
		gap = 1
	}
	return row + strings.Repeat(" ", gap) + m.theme.Count.Render(counts)
}

// segmentRow draws the toggle every tab hangs off the left of its header — Loops' statuses, Repos'
// kinds, Agents' ranges — marking the selected segment. The printed width comes back alongside the
// row, since the styling makes the string longer than the columns it occupies.
func (m Model) segmentRow(labels []string, active int) (string, int) {
	var row strings.Builder
	row.WriteString(" ")
	width := 1

	for i, label := range labels {
		if i > 0 {
			row.WriteString("  ")
			width += 2
		}

		style := m.theme.FilterInactive
		if i == active {
			label, style = "‹"+label+"›", m.theme.FilterActive
		}

		row.WriteString(style.Render(label))
		width += lipgloss.Width(label)
	}
	return row.String(), width
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
