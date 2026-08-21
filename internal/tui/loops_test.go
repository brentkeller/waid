package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/tree"
)

// loopsNow is the instant a Loops view renders its ages against, so a column that counts backwards
// prints the same thing on every run.
var loopsNow = time.Date(2026, 8, 17, 16, 0, 0, 0, time.Local)

// loopsFixture is the log §4 draws: a heading with two items under it, an item carrying the promoted
// tag, one waiting on someone, a run of items belonging to no heading, and one already closed.
func loopsFixture() loopsLoadedMsg {
	waid := `C:\dev\waid`
	devresults := `C:\dev\dr\devresults\devresults`

	return loopsLoadedMsg{
		items: []events.Item{
			{
				Id: "vq2n", Title: "Localized notifications", Status: events.StatusOpen,
				Origin: &devresults, Created: *stamp(14, 9, 0), Updated: *stamp(14, 9, 0),
			},
			{
				Id: "nktt", Title: "Background workers speak the requester's language",
				Status: events.StatusOpen, Origin: &devresults, Tags: []string{"promoted"},
				Parent:  text("vq2n"),
				Created: *stamp(15, 16, 0), Updated: *stamp(15, 16, 0),
			},
			{
				Id: "sga9", Title: "Design template + args persistence for localized persisted strings",
				Status: events.StatusOpen, Origin: &devresults, Parent: text("vq2n"),
				Notes:   []events.Note{{Ts: *stamp(17, 15, 41), Text: "needs to survive a round trip through the queue"}},
				Created: *stamp(17, 15, 41), Updated: *stamp(17, 15, 41),
			},
			{
				Id: "4h2k", Title: "Deploy blocked until the migration is approved",
				Status: events.StatusWaiting, Origin: &devresults, WaitingOn: text("maria"),
				Created: *stamp(12, 16, 0), Updated: *stamp(12, 16, 0),
			},
			{
				Id: "9xz1", Title: "TUI design spike", Status: events.StatusOpen, Origin: &waid,
				Created: *stamp(17, 15, 0), Updated: *stamp(17, 15, 0),
			},
			{
				Id: "p0rt", Title: "Read the port design once more", Status: events.StatusOpen,
				Created: *stamp(17, 14, 0), Updated: *stamp(17, 14, 0),
			},
			{
				Id: "shut", Title: "Already dealt with", Status: events.StatusDone, Origin: &waid,
				Created: *stamp(16, 9, 0), Updated: *stamp(16, 9, 0),
			},
		},
		at: loopsNow,
	}
}

// loopsNested is the §4 fixture with one row pushed a level deeper, so the log nests three deep:
// 4h2k hangs under nktt, which hangs under vq2n. Depth is where the indent, the fold counts and the
// (unassigned) rule read differently from a flat level, and the fixture beside it stops at two.
func loopsNested() loopsLoadedMsg {
	msg := loopsFixture()
	for i := range msg.items {
		if msg.items[i].Id == "4h2k" {
			msg.items[i].Parent = text("nktt")
		}
	}
	return msg
}

// looped is a model sized, left on Loops, and holding a finished read of the log.
func looped(t *testing.T, width int, msg loopsLoadedMsg) Model {
	t.Helper()

	m := offline(chrome(t, width), detect.Result{})
	m.clock = func() time.Time { return loopsNow }
	m.loops.load = func() tea.Msg { return msg }

	next, _ := m.Update(m.loops.load())
	return next.(Model)
}

// rowFor is the printed line the item with this id was drawn on, which is what the column assertions
// measure rather than the whole view.
func rowFor(t *testing.T, view, id string) string {
	t.Helper()

	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, " "+id+" ") {
			return line
		}
	}
	t.Fatalf("no row for %q in:\n%s", id, view)
	return ""
}

// loopsChain is a log of one branch nested to the depth named, the deepest of them a leaf. The §4
// fixture is two levels, so the indent assertions are measured against this instead.
func loopsChain(depth int) loopsLoadedMsg {
	msg := loopsLoadedMsg{at: loopsNow}
	for level := range depth {
		item := events.Item{
			Id: fmt.Sprintf("n%03d", level), Title: fmt.Sprintf("level %d", level),
			Status: events.StatusOpen, Created: *stamp(17, 12, 0), Updated: *stamp(17, 12, 0),
		}
		if level > 0 {
			item.Parent = text(fmt.Sprintf("n%03d", level-1))
		}
		msg.items = append(msg.items, item)
	}
	return msg
}

