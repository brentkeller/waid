package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/brentkeller/waid/internal/detect"
)

// scannedAt and scanNow pin the clock a scan view renders against, so the age in the header is the
// same on every run.
var (
	scannedAt = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	scanNow   = scannedAt.Add(2 * time.Minute)
)

func text(s string) *string { return &s }

// scanFixture is a detection pass with one signal of every kind, two of them against a repo waid
// has a checkout of and two against one it does not.
func scanFixture() detect.Result {
	waid := `C:\dev\waid`

	return detect.Result{
		Signals: []detect.Signal{
			{
				Key:     "review:DevResults/DevResults#6886",
				Kind:    detect.KindReview,
				Title:   "Add SharedDashboards role to non-owners",
				Subject: "Add SharedDashboards role to non-owners",
				Detail:  "@copilot · 15w",
				Age:     "15w",
			},
			{
				Key:     "pr:DevResults/DevResults#5862",
				Kind:    detect.KindPr,
				Title:   "Activity Compendium prototype",
				Subject: "Activity Compendium prototype",
				Detail:  "compendium-spike · open · 1y",
				Branch:  text("compendium-spike"),
				Age:     "1y",
			},
			{
				Key:     `ahead:C:\dev\waid:tui`,
				Kind:    detect.KindAhead,
				Title:   "1 unpushed commit on tui in waid",
				Subject: "1 commit ahead",
				Detail:  "tui · 4m",
				Branch:  text("tui"),
				Project: &waid,
				Age:     "4m",
			},
			{
				Key:     `dirty:C:\dev\waid`,
				Kind:    detect.KindDirty,
				Title:   "3 uncommitted files in waid",
				Subject: "3 uncommitted files",
				Detail:  "tui · 4m",
				Branch:  text("tui"),
				Project: &waid,
				Age:     "4m",
			},
		},
		DismissedCount: 1,
	}
}

// offline pins the read seams and the clock, so a model under test never reaches the filesystem, git
// or gh, and the ages it prints do not move between runs.
func offline(m Model, result detect.Result) Model {
	m.clock = func() time.Time { return scanNow }
	m.scan.load = func() tea.Msg { return scanLoadedMsg{result: result, at: scannedAt} }
	m.review.load = func() tea.Msg { return reviewLoadedMsg{at: scannedAt} }
	return m
}

// scanned is a model sized, switched to Scan, and holding a finished detection pass.
func scanned(t *testing.T, width int, result detect.Result) Model {
	t.Helper()

	m := offline(chrome(t, width), result)
	m, _ = press(t, m, "2")

	next, _ := m.Update(m.scan.load())
	return next.(Model)
}

