package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/tree"
)

// LoopsResult is everything still owed: the loops declared to waid and the ones detection found
// alongside them. Fields are declared in the order Node emits them, which is the order --json
// renders.
//
// Items stays flat, as list's does: it is the set the filters selected, and a --json caller that
// wants the hierarchy reads each item's parent. The forest the text rendering nests is held beside
// it, unexported so the two surfaces share one build without --json growing a second copy of every
// item.
type LoopsResult struct {
	Detected       []detect.Signal `json:"detected"`
	DismissedCount int             `json:"dismissedCount"`
	Notes          []string        `json:"notes"`
	Items          []events.Item   `json:"items"`

	roots []tree.Node
}

func runLoops(ctx *cli.Ctx) (LoopsResult, error) {
	state := events.Load(ctx.Cfg.EventsPath)
	result := detectForLoops(ctx, state)

	requested, _ := ctx.Flags.String("project")
	target, err := tree.Resolve(requested, state, ctx.Cwd)
	if err != nil {
		return LoopsResult{}, err
	}

	// Resolved after detection so an --origin partial can name a repo waid has only ever detected,
	// matching scan. A -p carrying a path asks the same question -p carries everywhere — provenance
	// rather than a place in the tree — so it is answered here rather than against the item titles.
	origin, err := resolveSignalPath(ctx, state, result.Detected, originRequest(ctx, requested, target))
	if err != nil {
		return LoopsResult{}, err
	}

	roots, _ := tree.Build(state)
	roots = tree.Prune(roots, isOpen)
	if origin != nil {
		roots = tree.Filter(roots, func(item events.Item) bool {
			return item.Origin != nil && *item.Origin == *origin
		})
	}
	if target.Parent != nil {
		roots = tree.Subtree(roots, *target.Parent)
	}

	// Empty headings are dropped after the filters and before bucketing, so a shelf with nothing on
	// it stays out of the view and a level whose only head was one renders its leaves directly. The
	// drop runs ahead of declared, so --headings reaches the flat item set as well as the tree.
	if !ctx.Flags.Bool("headings") {
		roots = tree.DropEmptyHeadings(roots)
	}

	result.Detected = signalsIn(result.Detected, origin)
	result.Items = declared(roots)
	result.roots = tree.Bucket(roots)
	return result, nil
}

// originRequest is the path the view is narrowed to, as typed. --origin says it outright; -p says
// it only when the resolver read its value as a path rather than as an item title, which is what
// keeps the `waid loops -p .` form working now that -p otherwise names a parent.
func originRequest(ctx *cli.Ctx, requested string, target tree.Target) string {
	if explicit, _ := ctx.Flags.String("origin"); explicit != "" {
		return explicit
	}
	if target.Origin != nil {
		return requested
	}
	return ""
}

// isOpen reports whether an item is still owed: closing it is the only way out of the list.
func isOpen(item events.Item) bool { return item.Status != events.StatusDone }

// declared flattens the rendered forest back to the items in it, oldest first. Reading the flat set
// off the tree rather than filtering the log a second time is what keeps --json and the text
// rendering answering with the same items.
func declared(nodes []tree.Node) []events.Item {
	items := []events.Item{}
	var walk func([]tree.Node)
	walk = func(level []tree.Node) {
		for _, node := range level {
			if !node.Synthetic {
				items = append(items, node.Item)
			}
			walk(node.Children)
		}
	}
	walk(nodes)

	slices.SortStableFunc(items, func(a, b events.Item) int {
		return strings.Compare(a.Updated, b.Updated)
	})
	return items
}

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
	if len(data.roots) == 0 {
		lines = append(lines, "no open loops")
	} else {
		lines = append(lines, "OPEN LOOPS", "")
		lines = append(lines, treeLines(data.roots, ctx.Now)...)
	}

	// Detected signals stay a section of their own: they are guesses, and merging them into the
	// declared items would blur which is which.
	if detected := detectedSection(data.Detected, data.DismissedCount); len(detected) > 0 {
		lines = append(lines, "")
		lines = append(lines, detected...)
	}

	return strings.Join(append(lines, noteLines(data.Notes)...), "\n")
}