// lineFor is the printed line a row's text was drawn on, which is what the fold assertions read
// rather than the whole view.
func lineFor(t *testing.T, view, text string) string {
	t.Helper()

	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, text) {
			return line
		}
	}

	t.Fatalf("no line carrying %q in:\n%s", text, view)
	return ""
}

// leading is the blank margin a row sits behind, measured rather than counted in bytes: the markers
// the rows carry are wide runes.
func leading(t *testing.T, view, text string) int {
	t.Helper()

	line := lineFor(t, view, text)
	return lipgloss.Width(line) - lipgloss.Width(strings.TrimLeft(line, " "))
}

// The list nests to whatever depth the log declares rather than to the two levels the tabs beside it
// draw, a step of indent per generation (§4).
func TestLoopsRowsNestToFullDepth(t *testing.T) {
	m := looped(t, 140, loopsChain(4))

	rows := m.loopsTree(140).rows()
	if len(rows) != 4 {
		t.Fatalf("a four-level branch drew %d rows, want one per level", len(rows))
	}
	for level, row := range rows {
		if row.Depth != level {
			t.Errorf("level %d is drawn at depth %d", level, row.Depth)
		}
	}

	// G puts the cursor on the deepest row, so every row measured carries a marker: a fold's own, or
	// the cursor's on the leaf at the bottom.
	focused, _ := press(t, m, "G")

	view := plain(focused.View())
	for level := range 4 {
		title := fmt.Sprintf("level %d", level)
		if got, want := leading(t, view, title), headingIndent+indentStep*level; got != want {
			t.Errorf("%q is indented %d columns, want %d", title, got, want)
		}
	}
}

// A level that mixes headings with items gathers the loose items into one synthetic bucket, and a
// level that holds only items keeps them where they are (§4).
func TestLoopsGathersLooseItemsIntoTheUnassignedBucket(t *testing.T) {
	view := plain(looped(t, 140, loopsFixture()).View())

	if got := strings.Count(view, tree.UnassignedTitle); got != 1 {
		t.Fatalf("the view holds %d buckets, want the one the top level earns:\n%s", got, view)
	}
	// The bucket sorts last, so everything the heading above it holds is drawn before it.
	if at, heading := strings.Index(view, tree.UnassignedTitle), strings.Index(view, "sga9"); at < heading {
		t.Errorf("the bucket is drawn above the heading's own rows:\n%s", view)
	}
	// The items under the heading are all leaves, so that level gets no bucket of its own.
	if got := leading(t, view, tree.UnassignedTitle); got != headingIndent {
		t.Errorf("the bucket is indented %d columns, want it at the top level", got)
	}
}

// A fold says how many open loops sit beneath it, which is what a collapsed branch is worth reading
// (§4).
func TestLoopsFoldsCountWhatIsOpenBeneathThem(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	view := plain(m.View())
	if line := lineFor(t, view, "Localized notifications"); !strings.Contains(line, "2 open") {
		t.Errorf("the heading does not count what it holds: %q", line)
	}
	if line := lineFor(t, view, tree.UnassignedTitle); !strings.Contains(line, "3 open") {
		t.Errorf("the bucket does not count what it holds: %q", line)
	}

	// Three presses of s put the status row on all, which draws the closed item as well. It is not
	// open, so the count it lands in does not move.
	all, _ := press(t, m, "s", "s", "s")
	if line := lineFor(t, plain(all.View()), tree.UnassignedTitle); !strings.Contains(line, "3 open") {
		t.Errorf("the bucket counts a closed row as open: %q", line)
	}
}

