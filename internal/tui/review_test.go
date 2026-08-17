package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// The transcripts the fixture's two Review sessions were parsed from. The preview reads a session by
// the path the cache recorded for it, so the fixture carries them the way a real read would.
const (
	wirePath   = `C:\transcripts\wire-i.jsonl`
	pickerPath = `C:\transcripts\picker.jsonl`
)

// reviewFixture is a history spanning two projects and two weeks: four sessions today, one in the
// week before, and one that recorded no prompts at all.
func reviewFixture() reviewLoadedMsg {
	waid := `C:\dev\waid`
	devresults := `C:\dev\dr\devresults\devresults`

	return reviewLoadedMsg{
		paths: map[string]string{
			"017516d6-7c60-4081-961e-f2120aa11111": wirePath,
			"017516d6-7c60-4081-961e-f2120aa22222": pickerPath,
		},
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

// transcripts are the turns the preview's read seam hands back, keyed by the path the cache recorded
// for a session. A session with no entry here has no transcript on record, which is the state a file
// moved or deleted since the last sync leaves behind.
var transcripts = map[string][]sessions.Turn{
	wirePath: {
		{Role: "user", Text: "Add the terminal I/O layer now — raw mode, the alternate screen, and a resize handler that survives a narrow terminal."},
		{Role: "assistant", Text: "Starting with term.ts, the only file touching stdin."},
	},
	pickerPath: {
		{Role: "user", Text: "Wire the picker into the CLI behind -i."},
	},
}

// previewing is a Review model with the transcript seam faked and the cursor resting on the first
// session of the first project, which is the row the preview reads.
func previewing(t *testing.T, width int) Model {
	t.Helper()

	m := reviewed(t, width, reviewFixture())
	m.review.readTurns = func(path string) ([]sessions.Turn, error) {
		turns, recorded := transcripts[path]
		if !recorded {
			return nil, fmt.Errorf("cannot read %s", path)
		}
		return turns, nil
	}

	m, _ = press(t, m, "j", "enter")
	return m
}

// deliver runs a command and feeds everything it produced back through Update, which is what the
// program does with the read a keypress started.
func deliver(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	for _, msg := range messages(cmd) {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// space opens the pane and closes it again, and the transcript behind it is read off the update loop
// like every other read (§6).
func TestReviewSpaceTogglesThePreview(t *testing.T) {
	m := previewing(t, 140)
	if view := plain(m.View()); strings.Contains(view, "raw mode") {
		t.Fatalf("the preview is showing before space was pressed:\n%s", view)
	}

	opened, cmd := press(t, m, " ")
	if cmd == nil {
		t.Fatal("space issued no command, want the transcript read off the update loop")
	}

	opened = deliver(t, opened, cmd)
	if view := plain(opened.View()); !strings.Contains(view, "raw mode") {
		t.Errorf("the preview does not carry the transcript after space:\n%s", view)
	}

	closed, _ := press(t, opened, " ")
	if view := plain(closed.View()); strings.Contains(view, "raw mode") {
		t.Errorf("space did not close the preview:\n%s", view)
	}
}

// The pane identifies the session and then prints its turns by role, named the way the conversation
// reads rather than the way the transcript stores it (§1.3).
func TestReviewPreviewRendersTheTurnsWithTheirRoles(t *testing.T) {
	m, cmd := press(t, previewing(t, 140), " ")
	view := plain(deliver(t, m, cmd).View())

	order := []string{
		"Wire -i into the CLI", "14:22–14:51 · 12 prompts · tui", "017516d6-7c60-4081-961e-f2120aa11111",
		"▸ you", "Add the terminal", "▸ claude", "term.ts",
	}
	at := 0
	for _, want := range order {
		found := strings.Index(view[at:], want)
		if found < 0 {
			t.Fatalf("%q is missing or out of order in:\n%s", want, view)
		}
		at += found
	}
	if strings.Contains(view, "▸ assistant") {
		t.Errorf("the pane names a role the way the transcript stores it:\n%s", view)
	}
}

// At or above the split threshold the preview sits beside the list, so both are readable at once (§5).
func TestReviewPreviewIsASideSplitAtWideWidths(t *testing.T) {
	m, cmd := press(t, previewing(t, 140), " ")
	view := plain(deliver(t, m, cmd).View())

	if !strings.Contains(view, "Show PR title and branch") {
		t.Errorf("the list is not drawn beside the preview at 140 columns:\n%s", view)
	}

	split := false
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, `C:\dev\dr\devresults\devresults`) && strings.Contains(line, "Wire -i into the CLI") {
			split = true
		}
		if lipgloss.Width(line) > 140 {
			t.Errorf("a split line is %d columns wide, want no more than 140:\n%s", lipgloss.Width(line), line)
		}
	}
	if !split {
		t.Errorf("no row carries the list and the preview at once, want them side by side:\n%s", view)
	}
}

// Below the threshold a side-by-side split leaves the transcript a gutter too narrow for prose, so
// the preview takes the whole width and the list gives way to it (§5).
func TestReviewPreviewIsAnOverlayBelowTheSplit(t *testing.T) {
	m, cmd := press(t, previewing(t, 100), " ")
	view := plain(deliver(t, m, cmd).View())

	if strings.Contains(view, `C:\dev\dr\devresults\devresults`) {
		t.Errorf("the list is still drawn at 100 columns, want the preview as a full-width overlay:\n%s", view)
	}
	if !strings.Contains(view, "▸ you") {
		t.Errorf("the overlay does not carry the transcript:\n%s", view)
	}
	// A line this long only fits the pane when the pane has the whole terminal.
	if !strings.Contains(view, "Add the terminal I/O layer now — raw mode, the alternate") {
		t.Errorf("the overlay wrapped as if it were a split pane:\n%s", view)
	}
}

// A transcript is longer than any terminal, so the pane is cut to the rows it has and says it was
// cut rather than pushing the footer off the screen.
func TestReviewPreviewIsCutToThePaneItHas(t *testing.T) {
	m := previewing(t, 140)
	m.height = 14

	opened, cmd := press(t, m, " ")
	view := plain(deliver(t, opened, cmd).View())

	if !strings.Contains(view, "…") {
		t.Errorf("the cut preview is not marked as cut:\n%s", view)
	}
	if strings.Contains(view, "term.ts") {
		t.Errorf("the preview drew past the rows the pane has:\n%s", view)
	}
	if got, want := len(strings.Split(view, "\n")), 14; got != want {
		t.Errorf("the view is %d lines tall, want %d", got, want)
	}
}

// The pane follows the cursor: a row it has not read is read, a row with no transcript on record says
// so, and a project heading has nothing to preview at all.
func TestReviewPreviewFollowsTheCursor(t *testing.T) {
	m, cmd := press(t, previewing(t, 140), " ")
	m = deliver(t, m, cmd)

	moved, cmd := press(t, m, "j")
	if cmd == nil {
		t.Fatal("moving the cursor with the preview open issued no read for the row it landed on")
	}
	moved = deliver(t, moved, cmd)

	view := plain(moved.View())
	if !strings.Contains(view, "Wire the picker into the CLI") {
		t.Errorf("the pane does not carry the transcript of the row the cursor moved to:\n%s", view)
	}
	if strings.Contains(view, "term.ts") {
		t.Errorf("the pane still carries the session the cursor left:\n%s", view)
	}

	// The last session of the project was never in the fixture's cache, so its transcript cannot be read.
	missing, cmd := press(t, moved, "j")
	view = plain(deliver(t, missing, cmd).View())
	if !strings.Contains(view, "no transcript") {
		t.Errorf("a session with no transcript on record does not say so:\n%s", view)
	}

	heading, cmd := press(t, moved, "k", "k")
	view = plain(deliver(t, heading, cmd).View())
	if !strings.Contains(view, "select a session") {
		t.Errorf("the pane on a project heading does not say there is nothing to preview:\n%s", view)
	}
}

// onSession is a Review model with the first project expanded and the cursor resting on its first
// session, which is the row the actions act on.
func onSession(t *testing.T, msg reviewLoadedMsg) Model {
	t.Helper()

	m, _ := press(t, reviewed(t, 140, msg), "j", "enter")
	return m
}

// watchResume swaps the suspension seam for one that records what would have been run, so a test
// never launches Claude from the test binary.
func watchResume(t *testing.T) *[]*exec.Cmd {
	t.Helper()

	launched := []*exec.Cmd{}
	restore := resumeProcess
	resumeProcess = func(cmd *exec.Cmd) tea.Cmd {
		launched = append(launched, cmd)
		return func() tea.Msg { return resumedMsg{} }
	}
	t.Cleanup(func() { resumeProcess = restore })
	return &launched
}

// R hands the session under the cursor back to Claude, run in the directory the session belongs to
// since Claude files its transcripts per project (§5).
func TestReviewResumeRunsClaudeOnTheSelectedSession(t *testing.T) {
	launched := watchResume(t)

	m, cmd := press(t, onSession(t, reviewFixture()), "R")
	if cmd == nil {
		t.Fatal("R produced no command, want the app suspended into claude")
	}
	if len(*launched) != 1 {
		t.Fatalf("R launched %d processes, want 1", len(*launched))
	}

	want := []string{"claude", "--resume", "017516d6-7c60-4081-961e-f2120aa11111"}
	if got := (*launched)[0].Args; !slices.Equal(got, want) {
		t.Errorf("R runs %v, want %v", got, want)
	}
	if got := (*launched)[0].Dir; got != `C:\dev\waid` {
		t.Errorf("R runs in %q, want the session's own project", got)
	}
	if m.hint != "" {
		t.Errorf("R on a session left the hint %q, want none", m.hint)
	}
}

// The suspension is Bubble Tea's own, so the terminal is released and restored around Claude rather
// than proxied through the update loop (§5).
func TestReviewResumeSuspendsThroughExecProcess(t *testing.T) {
	_, cmd := press(t, onSession(t, reviewFixture()), "R")

	msgs := messages(cmd)
	if len(msgs) != 1 {
		t.Fatalf("R produced %d messages, want 1", len(msgs))
	}
	if got := fmt.Sprintf("%T", msgs[0]); got != "tea.execMsg" {
		t.Errorf("R produced %s, want the message tea.ExecProcess suspends on", got)
	}
}

// Claude exiting brings the app back, and the history is re-read: the session that was just resumed
// has grown prompts the loaded read knows nothing about.
func TestReviewResumeRereadsTheHistoryWhenClaudeExits(t *testing.T) {
	m := onSession(t, reviewFixture())

	next, cmd := m.Update(resumedMsg{})
	back := next.(Model)
	if back.hint != "" {
		t.Errorf("a clean exit left the hint %q, want none", back.hint)
	}

	found := false
	for _, msg := range messages(cmd) {
		if _, ok := msg.(reviewLoadedMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("returning from claude produced no reviewLoadedMsg, want the history re-read")
	}

	failed, _ := m.Update(resumedMsg{err: errors.New("executable file not found")})
	if hint := failed.(Model).hint; !strings.Contains(hint, "executable file not found") {
		t.Errorf("a failed resume left the hint %q, want the reason in the footer", hint)
	}
}

// o opens the checkout the session ran in, and says so when the session recorded none (§4).
func TestReviewOpenRepo(t *testing.T) {
	opened := []string{}
	restore := openTarget
	openTarget = func(target string) error {
		opened = append(opened, target)
		return nil
	}
	t.Cleanup(func() { openTarget = restore })

	m, cmd := press(t, onSession(t, reviewFixture()), "o")
	messages(cmd)
	if len(opened) != 1 || opened[0] != `C:\dev\waid` {
		t.Fatalf("o opened %v, want the session's project", opened)
	}
	if m.hint != "" {
		t.Errorf("o on a session with a project left the hint %q, want none", m.hint)
	}

	// A session whose transcript never recorded a cwd has no checkout to open.
	msg := reviewFixture()
	msg.sessions = append(msg.sessions, sessions.Session{
		Id: "017516d6-7c60-4081-961e-f2120aa77777", Title: "Ad-hoc question",
		Started: stamp(17, 12, 0), Ended: stamp(17, 12, 5), Prompts: 2,
	})

	loose, cmd := press(t, reviewed(t, 140, msg), "j", "j", "enter", "o")
	messages(cmd)
	if len(opened) != 1 {
		t.Errorf("o opened %v from a session with no project, want nothing opened", opened)
	}
	if !strings.Contains(loose.hint, "no project") {
		t.Errorf("o on a session with no project left the hint %q, want it to say why", loose.hint)
	}
}

// y puts the session id on the clipboard, which is what a resume from another terminal needs.
func TestReviewCopiesTheSessionId(t *testing.T) {
	copied := []string{}
	restore := copyText
	copyText = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	t.Cleanup(func() { copyText = restore })

	m, cmd := press(t, onSession(t, reviewFixture()), "y")
	m = deliver(t, m, cmd)

	want := "017516d6-7c60-4081-961e-f2120aa11111"
	if len(copied) != 1 || copied[0] != want {
		t.Fatalf("y copied %v, want the session id %q", copied, want)
	}
	if !strings.Contains(m.hint, want) {
		t.Errorf("y left the hint %q, want it to name what was copied", m.hint)
	}
	if len(m.receipts) != 0 {
		t.Errorf("y recorded %d receipts, want none — a copy writes nothing to the log", len(m.receipts))
	}

	copyText = func(string) error { return errors.New("no clipboard tool available") }
	broken, cmd := press(t, onSession(t, reviewFixture()), "y")
	broken = deliver(t, broken, cmd)
	if !strings.Contains(broken.hint, "no clipboard tool") {
		t.Errorf("a failed copy left the hint %q, want the reason in the footer", broken.hint)
	}
}

// None of the three act on a fold: a project is not a session to resume, open or address (§4).
func TestReviewActionsAreInertOnAProjectHeading(t *testing.T) {
	launched := watchResume(t)

	opened, copied := []string{}, []string{}
	restoreOpen, restoreCopy := openTarget, copyText
	openTarget = func(target string) error {
		opened = append(opened, target)
		return nil
	}
	copyText = func(text string) error {
		copied = append(copied, text)
		return nil
	}
	t.Cleanup(func() { openTarget, copyText = restoreOpen, restoreCopy })

	m := reviewed(t, 140, reviewFixture())
	for _, pressed := range []string{"R", "o", "y"} {
		heading, cmd := press(t, m, pressed)
		messages(cmd)
		if !strings.Contains(heading.hint, "project") {
			t.Errorf("%s on a project heading left the hint %q, want it to name the row", pressed, heading.hint)
		}
	}

	if len(*launched)+len(opened)+len(copied) != 0 {
		t.Errorf("a heading resumed %v, opened %v and copied %v, want nothing acted on", *launched, opened, copied)
	}

	// An empty list has no row to describe at all, so the heading's reason would be a lie there.
	empty, _ := press(t, reviewed(t, 140, reviewLoadedMsg{at: reviewNow}), "R")
	if !strings.Contains(empty.hint, "no sessions") {
		t.Errorf("R on an empty list left the hint %q, want it to say the list is empty", empty.hint)
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