// messages runs a command and flattens the batch it may be, so a test can look for the message it
// cares about without knowing how the command was assembled.
func messages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, batched := range msg {
			out = append(out, messages(batched)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

// Signals are filed under the project they belong to: the local checkout when detection matched one,
// and the GitHub repository the key names when it did not (§1.2).
func TestScanGroupsSignalsByProject(t *testing.T) {
	view := plain(scanned(t, 140, scanFixture()).View())

	for _, want := range []string{"DevResults/DevResults", `C:\dev\waid`, "2 signals"} {
		if !strings.Contains(view, want) {
			t.Errorf("the scan view does not carry %q:\n%s", want, view)
		}
	}

	rows := strings.Split(view, "\n")
	order := []string{"DevResults/DevResults", "#6886", "#5862", `C:\dev\waid`, "1 commit ahead", "3 uncommitted files"}
	at := 0
	for _, want := range order {
		found := -1
		for i := at; i < len(rows); i++ {
			if strings.Contains(rows[i], want) {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("%q is missing or out of order in:\n%s", want, view)
		}
		at = found
	}
}

// The header counts what is shown, what was suppressed, and how old the pass is (§1.2).
func TestScanHeaderCountsSignalsAndDismissals(t *testing.T) {
	m := scanned(t, 140, scanFixture())

	view := plain(m.View())
	for _, want := range []string{"4 signals", "1 dismissed", "2m ago"} {
		if !strings.Contains(view, want) {
			t.Errorf("the scan header does not carry %q:\n%s", want, view)
		}
	}

	if m.counts[tabScan] != 4 {
		t.Errorf("the Scan tab carries the count %d, want 4", m.counts[tabScan])
	}
}

// The kind row is a segmented toggle over the loaded signals: it filters client-side and issues no
// work (§1.2).
func TestScanKindFilterCyclesAndNarrowsTheList(t *testing.T) {
	m := scanned(t, 140, scanFixture())

	view := plain(m.View())
	for _, want := range []string{"‹all›", "review", "pr", "ahead", "dirty"} {
		if !strings.Contains(view, want) {
			t.Errorf("the filter row does not carry %q:\n%s", want, view)
		}
	}

	filtered, cmd := press(t, m, "s")
	if cmd != nil {
		t.Error("cycling the kind filter issued a command, want it filtered client-side")
	}

	view = plain(filtered.View())
	if !strings.Contains(view, "‹review›") {
		t.Errorf("the filter row does not mark review as selected:\n%s", view)
	}
	if !strings.Contains(view, "#6886") {
		t.Errorf("the review signal is missing under the review filter:\n%s", view)
	}
	for _, gone := range []string{"#5862", "1 commit ahead", "3 uncommitted files"} {
		if strings.Contains(view, gone) {
			t.Errorf("%q is still listed under the review filter:\n%s", gone, view)
		}
	}
	if !strings.Contains(view, "1 signal ·") {
		t.Errorf("the header does not count the filtered signals:\n%s", view)
	}

	for range len(scanKinds) - 1 {
		filtered, _ = press(t, filtered, "s")
	}
	if filtered.scan.kind != "" {
		t.Errorf("s left the filter on %q after a full cycle, want it back on all", filtered.scan.kind)
	}
}

// r runs detection off the update loop and the progress slot says so while it is in flight (§6).
func TestRefreshIssuesAScan(t *testing.T) {
	refreshing, cmd := press(t, scanned(t, 140, scanFixture()), "r")
	if !refreshing.scan.loading {
		t.Error("r left the model not loading, want a refresh in flight")
	}
	if refreshing.progress == "" {
		t.Error("r left the progress slot empty, want the refresh visible in the tab bar")
	}

	var loaded *scanLoadedMsg
	for _, msg := range messages(cmd) {
		if got, ok := msg.(scanLoadedMsg); ok {
			loaded = &got
		}
	}
	if loaded == nil {
		t.Fatal("r produced no scanLoadedMsg")
	}
	if len(loaded.result.Signals) != 4 {
		t.Fatalf("the refresh returned %d signals, want 4", len(loaded.result.Signals))
	}

	next, _ := refreshing.Update(*loaded)
	done := next.(Model)
	if done.scan.loading {
		t.Error("the model is still loading after the signals arrived")
	}
	if done.progress != "" {
		t.Errorf("the progress slot still says %q after the refresh finished", done.progress)
	}
}

// The app loads the signals as it starts rather than waiting to be asked (§6).
func TestInitLoadsTheSignals(t *testing.T) {
	m := offline(New(Options{}), scanFixture())

	found := false
	for _, msg := range messages(m.Init()) {
		if _, ok := msg.(scanLoadedMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("Init produced no scanLoadedMsg, want the first pass started")
	}
}

// o opens the pull request a GitHub signal names, and is inert with a reason on a row that has no
// page to open (§4).
func TestOpenSignalInBrowser(t *testing.T) {
	opened := []string{}
	restore := openUrl
	openUrl = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() { openUrl = restore })

	m := scanned(t, 140, scanFixture())

	m, cmd := press(t, m, "o")
	messages(cmd)
	if len(opened) != 1 || opened[0] != "https://github.com/DevResults/DevResults/pull/6886" {
		t.Fatalf("o opened %v, want the review's pull request", opened)
	}
	if m.hint != "" {
		t.Errorf("o on a signal with a page left the hint %q, want none", m.hint)
	}

	local, cmd := press(t, m, "j", "j", "j", "o")
	messages(cmd)
	if len(opened) != 1 {
		t.Errorf("o opened %v from a local signal, want nothing opened", opened)
	}
	if local.hint == "" {
		t.Error("o on a local signal left no hint, want a reason in the footer")
	}

	heading, cmd := press(t, local, "enter", "o")
	messages(cmd)
	if len(opened) != 1 {
		t.Errorf("o opened %v from a project heading, want nothing opened", opened)
	}
	if !strings.Contains(heading.hint, "project") {
		t.Errorf("o on a project heading left the hint %q, want it to name the row", heading.hint)
	}
}

// The list is a tree: the cursor skips open headings, and enter folds the project under it (§2).
func TestScanFoldsAProject(t *testing.T) {
	m := scanned(t, 140, scanFixture())

	m, _ = press(t, m, "enter")
	view := plain(m.View())
	if strings.Contains(view, "#6886") {
		t.Errorf("enter left the folded project's signals on screen:\n%s", view)
	}
	if !strings.Contains(view, "DevResults/DevResults") {
		t.Errorf("the folded project is no longer listed:\n%s", view)
	}

	m, _ = press(t, m, "enter")
	if got := plain(m.View()); !strings.Contains(got, "#6886") {
		t.Errorf("enter did not reopen the fold:\n%s", got)
	}
}

// The typed query narrows the list the same way the kind row does, and over the same loaded state.
func TestScanQueryFiltersTheList(t *testing.T) {
	m := scanned(t, 140, scanFixture())
	m, _ = press(t, m, "/", "c", "o", "m", "p", "e", "n", "d", "enter")

	view := plain(m.View())
	if !strings.Contains(view, "#5862") {
		t.Errorf("the matching signal is missing under the query:\n%s", view)
	}
	if strings.Contains(view, "#6886") {
		t.Errorf("a signal the query does not match is still listed:\n%s", view)
	}
}

// Golden views of the Scan tab at the two widths §8 names.
func TestGoldenScanAt80Columns(t *testing.T) { goldenScan(t, 80) }

func TestGoldenScanAt140Columns(t *testing.T) { goldenScan(t, 140) }

func goldenScan(t *testing.T, width int) {
	t.Helper()

	teatest.RequireEqualOutput(t, []byte(plain(scanned(t, width, scanFixture()).View())))
}