// Indent is capped past a few levels, so a deep branch cannot squeeze the title column away on a
// narrow terminal (§7).
func TestLoopsIndentIsCappedOnADeepBranch(t *testing.T) {
	view := plain(looped(t, 80, loopsChain(8)).View())

	capped := leading(t, view, fmt.Sprintf("level %d", maxIndentDepth))
	for _, level := range []int{maxIndentDepth + 1, maxIndentDepth + 2} {
		title := fmt.Sprintf("level %d", level)
		if got := leading(t, view, title); got != capped {
			t.Errorf("%q is indented %d columns, want the cap of %d", title, got, capped)
		}
	}

	// The row at the bottom of the branch still has room for every column it carries.
	row := rowFor(t, view, "n007")
	for _, want := range []string{"open", "level 7"} {
		if !strings.Contains(row, want) {
			t.Errorf("the deepest row lost %q:\n%s", want, row)
		}
	}
	if strings.Contains(row, "…") {
		t.Errorf("the deepest row was clipped:\n%s", row)
	}
}

// A row is the id, the status, the title, what the item is waiting on, and its age (§1.1).
func TestLoopsRowCarriesItsColumns(t *testing.T) {
	view := plain(looped(t, 140, loopsFixture()).View())

	row := rowFor(t, view, "4h2k")
	for _, want := range []string{"waiting", "Deploy blocked until the migration is approved", "@maria", "5d"} {
		if !strings.Contains(row, want) {
			t.Errorf("the row for 4h2k does not carry %q:\n%s", want, row)
		}
	}

	// The columns are shared across every visible item, so the list reads down a column rather than
	// across a row.
	other := rowFor(t, view, "9xz1")
	if got, want := column(t, row, "waiting"), column(t, other, "open"); got != want {
		t.Errorf("the status column starts at %d on one row and %d on another:\n%s\n%s", got, want, row, other)
	}
}

// column is the printed column a row's cell begins in, measured rather than counted in bytes: the
// cursor marker is a wide rune, so the focused row's cells sit at different byte offsets.
func column(t *testing.T, row, cell string) int {
	t.Helper()

	at := strings.Index(row, cell)
	if at < 0 {
		t.Fatalf("no %q in row:\n%s", cell, row)
	}
	return lipgloss.Width(row[:at])
}

// A promoted item wears the tag it was written with, which is what tells a triaged signal apart from
// an item that was declared by hand (§3).
func TestLoopsRendersThePromotedTag(t *testing.T) {
	view := plain(looped(t, 140, loopsFixture()).View())

	if row := rowFor(t, view, "nktt"); !strings.Contains(row, "[promoted]") {
		t.Errorf("the row for nktt does not carry its tag:\n%s", row)
	}
}

// The header counts the rows the list is showing, headings among them (§1.1).
func TestLoopsHeaderCountsItems(t *testing.T) {
	view := plain(looped(t, 140, loopsFixture()).View())

	if want := "6 items"; !strings.Contains(view, want) {
		t.Errorf("the loops header does not say %q:\n%s", want, view)
	}
}

// Closing an item is the only way out of the list, so a closed one is neither listed nor counted, and
// the tab's badge is what is still owed.
func TestLoopsLeavesClosedItemsOut(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	if view := plain(m.View()); strings.Contains(view, "Already dealt with") {
		t.Errorf("a closed item is still in the list:\n%s", view)
	}
	if got, want := m.counts[tabLoops], 6; got != want {
		t.Errorf("the Loops badge is %d, want %d", got, want)
	}
}

// The tab says so when the log holds nothing still owed, rather than drawing an empty list.
func TestLoopsSaysWhenThereIsNothingOwed(t *testing.T) {
	view := plain(looped(t, 140, loopsLoadedMsg{at: loopsNow}).View())

	if !strings.Contains(view, tabEmpty[tabLoops]) {
		t.Errorf("an empty Loops tab does not say %q:\n%s", tabEmpty[tabLoops], view)
	}
}

// The cursor walks the rows, and enter folds the branch it is in — the same tree Repos and Agents
// draw (§7).
func TestLoopsCursorWalksTheTree(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	moved, _ := press(t, m, "j")
	item, ok := moved.loopsTree(140).SelectedItem()
	if !ok {
		t.Fatalf("the cursor is on no item after a move, want the second row of the branch")
	}
	if item.Id != "sga9" {
		t.Errorf("one move landed on %q, want sga9", item.Id)
	}

	folded, _ := press(t, moved, "enter")
	if view := plain(folded.View()); strings.Contains(view, "Design template") {
		t.Errorf("enter did not collapse the branch the cursor was in:\n%s", view)
	}
}

