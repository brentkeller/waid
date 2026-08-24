package command

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/project"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/tree"
)

// ListFilters are the filters that produced a list, echoed back so --json callers can see what was
// applied.
type ListFilters struct {
	Status   *events.Status `json:"status"`
	Origin   *string        `json:"origin"`
	Tag      []string       `json:"tag"`
	All      bool           `json:"all"`
	Headings bool           `json:"headings"`
}

// ListResult is the filtered items alongside the filters that selected them. Items stays flat: it
// is the set the filters selected, and a --json caller that wants the hierarchy reads each item's
// parent. The forest the text rendering nests is held beside it, unexported so the two surfaces
// share one build without --json growing a second copy of every item.
type ListResult struct {
	Items   []events.Item `json:"items"`
	Filters ListFilters   `json:"filters"`

	roots []tree.Node
}

// Column widths for the shared item row; titles beyond the width are ellipsized, not wrapped.
const (
	idWidth     = 4
	statusWidth = 10
	titleWidth  = 44
	metaWidth   = 10
)

// maxIndentDepth is the deepest generation that still earns an indent of its own, so a long branch
// cannot squeeze the title column away.
const maxIndentDepth = 4

func runList(ctx *cli.Ctx) (ListResult, error) {
	state := events.Load(ctx.Cfg.EventsPath)

	status, err := readStatus(ctx.Flags.String("status"))
	if err != nil {
		return ListResult{}, err
	}
	requested, _ := ctx.Flags.String("origin")
	path, err := project.Resolve(requested, project.KnownProjects(state.Items, nil), ctx.Cwd)
	if err != nil {
		return ListResult{}, err
	}

	tags := ctx.Flags.List("tag")
	if tags == nil {
		tags = []string{}
	}
	filters := ListFilters{
		Status:   status,
		Origin:   path,
		Tag:      tags,
		All:      ctx.Flags.Bool("all"),
		Headings: ctx.Flags.Bool("headings"),
	}

	items := []events.Item{}
	for _, item := range state.Items {
		if matches(item, filters) {
			items = append(items, item)
		}
	}
	slices.SortStableFunc(items, func(a, b events.Item) int {
		return strings.Compare(a.Updated, b.Updated)
	})

	return ListResult{Items: items, Filters: filters, roots: forest(state, filters)}, nil
}

// forest builds the tree the text rendering walks. The two kinds of filter are applied separately
// because they mean different things: status gates each row in its own right, so a done row under
// an open parent still goes, while a tag or an origin is a query, and a heading that answers it is
// asked for along with everything under it. Empty headings are dropped next, unless --headings asked
// for them, so a shelf with nothing on it stays out of the view. Bucketing comes last, so
// (unassigned) is computed against what survived rather than against the log.
func forest(state events.State, filters ListFilters) []tree.Node {
	roots, _ := tree.Build(state)
	roots = tree.Prune(roots, func(item events.Item) bool { return passesStatus(item, filters) })
	roots = tree.Filter(roots, func(item events.Item) bool { return passesQuery(item, filters) })
	if !filters.Headings {
		roots = tree.DropEmptyHeadings(roots)
	}
	return tree.Bucket(roots)
}

// matches reports whether an item survives every filter. Filters are conjunctive.
func matches(item events.Item, filters ListFilters) bool {
	return passesStatus(item, filters) && passesQuery(item, filters)
}

// passesStatus applies the status gate. An explicit --status speaks for itself, so it overrides the
// done gate.
func passesStatus(item events.Item, filters ListFilters) bool {
	if filters.Status != nil {
		return item.Status == *filters.Status
	}
	return filters.All || item.Status != events.StatusDone
}

// passesQuery applies the filters that ask a question of an item rather than of its state.
func passesQuery(item events.Item, filters ListFilters) bool {
	if filters.Origin != nil && (item.Origin == nil || *item.Origin != *filters.Origin) {
		return false
	}
	for _, tag := range filters.Tag {
		if !slices.Contains(item.Tags, tag) {
			return false
		}
	}
	return true
}

// readStatus narrows the --status value, which an unpassed flag leaves absent rather than empty.
func readStatus(value string, passed bool) (*events.Status, error) {
	if !passed {
		return nil, nil
	}
	if !events.IsStatus(value) {
		names := make([]string, 0, len(events.Statuses))
		for _, status := range events.Statuses {
			names = append(names, string(status))
		}
		return nil, errs.Userf("unknown status: %s (expected %s)", value, strings.Join(names, ", "))
	}
	status := events.Status(value)
	return &status, nil
}

func renderList(data ListResult, ctx *cli.Ctx) string {
	if len(data.Items) == 0 {
		return "no items"
	}

	return strings.Join(treeLines(data.roots, ctx.Now), "\n")
}

// treeLines renders a whole forest, shared with waid loops so the two nest identically.
func treeLines(roots []tree.Node, now time.Time) []string {
	lines := []string{}
	nested := false
	for _, root := range roots {
		block := TreeLines(root, now, 0)
		// A branch is given air on either side of it; a run of bare top-level rows is not, since
		// blank lines between single rows is the wall of text the tree replaces.
		if len(lines) > 0 && (len(block) > 1 || nested) {
			lines = append(lines, "")
		}
		lines = append(lines, block...)
		nested = len(block) > 1
	}
	return lines
}

// TreeLines renders one node and everything under it, a level of indent per generation. A synthetic
// node is a heading rather than a row: it holds no id, status or age to put in the columns.
func TreeLines(node tree.Node, now time.Time, depth int) []string {
	lines := []string{indentFor(depth) + node.Item.Title}
	if !node.Synthetic {
		lines[0] = ItemLine(node.Item, now, depth)
	}
	for _, child := range node.Children {
		lines = append(lines, TreeLines(child, now, depth+1)...)
	}
	return lines
}

// indentFor is the leading whitespace of a row at a given depth. Roots are indented one level, so
// the whole listing sits clear of the left margin. Indent is capped, since past a few levels the
// nesting is already plain from the rows above and the title column is worth more than the
// depth-for-depth fidelity.
func indentFor(depth int) string { return strings.Repeat("  ", min(depth, maxIndentDepth)+1) }

// titleFor is the title column at a given depth. The indent is taken out of the title rather than
// added to the row, so the meta and age columns stay in one place down a branch.
func titleFor(depth int) int {
	return titleWidth - 2*min(depth, maxIndentDepth)
}

// ItemLine renders one item as an aligned row, indented to its depth in the tree. Shared with waid
// loops.
func ItemLine(item events.Item, now time.Time, depth int) string {
	parts := []string{}
	if len(item.Tags) > 0 {
		parts = append(parts, "["+strings.Join(item.Tags, ",")+"]")
	}
	if item.WaitingOn != nil {
		parts = append(parts, "← "+*item.WaitingOn)
	}

	row := render.Pad(item.Id, idWidth) +
		"  " +
		render.Pad(string(item.Status), statusWidth) +
		render.Pad(item.Title, titleFor(depth)) +
		"  " +
		render.Pad(strings.Join(parts, " "), metaWidth) +
		render.RelTime(item.Updated, now)

	return strings.TrimRightFunc(indentFor(depth)+row, unicode.IsSpace)
}
