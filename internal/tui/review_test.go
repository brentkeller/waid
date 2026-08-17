package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/sessions"
)

// reviewNow is the instant a Review view renders against. It is a Monday, so the current week opens
// on it and the previous week is unambiguously behind it.
var reviewNow = time.Date(2026, 8, 17, 16, 0, 0, 0, time.Local)

// stamp is a local wall-clock instant in August 2026, written the way the log stores timestamps.
func stamp(day, hour, minute int) *string {
	return text(events.FormatTs(time.Date(2026, 8, day, hour, minute, 0, 0, time.Local)))
}

// reviewFixture is a history spanning two projects and two weeks: four sessions today, one in the
// week before, and one that recorded no prompts at all.
func reviewFixture() reviewLoadedMsg {
	waid := `C:\dev\waid`
	devresults := `C:\dev\dr\devresults\devresults`

	return reviewLoadedMsg{
		sessions: []sessions.Session{
			{
				Id: "017516d6-7c60-4081-961e-f2120aa11111", Title: "Wire -i into the CLI",
				Project: &waid, Branch: text("tui"),
				Started: stamp(17, 14, 22), Ended: stamp(17, 14, 51), Prompts: 12,
			},
			{
				Id: "017516d6-7c60-4081-961e-f2120aa22222", Title: "Add terminal I/O and the picker",
				Project: &waid, Branch: text("tui"),
				Started: stamp(17, 15, 4), Ended: stamp(17, 15, 42), Prompts: 9,
			},
			{
				Id: "017516d6-7c60-4081-961e-f2120aa33333", Title: "Show PR title and branch",
				Project: &waid, Branch: text("tui"),
				Started: stamp(17, 15, 58), Ended: stamp(17, 16, 0), Prompts: 4,
			},
			{
				Id: "017516d6-7c60-4081-961e-f2120aa44444", Title: "Background workers speak the requester's language",
				Project: &devresults, Branch: text("main"),
				Started: stamp(17, 9, 10), Ended: stamp(17, 11, 30), Prompts: 28,
			},
			{
				Id: "017516d6-7c60-4081-961e-f2120aa55555", Title: "Sketch the event log",
				Project: &waid,
				Started: stamp(13, 10, 0), Ended: stamp(13, 10, 40), Prompts: 7,
			},
			{
				Id: "017516d6-7c60-4081-961e-f2120aa66666", Title: "Opened and abandoned",
				Project: &waid,
				Started: stamp(17, 8, 0), Ended: stamp(17, 8, 1), Prompts: 0,
			},
		},
		closures: []closure{
			{id: "sga9", project: &waid, at: instantAt(17, 13, 0)},
			{id: "4h2k", project: &devresults, at: instantAt(13, 9, 0)},
		},
		at: reviewNow,
	}
}

func instantAt(day, hour, minute int) time.Time {
	return time.Date(2026, 8, day, hour, minute, 0, 0, time.Local)
}

// reviewed is a model sized, switched to Review, and holding a finished read of the history.
func reviewed(t *testing.T, width int, msg reviewLoadedMsg) Model {
	t.Helper()

	m := offline(chrome(t, width), detect.Result{})
	m.clock = func() time.Time { return reviewNow }
	m.review.load = func() tea.Msg { return msg }

	m, _ = press(t, m, "3")
	next, _ := m.Update(m.review.load())
	return next.(Model)
}

// Projects are collapsed by default with their session and prompt counts on the fold, which is the
// whole reason the tab reads better than the command — a busy day is three lines until you ask for
// more (§1.3).
func TestReviewCollapsesProjectsWithCountsOnTheFold(t *testing.T) {
	m := reviewed(t, 140, reviewFixture())

	view := plain(m.View())
	for _, want := range []string{`C:\dev\waid`, `C:\dev\dr\devresults\devresults`, "3 sessions · 25 prompts", "1 session · 28 prompts"} {
		if !strings.Contains(view, want) {
			t.Errorf("the review view does not carry %q:\n%s", want, view)
		}
	}
	for _, hidden := range []string{"Wire -i into the CLI", "Show PR title and branch"} {
		if strings.Contains(view, hidden) {
			t.Errorf("%q is listed before its project was expanded:\n%s", hidden, view)
		}
	}

	// The busiest project leads, so the fold order reads as where the time went.
	if devresults, waid := strings.Index(view, `C:\dev\dr`), strings.Index(view, `C:\dev\waid`); devresults > waid {
		t.Errorf("the busiest project is not listed first:\n%s", view)
	}
}

