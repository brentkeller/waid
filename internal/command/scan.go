package command

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/sessions"
)

// ScanResult is one detection pass. Fields are declared in the order Node emits them, which is the
// order --json renders.
type ScanResult struct {
	Signals []detect.Signal `json:"signals"`
	// DismissedCount is how many signals were suppressed because their key was dismissed, counted
	// before -p narrowing.
	DismissedCount int `json:"dismissedCount"`
	// Notes are non-fatal degradations, today only an unavailable gh.
	Notes []string `json:"notes"`
}

// detectedFooter names the two commands a listed signal can be taken to.
const detectedFooter = "waid promote <key> to track · waid dismiss <key> to hide"

// subjectWidth is the width of the subject column, matching the title column waid list renders
// items in.
const subjectWidth = titleWidth

// subjectGap is the blank columns between the subject and the meta that follows it.
const subjectGap = 2

// detectedHeaderLines are the lines detectedSection puts above the signals: the header and the blank
// beneath it.
const detectedHeaderLines = 2

func runScan(ctx *cli.Ctx) (ScanResult, error) {
	state := events.Load(ctx.Cfg.EventsPath)
	detected := detect.Signals(ctx.Cfg, detectionDeps(ctx, state))

	path, err := resolveSignalProject(ctx, state, detected.Signals)
	if err != nil {
		return ScanResult{}, err
	}

	return ScanResult{
		Signals:        signalsIn(detected.Signals, path),
		DismissedCount: detected.DismissedCount,
		Notes:          detected.Notes,
	}, nil
}

// detectionDeps are the inputs and seams a command hands detection.
func detectionDeps(ctx *cli.Ctx, state events.State) detect.Deps {
	return detect.Deps{
		Sessions: ctx.Sessions,
		State:    state,
		Now:      ctx.Now,
		Git:      ctx.Git,
		Gh:       ctx.Gh,
	}
}

// resolveSignalProject resolves -p against the known projects extended with the repos detection just
// found. A repo waid has never recorded an item or session against is still a legitimate target for
// -p, which is why this runs after detection rather than before it.
func resolveSignalProject(ctx *cli.Ctx, state events.State, signals []detect.Signal) (*string, error) {
	known := project.KnownProjects(state.Items, sessionProjects(ctx.Sessions))
	for _, signal := range signals {
		if signal.Project != nil && !slices.Contains(known, *signal.Project) {
			known = append(known, *signal.Project)
		}
	}

	requested, _ := ctx.Flags.String("project")
	return project.Resolve(requested, known, ctx.Cwd)
}

// signalsIn keeps the signals belonging to path, or every signal when no project was named.
func signalsIn(signals []detect.Signal, path *string) []detect.Signal {
	if path == nil {
		return signals
	}

	kept := []detect.Signal{}
	for _, signal := range signals {
		if signal.Project != nil && *signal.Project == *path {
			kept = append(kept, signal)
		}
	}
	return kept
}

// sessionProjects are the project paths the harvested sessions ran in, in order.
func sessionProjects(harvested []sessions.CachedSession) []string {
	projects := make([]string, 0, len(harvested))
	for _, session := range harvested {
		if session.Project != nil {
			projects = append(projects, *session.Project)
		}
	}
	return projects
}

// detectedSection is the DETECTED block, shared with loops so the two render signals identically. It
// returns no lines at all when there is nothing detected and nothing dismissed, leaving the caller to
// decide what to say instead.
//
// Each signal is three columns: the key it is addressed by, what it is about, and where and when. The
// key column widens to fit the longest key, so the other two stay aligned down the block.
func detectedSection(signals []detect.Signal, dismissedCount int) []string {
	if len(signals) == 0 && dismissedCount == 0 {
		return nil
	}

	width := 0
	for _, signal := range signals {
		width = max(width, render.Width(signal.Key))
	}
	width += 2

	lines := make([]string, 0, len(signals)+detectedHeaderLines+2)
	lines = append(lines, fmt.Sprintf("DETECTED  (%d shown, %d dismissed)", len(signals), dismissedCount), "")
	for _, signal := range signals {
		// Padded separately from the gap so a truncated subject still keeps the columns apart.
		row := "  " + render.Pad(signal.Key, width) +
			render.Pad(signal.Subject, subjectWidth) +
			strings.Repeat(" ", subjectGap) +
			signal.Detail
		lines = append(lines, strings.TrimRightFunc(row, unicode.IsSpace))
	}
	return append(lines, "", "  "+detectedFooter)
}

// noteLines are the degradation notes in the shape both commands print them: each set off from
// whatever came before it.
func noteLines(notes []string) []string {
	lines := make([]string, 0, len(notes)*2)
	for _, note := range notes {
		lines = append(lines, "", "  "+note)
	}
	return lines
}

func renderScan(data ScanResult, _ *cli.Ctx) string {
	lines := detectedSection(data.Signals, data.DismissedCount)
	if len(lines) == 0 {
		lines = []string{"nothing detected"}
	}
	return strings.Join(append(lines, noteLines(data.Notes)...), "\n")
}
