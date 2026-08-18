package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/sessions"
)

// The two projects the fixture files items under, and a third that exists only as a checkout on
// disk, which is what proves the prompt matches against more than the log has seen.
const (
	devresultsPath = `C:\dev\dr\devresults\devresults`
	waidPath       = `C:\dev\waid`
	quietRepoPath  = `C:\dev\quiet-repo`
)

// filing is a Loops tab whose detection pass has landed, so the prompt matches against the repos on
// disk as well as the projects the log names.
func filing(t *testing.T) (Model, string) {
	t.Helper()

	m, path := working(t)
	m.scan.load = func() tea.Msg {
		return scanLoadedMsg{result: detect.Result{Repos: []string{quietRepoPath, waidPath}}, at: loopsNow}
	}

	loaded, _ := m.Update(m.scan.load())
	return loaded.(Model), path
}

// P opens on the project the item is already filed under, so a move is read against where it sits
// rather than typed blind.
func TestProjectOpensOnTheProjectTheItemIsFiledUnder(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "P")

	if got := m.prompt.value; got != devresultsPath {
		t.Errorf("the prompt opened on %q, want the item's project %q", got, devresultsPath)
	}
	if view := plain(m.View()); !strings.Contains(view, "project 4h2k "+devresultsPath) {
		t.Errorf("the prompt does not name the item and its project:\n%s", view)
	}
}

// An item belonging to no project opens on nothing to clear, so the first keystroke searches.
func TestProjectOpensEmptyForAnItemFiledUnderNothing(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "j", "j", "j", "j", "P")

	if m.prompt.subject != "p0rt" {
		t.Fatalf("the prompt addresses %q, want the item filed under nothing", m.prompt.subject)
	}
	if m.prompt.value != "" {
		t.Errorf("the prompt opened on %q, want it empty", m.prompt.value)
	}
}

// A fragment matches any project the log or the disk knows, case-insensitively, and the footer lists
// what it matched so enter is never a guess.
func TestProjectMatchesAFragmentAgainstProjectsAndRepos(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "QUIET")

	// The list above the footer draws every project as a heading, so the offer is read off the footer's
	// own lines rather than the whole view.
	offered := plain(m.projectChoices())
	if !strings.Contains(offered, "› "+quietRepoPath) {
		t.Errorf("the footer does not offer the repo the fragment matched:\n%s", offered)
	}
	if strings.Contains(offered, devresultsPath) {
		t.Errorf("the footer offers a project the fragment does not match:\n%s", offered)
	}
}

// tab moves through the matches when a fragment names more than one, wrapping at the end.
func TestProjectTabMovesThroughTheMatches(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "dev")
	if got := m.projectMatches(m.prompt.value); len(got) != 3 {
		t.Fatalf("dev matched %v, want all three candidates", got)
	}

	m, _ = press(t, m, "tab")
	if view := plain(m.View()); !strings.Contains(view, "› "+quietRepoPath) {
		t.Errorf("tab did not move to the second match:\n%s", view)
	}

	m, _ = press(t, m, "tab", "tab")
	if view := plain(m.View()); !strings.Contains(view, "› "+devresultsPath) {
		t.Errorf("tab did not wrap back to the first match:\n%s", view)
	}
}

// Enter files the item under the match it is on: the log takes the move and the row leaves the group
// it was in, in the frame the key was pressed in.
func TestProjectFilesTheItemUnderTheChosenMatch(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "waid", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"4h2k","project":"C:\\dev\\waid"}`})

	// The receipt names the project the way every other line of prose does — by its directory rather
	// than its path.
	view := plain(m.View())
	if !strings.Contains(view, "filed 4h2k  waid") {
		t.Errorf("the footer carries no receipt for the move:\n%s", view)
	}
	if got := groupOf(t, view, "4h2k"); got != waidPath {
		t.Errorf("the row sits under %q, want it moved to %q", got, waidPath)
	}
}

// A prompt answered with nothing files the item under no project, which is the one prompt where an
// empty answer is a write rather than an abandoned edit.
func TestProjectClearedFilesTheItemUnderNoProject(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"4h2k","project":null}`})

	if got := groupOf(t, plain(m.View()), "4h2k"); got != "(no project)" {
		t.Errorf("the row sits under %q, want it under no project", got)
	}
}