// selectedTitle is the title of the row under the cursor, which is what the heading assertions read:
// a fold and the synthetic bucket carry one where they carry no item.
func selectedTitle(t *testing.T, m Model) string {
	t.Helper()

	row, ok := m.loopsTree(m.viewWidth()).selected()
	if !ok {
		t.Fatal("the cursor is on no row at all")
	}
	return row.Title
}

// Loops' headings are items, so the cursor rests on them rather than stepping over them the way it
// does on the two tabs whose headings are projects (§7).
func TestLoopsCursorRestsOnHeadings(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	item, ok := m.loopsTree(140).SelectedItem()
	if !ok || item.Id != "vq2n" {
		t.Fatalf("the tab opens on %+v, want the heading vq2n", item)
	}

	// Every row below it is reachable in turn, the expanded bucket among them.
	want := []string{
		"Design template + args persistence for localized persisted strings",
		"Background workers speak the requester's language",
		tree.UnassignedTitle,
		"TUI design spike",
	}
	for _, title := range want {
		m, _ = press(t, m, "j")
		if got := selectedTitle(t, m); got != title {
			t.Fatalf("a move down landed on %q, want %q", got, title)
		}
	}
}

// h closes the fold the cursor is in and l opens it, so a branch is walked out of and back into
// without the cursor leaving it. enter still toggles (§7).
func TestLoopsFoldsWithHAndL(t *testing.T) {
	const child = "Design template"

	m := looped(t, 140, loopsFixture())

	// The cursor opens on a heading, so h closes that heading itself.
	collapsed, _ := press(t, m, "h")
	if view := plain(collapsed.View()); strings.Contains(view, child) {
		t.Errorf("h did not collapse the heading the cursor was on:\n%s", view)
	}
	if got := selectedTitle(t, collapsed); got != "Localized notifications" {
		t.Errorf("h left the cursor on %q, want the fold it closed", got)
	}

	expanded, _ := press(t, collapsed, "l")
	if view := plain(expanded.View()); !strings.Contains(view, child) {
		t.Errorf("l did not expand the fold under the cursor:\n%s", view)
	}

	// From a row inside the fold, h closes the fold it hangs under and lands on it.
	inside, _ := press(t, m, "j", "h")
	if view := plain(inside.View()); strings.Contains(view, child) {
		t.Errorf("h from inside the fold did not close it:\n%s", view)
	}
	if got := selectedTitle(t, inside); got != "Localized notifications" {
		t.Errorf("h from inside the fold left the cursor on %q, want the fold it closed", got)
	}

	// l on a fold already open leaves both the tree and the cursor where they are.
	still, _ := press(t, m, "l")
	if got := selectedTitle(t, still); got != "Localized notifications" {
		t.Errorf("l on an open fold moved the cursor to %q", got)
	}
	if view := plain(still.View()); !strings.Contains(view, child) {
		t.Errorf("l on an open fold closed it:\n%s", view)
	}

	toggled, _ := press(t, m, "enter")
	if view := plain(toggled.View()); strings.Contains(view, child) {
		t.Errorf("enter no longer toggles the fold under the cursor:\n%s", view)
	}
}

// r re-reads the log, which is the only thing that brings in an item another session declared.
func TestLoopsRefreshRereadsTheLog(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	_, cmd := press(t, m, "r")
	if cmd == nil {
		t.Fatal("r on Loops issued no read")
	}
	if _, ok := cmd().(loopsLoadedMsg); !ok {
		t.Errorf("r produced %T, want a loopsLoadedMsg", cmd())
	}
}

// s walks the status row and wraps, in the order §1.1 draws it.
func TestLoopsStatusRowCycles(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	for _, want := range []loopsStatus{loopsWaiting, loopsDone, loopsAll, loopsOwed} {
		next, _ := press(t, m, "s")
		if next.loops.status != want {
			t.Fatalf("s moved the status row from %q to %q, want %q", m.loops.status, next.loops.status, want)
		}
		m = next
	}
}

// The selected segment is the one the row marks, so the list always says what it is showing.
func TestLoopsStatusRowMarksTheSelectedSegment(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	if view := plain(m.View()); !strings.Contains(view, "‹open›  waiting  done  all") {
		t.Errorf("the status row does not open on open:\n%s", view)
	}

	waiting, _ := press(t, m, "s")
	if view := plain(waiting.View()); !strings.Contains(view, "open  ‹waiting›  done  all") {
		t.Errorf("s did not move the marker to waiting:\n%s", view)
	}
}

