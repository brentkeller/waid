package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
)

// LoopsResult is everything still owed: the loops declared to waid, grouped by project, and the ones
// detection found alongside them. Fields are declared in the order Node emits them, which is the
// order --json renders.
type LoopsResult struct {
	Detected       []detect.Signal `json:"detected"`
	DismissedCount int             `json:"dismissedCount"`
	Notes          []string        `json:"notes"`
	Groups         []project.Group `json:"groups"`
}

func runLoops(ctx *cli.Ctx) (LoopsResult, error) {
	state := events.Load(ctx.Cfg.EventsPath)
	result := detectForLoops(ctx, state)

	// Resolved after detection so a -p partial can name a repo waid has only ever detected, matching
	// scan.
	path, err := resolveSignalProject(ctx, state, result.Detected)
	if err != nil {
		return LoopsResult{}, err
	}

	items := []events.Item{}
	for _, item := range state.Items {
		if isOpen(item) && (path == nil || (item.Project != nil && *item.Project == *path)) {
			items = append(items, item)
		}
	}
	slices.SortStableFunc(items, func(a, b events.Item) int {
		return strings.Compare(a.Updated, b.Updated)
	})

	result.Detected = signalsIn(result.Detected, path)
	result.Groups = project.GroupByProject(items)
	return result, nil
}

// isOpen reports whether an item is still owed: closing it is the only way out of the list.
func isOpen(item events.Item) bool { return item.Status != events.StatusDone }

// detectForLoops runs detection as a bonus on top of the declared items, so a failure costs a note
// rather than the command: whatever waid was told about is still worth showing.
func detectForLoops(ctx *cli.Ctx, state events.State) (result LoopsResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = LoopsResult{
				Detected: []detect.Signal{},
				Notes:    []string{fmt.Sprintf("detection failed (%v); showing declared items only", recovered)},
			}
		}
	}()

	found := detect.Signals(ctx.Cfg, detectionDeps(ctx, state))
	return LoopsResult{Detected: found.Signals, DismissedCount: found.DismissedCount, Notes: found.Notes}
}

func renderLoops(data LoopsResult, ctx *cli.Ctx) string {
	lines := []string{}
	if len(data.Groups) == 0 {
		lines = append(lines, "no open loops")
	} else {
		lines = append(lines, "OPEN LOOPS")
		for _, group := range data.Groups {
			heading := "(no project)"
			if group.Project != nil {
				heading = *group.Project
			}
			lines = append(lines, "", "  "+heading)
			for _, item := range group.Items {
				lines = append(lines, ItemLine(item, ctx.Now))
			}
		}
	}

	// Detected signals stay a section of their own: they are guesses, and merging them into the
	// declared items would blur which is which.
	if detected := detectedSection(data.Detected, data.DismissedCount); len(detected) > 0 {
		lines = append(lines, "")
		lines = append(lines, detected...)
	}

	return strings.Join(append(lines, noteLines(data.Notes)...), "\n")
}