// The inverse of a move is the project the item held before it, null included — which is what makes
// filing an item that had none reversible.
func TestUndoOfAProjectRestoresTheOneTheItemHeld(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "j", "j", "j", "j", "P", "waid", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"p0rt","project":"C:\\dev\\waid"}`,
		`{"ts":"` + ts + `","ev":"update","id":"p0rt","project":null}`,
	})

	if got := groupOf(t, plain(m.View()), "p0rt"); got != "(no project)" {
		t.Errorf("the undone row sits under %q, want it back under no project", got)
	}
}

// Committing the project the item is already filed under is not a write: the log would hold a move
// that moved nothing.
func TestProjectCommittedUnchangedWritesNothing(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "enter")

	assertLog(t, path, nil)
	if _, written := m.lastReceipt(); written {
		t.Error("an unchanged project left a receipt, want the prompt treated as abandoned")
	}
}

// A path nothing matches is taken as typed, so a checkout outside the scan roots can still be named.
func TestProjectTakesAnUnmatchedAbsolutePathAsTyped(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", `D:\elsewhere\repo`, "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"4h2k","project":"D:\\elsewhere\\repo"}`})
}

// A fragment matching nothing and naming no path leaves the item where it is, and the footer says so
// rather than writing a project that does not exist.
func TestProjectSaysSoWhenAFragmentMatchesNothing(t *testing.T) {
	m, path := filing(t)

	m, _ = press(t, m, "P", "ctrl+u", "nowhere", "enter")

	assertLog(t, path, nil)
	if !strings.Contains(m.hint, "nowhere") {
		t.Errorf("the hint is %q, want it to name the fragment that matched nothing", m.hint)
	}
}

// A project heading is not an item, so P is inert there and the footer says which row it was pressed
// on (§4). A collapsed fold is the one heading the cursor can reach.
func TestProjectIsInertOnAProjectHeading(t *testing.T) {
	m, _ := filing(t)

	m, _ = press(t, m, "enter", "P")

	if m.prompt.kind != promptNone {
		t.Error("P opened a prompt on a project heading, want it inert")
	}
	if !strings.Contains(m.hint, "this row is a project") {
		t.Errorf("the hint is %q, want it to name the row P was pressed on", m.hint)
	}
}

// The projects a session recorded are candidates too, since a loop is as often filed under a repo
// the agent worked in as one the log already names.
func TestProjectMatchesAProjectOnlyASessionKnows(t *testing.T) {
	m, _ := filing(t)
	only := `C:\dev\session-only`
	m.review.sessions = []sessions.Session{{Id: "sess-1", Project: &only}}

	m, _ = press(t, m, "P", "ctrl+u", "session-only")

	if got := m.projectMatches(m.prompt.value); len(got) != 1 || got[0] != only {
		t.Errorf("the fragment matched %v, want the project only a session knows", got)
	}
}

// groupOf is the project heading the row for id sits under, which is what says a move landed in the
// list and not only in the log.
func groupOf(t *testing.T, view, id string) string {
	t.Helper()

	group := ""
	for _, line := range strings.Split(view, "\n") {
		if heading, ok := headingName(line); ok {
			group = heading
		}
		if strings.Contains(line, " "+id+" ") {
			return group
		}
	}
	t.Fatalf("no row for %q in:\n%s", id, view)
	return ""
}

// headingName is the project a fold row names. Both markers open an item row too — the cursor takes
// the collapsed one — so a row whose second column is a status is read as an item rather than a
// heading.
func headingName(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, marker := range []string{"▾ ", "▸ "} {
		rest, found := strings.CutPrefix(trimmed, marker)
		if !found {
			continue
		}
		columns := strings.Split(rest, "  ")
		if len(columns) > 1 && isStatusColumn(columns[1]) {
			return "", false
		}
		return strings.TrimSpace(columns[0]), true
	}
	return "", false
}

func isStatusColumn(column string) bool {
	switch strings.TrimSpace(column) {
	case "open", "waiting", "done":
		return true
	}
	return false
}