// The row filters over what is already loaded, so no segment issues a read.
func TestLoopsStatusFilterIssuesNoWork(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	for i := range loopsStatuses {
		next, cmd := press(t, m, "s")
		if cmd != nil {
			t.Errorf("segment %d issued %T, want no work", i, cmd())
		}
		m = next
	}
}

// Each segment narrows the list to what it names: `open` is everything still owed, the way
// `waid loops` means an open loop, and `all` is the whole log (§1.1).
func TestLoopsStatusFilterNarrowsTheList(t *testing.T) {
	cases := []struct {
		presses int
		status  loopsStatus
		shown   []string
		hidden  []string
		counts  string
	}{
		{0, loopsOwed, []string{"TUI design spike", "Deploy blocked"}, []string{"Already dealt with"}, "6 items"},
		{1, loopsWaiting, []string{"Deploy blocked"}, []string{"TUI design spike", "Already dealt with"}, "1 item"},
		{2, loopsDone, []string{"Already dealt with"}, []string{"TUI design spike", "Deploy blocked"}, "1 item"},
		{3, loopsAll, []string{"TUI design spike", "Deploy blocked", "Already dealt with"}, nil, "7 items"},
	}

	for _, c := range cases {
		m := looped(t, 140, loopsFixture())
		for range c.presses {
			m, _ = press(t, m, "s")
		}
		if m.loops.status != c.status {
			t.Fatalf("%d presses left the row on %q, want %q", c.presses, m.loops.status, c.status)
		}

		view := plain(m.View())
		for _, want := range c.shown {
			if !strings.Contains(view, want) {
				t.Errorf("the %q list drops %q:\n%s", c.status, want, view)
			}
		}
		for _, unwanted := range c.hidden {
			if strings.Contains(view, unwanted) {
				t.Errorf("the %q list keeps %q:\n%s", c.status, unwanted, view)
			}
		}
		if !strings.Contains(view, c.counts) {
			t.Errorf("the %q header does not say %q:\n%s", c.status, c.counts, view)
		}
	}
}

// The badge is what is owed whatever the row is showing: a filter narrows the view and not the work,
// which is the rule the Scan badge already follows.
func TestLoopsBadgeIgnoresTheStatusFilter(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	for range loopsStatuses {
		m, _ = press(t, m, "s")
		if got, want := m.counts[tabLoops], 6; got != want {
			t.Errorf("the Loops badge is %d under %q, want %d", got, m.loops.status, want)
		}
	}
}

// The digits address the tabs on every tab, so the status row is reached by `s` alone (§4).
func TestLoopsDigitsStayWithTheTabs(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	switched, _ := press(t, m, "2")
	if switched.tab != tabScan {
		t.Errorf("2 on Loops selected %s, want Repos", tabTitles[switched.tab])
	}
	if switched.loops.status != loopsOwed {
		t.Errorf("2 on Loops moved the status row to %q", switched.loops.status)
	}

	fourth, _ := press(t, m, "4")
	if fourth.loops.status != loopsOwed {
		t.Errorf("4 on Loops moved the status row to %q", fourth.loops.status)
	}
	if fourth.hint == "" {
		t.Error("4 on Loops left no footer hint, so an unbound digit says nothing")
	}
}

// The typed query narrows the list client-side over what is already loaded, so it issues no work.
func TestLoopsFilterNarrowsTheList(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	filtered, _ := press(t, m, "/", "m", "a", "r", "i", "a", "enter")
	view := plain(filtered.View())
	if !strings.Contains(view, "Deploy blocked") {
		t.Errorf("the filter dropped the row it matches:\n%s", view)
	}
	if strings.Contains(view, "TUI design spike") {
		t.Errorf("the filter kept a row it does not match:\n%s", view)
	}
	if want := "1 item"; !strings.Contains(view, want) {
		t.Errorf("the filtered header does not say %q:\n%s", want, view)
	}
}

// The note the fixture's sga9 carries. It is written nowhere else in the view, so its presence is
// what says the detail pane is open.
const loopsNote = "needs to survive a round trip through the queue"