// enter opens a fold, and the session rows carry their start time, their title and their prompt
// count, oldest first.
func TestReviewExpandsAProjectIntoItsSessions(t *testing.T) {
	m, _ := press(t, reviewed(t, 140, reviewFixture()), "j", "enter")

	view := plain(m.View())
	order := []string{"14:22", "Wire -i into the CLI", "12 prompts", "15:04", "15:58", "Show PR title and branch"}
	at := 0
	for _, want := range order {
		found := strings.Index(view[at:], want)
		if found < 0 {
			t.Fatalf("%q is missing or out of order in:\n%s", want, view)
		}
		at += found
	}
	if strings.Contains(view, "Opened and abandoned") {
		t.Errorf("a session that recorded no prompts is listed:\n%s", view)
	}
}

// The header totals the window: the sessions, the prompts they took, and the items closed inside it
// (§1.3).
func TestReviewHeaderTotalsTheWindow(t *testing.T) {
	view := plain(reviewed(t, 140, reviewFixture()).View())

	for _, want := range []string{"4 sessions", "53 prompts", "1 item closed"} {
		if !strings.Contains(view, want) {
			t.Errorf("the review header does not carry %q:\n%s", want, view)
		}
	}
}

// The range row is a segmented toggle over the loaded history: it moves the window client-side and
// issues no work.
func TestReviewRangeRowCyclesTheWindow(t *testing.T) {
	m := reviewed(t, 140, reviewFixture())

	view := plain(m.View())
	for _, want := range []string{"‹today›", "week", "last week"} {
		if !strings.Contains(view, want) {
			t.Errorf("the range row does not carry %q:\n%s", want, view)
		}
	}

	week, cmd := press(t, m, "s")
	if cmd != nil {
		t.Error("cycling the range issued a command, want the window moved over the loaded state")
	}
	view = plain(week.View())
	if !strings.Contains(view, "‹week›") {
		t.Errorf("the range row does not mark week as selected:\n%s", view)
	}
	// The current week opens on the day the fixture is pinned to, so it holds exactly today's work.
	if !strings.Contains(view, "4 sessions") {
		t.Errorf("the week totals do not match the day it opens on:\n%s", view)
	}

	last, _ := press(t, week, "s")
	view = plain(last.View())
	if !strings.Contains(view, "‹last week›") {
		t.Errorf("the range row does not mark last week as selected:\n%s", view)
	}
	for _, want := range []string{"1 session · 7 prompts", "1 item closed"} {
		if !strings.Contains(view, want) {
			t.Errorf("last week does not carry %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, `C:\dev\dr\devresults\devresults`) {
		t.Errorf("a project with no sessions last week is still listed:\n%s", view)
	}

	back, _ := press(t, last, "s")
	if back.review.window != rangeToday {
		t.Errorf("s left the range on %q after a full cycle, want it back on today", reviewRangeLabels[back.review.window])
	}
}

// The typed query narrows the list over the same loaded history the range row moves across.
func TestReviewQueryFiltersTheList(t *testing.T) {
	m := reviewed(t, 140, reviewFixture())
	m, _ = press(t, m, "/", "p", "i", "c", "k", "e", "r", "enter", "enter")

	view := plain(m.View())
	if !strings.Contains(view, "Add terminal I/O and the picker") {
		t.Errorf("the matching session is missing under the query:\n%s", view)
	}
	if strings.Contains(view, "Wire -i into the CLI") {
		t.Errorf("a session the query does not match is still listed:\n%s", view)
	}
	if strings.Contains(view, `C:\dev\dr\devresults\devresults`) {
		t.Errorf("a project with no matching sessions is still listed:\n%s", view)
	}
}

// The history is read off the update loop like every other read, and the app claims it as it starts
// (§6).
func TestInitLoadsTheHistory(t *testing.T) {
	m := offline(New(Options{}), detect.Result{})
	m.review.load = func() tea.Msg { return reviewFixture() }

	found := false
	for _, msg := range messages(m.Init()) {
		if _, ok := msg.(reviewLoadedMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("Init produced no reviewLoadedMsg, want the history read started")
	}
}

// r re-reads the history rather than filtering what is already loaded.
func TestReviewRefreshRereadsTheHistory(t *testing.T) {
	_, cmd := press(t, reviewed(t, 140, reviewFixture()), "r")

	found := false
	for _, msg := range messages(cmd) {
		if _, ok := msg.(reviewLoadedMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("r on Review produced no reviewLoadedMsg, want the history re-read")
	}
}

// Golden views of the Review tab at the two widths §8 names.
func TestGoldenReviewAt80Columns(t *testing.T) { goldenReview(t, 80) }

func TestGoldenReviewAt140Columns(t *testing.T) { goldenReview(t, 140) }

func goldenReview(t *testing.T, width int) {
	t.Helper()

	m, _ := press(t, reviewed(t, width, reviewFixture()), "enter")
	teatest.RequireEqualOutput(t, []byte(plain(m.View())))
}
