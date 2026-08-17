package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
)

// loopsNow is the instant a Loops view renders its ages against, so a column that counts backwards
// prints the same thing on every run.
var loopsNow = time.Date(2026, 8, 17, 16, 0, 0, 0, time.Local)

// loopsFixture is the log §1.1 draws: three projects, an item carrying the promoted tag, one waiting
// on someone, an item belonging to no project, and one already closed.
func loopsFixture() loopsLoadedMsg {
	waid := `C:\dev\waid`
	devresults := `C:\dev\dr\devresults\devresults`

	return loopsLoadedMsg{
		items: []events.Item{
			{
				Id: "nktt", Title: "Background workers speak the requester's language",
				Status: events.StatusOpen, Project: &devresults, Tags: []string{"promoted"},
				Created: *stamp(15, 16, 0), Updated: *stamp(15, 16, 0),
			},
			{
				Id: "sga9", Title: "Design template + args persistence for localized persisted strings",
				Status: events.StatusOpen, Project: &devresults,
				Created: *stamp(17, 15, 41), Updated: *stamp(17, 15, 41),
			},
			{
				Id: "4h2k", Title: "Deploy blocked until the migration is approved",
				Status: events.StatusWaiting, Project: &devresults, WaitingOn: text("maria"),
				Created: *stamp(12, 16, 0), Updated: *stamp(12, 16, 0),
			},
			{
				Id: "9xz1", Title: "TUI design spike", Status: events.StatusOpen, Project: &waid,
				Created: *stamp(17, 15, 0), Updated: *stamp(17, 15, 0),
			},
			{
				Id: "p0rt", Title: "Read the port design once more", Status: events.StatusOpen,
				Created: *stamp(17, 14, 0), Updated: *stamp(17, 14, 0),
			},
			{
				Id: "shut", Title: "Already dealt with", Status: events.StatusDone, Project: &waid,
				Created: *stamp(16, 9, 0), Updated: *stamp(16, 9, 0),
			},
		},
		at: loopsNow,
	}
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

// Items are filed under the project they belong to, with the ones belonging to none in a group of
// their own (§1.1).
func TestLoopsGroupsItemsByProject(t *testing.T) {
	view := plain(looped(t, 140, loopsFixture()).View())

	for _, want := range []string{`C:\dev\dr\devresults\devresults`, `C:\dev\waid`, noProject, "3 items"} {
		if !strings.Contains(view, want) {
			t.Errorf("the loops view does not carry %q:\n%s", want, view)
		}
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

// The header counts what is owed and how many projects it is spread across (§1.1).
func TestLoopsHeaderCountsItemsAndProjects(t *testing.T) {
	view := plain(looped(t, 140, loopsFixture()).View())

	if want := "5 items · 3 projects"; !strings.Contains(view, want) {
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
	if got, want := m.counts[tabLoops], 5; got != want {
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

// The cursor walks the items and steps over the project headings, and enter folds the group it is in
// — the same tree Scan and Review draw (§2).
func TestLoopsCursorWalksTheTree(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	moved, _ := press(t, m, "j", "j")
	item, ok := moved.loopsTree(140).SelectedItem()
	if !ok {
		t.Fatalf("the cursor is on a heading after two moves, want the third item")
	}
	if item.Id != "sga9" {
		t.Errorf("two moves landed on %q, want sga9", item.Id)
	}

	folded, _ := press(t, moved, "enter")
	if view := plain(folded.View()); strings.Contains(view, "Deploy blocked") {
		t.Errorf("enter did not collapse the group the cursor was in:\n%s", view)
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
		{0, loopsOwed, []string{"TUI design spike", "Deploy blocked"}, []string{"Already dealt with"}, "5 items · 3 projects"},
		{1, loopsWaiting, []string{"Deploy blocked"}, []string{"TUI design spike", "Already dealt with"}, "1 item · 1 project"},
		{2, loopsDone, []string{"Already dealt with"}, []string{"TUI design spike", "Deploy blocked"}, "1 item · 1 project"},
		{3, loopsAll, []string{"TUI design spike", "Deploy blocked", "Already dealt with"}, nil, "6 items · 3 projects"},
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
		if got, want := m.counts[tabLoops], 5; got != want {
			t.Errorf("the Loops badge is %d under %q, want %d", got, m.loops.status, want)
		}
	}
}

// The digits address the tabs on every tab, so the status row is reached by `s` alone (§4).
func TestLoopsDigitsStayWithTheTabs(t *testing.T) {
	m := looped(t, 140, loopsFixture())

	switched, _ := press(t, m, "2")
	if switched.tab != tabScan {
		t.Errorf("2 on Loops selected %s, want Scan", tabTitles[switched.tab])
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
	if want := "1 item · 1 project"; !strings.Contains(view, want) {
		t.Errorf("the filtered header does not say %q:\n%s", want, view)
	}
}