// detailPane is the block the pane drew: the lines under the last rule the body holds, which is the
// rule the pane hangs from (§1.1).
func detailPane(t *testing.T, view string) string {
	t.Helper()

	lines := strings.Split(plain(view), "\n")
	// The last three lines are the app's own divider and its two-line footer, which sit under the body.
	body := lines[:max(len(lines)-3, 0)]
	for i := len(body) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(body[i]); trimmed != "" && strings.Trim(trimmed, "─") == "" {
			return strings.Join(body[i+1:], "\n")
		}
	}

	t.Fatalf("no rule to hang a detail pane from in:\n%s", view)
	return ""
}

// The pane is `show`'s data for the row under the cursor: what identifies the item, its title, and
// the notes it has collected with the age of each (§1.1).
func TestLoopsDetailPaneRendersTheSelectedItem(t *testing.T) {
	m, _ := press(t, looped(t, 140, loopsFixture()), "j")

	pane := detailPane(t, m.View())
	for _, want := range []string{
		"sga9", "devresults", "open", "created 19m ago",
		"Design template + args persistence for localized persisted strings",
		"notes", "19m", loopsNote,
	} {
		if !strings.Contains(pane, want) {
			t.Errorf("the detail pane does not carry %q:\n%s", want, pane)
		}
	}
}

// A waiting item names whoever it waits on, joined the way `waid show` joins it.
func TestLoopsDetailPaneNamesWhoAnItemWaitsOn(t *testing.T) {
	m, _ := press(t, looped(t, 140, loopsFixture()), "G")
	pane := detailPane(t, m.View())

	for _, want := range []string{"4h2k", "waiting ← maria", "created 5d ago"} {
		if !strings.Contains(pane, want) {
			t.Errorf("the detail pane does not carry %q:\n%s", want, pane)
		}
	}
}

// The pane follows the cursor rather than being opened on one row, so moving the list moves it.
func TestLoopsDetailPaneFollowsTheCursor(t *testing.T) {
	m, _ := press(t, looped(t, 140, loopsFixture()), "j", "j")

	pane := detailPane(t, m.View())
	if !strings.Contains(pane, "nktt") {
		t.Errorf("the pane did not follow the cursor onto nktt:\n%s", pane)
	}
	if strings.Contains(pane, "sga9") {
		t.Errorf("the pane is still showing the row the cursor left:\n%s", pane)
	}
}

// An item with no notes prints no notes block, the way `waid show` leaves one out.
func TestLoopsDetailPaneLeavesOutAnEmptyNotesBlock(t *testing.T) {
	m, _ := press(t, looped(t, 140, loopsFixture()), "j", "j")

	if pane := detailPane(t, m.View()); strings.Contains(pane, "notes") {
		t.Errorf("the pane drew a notes block for an item with none:\n%s", pane)
	}
}

// A heading is an item, so the pane draws its own detail rather than treating the row as chrome (§7).
func TestLoopsDetailPaneRendersAHeading(t *testing.T) {
	pane := detailPane(t, looped(t, 140, loopsFixture()).View())

	for _, want := range []string{"vq2n", "devresults", "open", "Localized notifications"} {
		if !strings.Contains(pane, want) {
			t.Errorf("the detail pane on a heading does not carry %q:\n%s", want, pane)
		}
	}
}

// The (unassigned) bucket is a rendering artifact rather than an item, so the pane says what to do
// there rather than showing the last item it was pointed at (§4).
func TestLoopsDetailPaneSaysWhenNoItemIsSelected(t *testing.T) {
	folded, _ := press(t, looped(t, 140, loopsFixture()), "j", "j", "j")

	pane := detailPane(t, folded.View())
	if !strings.Contains(pane, "select an item") {
		t.Errorf("the pane on the bucket does not say what to do:\n%s", pane)
	}
	if strings.Contains(pane, "4h2k") {
		t.Errorf("the pane on the bucket is still showing an item:\n%s", pane)
	}
}

// The pane is open by default — §1.1 draws it — and p folds it away when density matters more.
func TestLoopsDetailPaneCollapsesWithP(t *testing.T) {
	m, _ := press(t, looped(t, 140, loopsFixture()), "j")
	if !strings.Contains(plain(m.View()), loopsNote) {
		t.Fatalf("the detail pane is not open before p was pressed:\n%s", plain(m.View()))
	}

	collapsed, cmd := press(t, m, "p")
	if cmd != nil {
		t.Errorf("p issued %T, want no work", cmd())
	}
	if view := plain(collapsed.View()); strings.Contains(view, loopsNote) {
		t.Errorf("p did not collapse the detail pane:\n%s", view)
	}

	reopened, _ := press(t, collapsed, "p")
	if view := plain(reopened.View()); !strings.Contains(view, loopsNote) {
		t.Errorf("p did not reopen the detail pane:\n%s", view)
	}
}

// The pane hangs off the bottom of the body rather than following the last row of the list, so the
// list does not shift under the cursor as the item it is pointed at grows notes.
func TestLoopsDetailPaneSitsOnTheBottomOfTheBody(t *testing.T) {
	m, _ := press(t, looped(t, 140, loopsFixture()), "j")

	lines := strings.Split(plain(m.View()), "\n")
	if got := lines[len(lines)-4]; !strings.Contains(got, loopsNote) {
		t.Errorf("the last body line is %q, want the pane's last line:\n%s", got, plain(m.View()))
	}
}

// The pane takes at most half the body, so a run of notes narrows the pane rather than squeezing the
// list out of the tab. The cut is marked, so a trimmed pane does not read as a short one.
func TestLoopsDetailPaneIsCutToItsShareOfTheBody(t *testing.T) {
	msg := loopsFixture()
	for i := range msg.items {
		if msg.items[i].Id != "sga9" {
			continue
		}
		for range 20 {
			msg.items[i].Notes = append(msg.items[i].Notes, events.Note{Ts: *stamp(17, 15, 41), Text: loopsNote})
		}
	}

	m, _ := press(t, looped(t, 140, msg), "j")
	pane := detailPane(t, m.View())

	if got, want := len(strings.Split(pane, "\n")), 9; got > want {
		t.Errorf("the pane drew %d lines, want at most %d:\n%s", got, want, pane)
	}
	if !strings.Contains(pane, "…") {
		t.Errorf("the pane was cut without saying so:\n%s", pane)
	}
}

// Golden views of the Loops tab at the two widths §8 names, which is the last of the three tabs to
// have one.
func TestGoldenLoopsAt80Columns(t *testing.T) { goldenLoops(t, 80) }

func TestGoldenLoopsAt140Columns(t *testing.T) { goldenLoops(t, 140) }

func goldenLoops(t *testing.T, width int) {
	t.Helper()

	teatest.RequireEqualOutput(t, []byte(plain(looped(t, width, loopsFixture()).View())))
}

// The same two widths over a log that nests three deep, since the fixture above stops at two and a
// third level is where the indent, a fold hanging under a fold and the (unassigned) rule applied to
// an inner level are all first visible (§9).
func TestGoldenLoopsNestedAt80Columns(t *testing.T) { goldenNested(t, 80) }

func TestGoldenLoopsNestedAt140Columns(t *testing.T) { goldenNested(t, 140) }

func goldenNested(t *testing.T, width int) {
	t.Helper()

	teatest.RequireEqualOutput(t, []byte(plain(looped(t, width, loopsNested()).View())))
}

// A filtered tree, where the one match is a leaf three levels down: the ancestors it hangs under are
// kept as the path to it, and the branches holding no match are dropped whole (§4).
func TestGoldenLoopsFilteredAt80Columns(t *testing.T) { goldenFiltered(t, 80) }

func TestGoldenLoopsFilteredAt140Columns(t *testing.T) { goldenFiltered(t, 140) }

func goldenFiltered(t *testing.T, width int) {
	t.Helper()

	m, _ := press(t, looped(t, width, loopsNested()), "/", "m", "i", "g", "r", "a", "t", "i", "o", "n", "enter")
	teatest.RequireEqualOutput(t, []byte(plain(m.View())))
}

// A deep tree at the narrow width, which is the case the indent cap exists for: past a few levels
// the step stops growing so the title column survives a branch deeper than 80 columns has room to
// walk. Only the narrow width is snapshotted — a cap that holds at 80 is not tested again at 140.
func TestGoldenLoopsDeepAt80Columns(t *testing.T) {
	teatest.RequireEqualOutput(t, []byte(plain(looped(t, 80, loopsChain(8)).View())))
}
