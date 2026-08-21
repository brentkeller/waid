package command

import (
	"fmt"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/tree"
)

// MoveResult is the item as it sits after the move, echoed back so --json callers need no follow-up
// read. Parent is null for an item moved to the top level, and ParentTitle names it for a reader
// who only has the fragment they typed.
type MoveResult struct {
	Id          string  `json:"id"`
	Title       string  `json:"title"`
	Parent      *string `json:"parent"`
	ParentTitle *string `json:"parentTitle"`
	Ts          string  `json:"ts"`
}

func runMove(ctx *cli.Ctx) (MoveResult, error) {
	id, err := requiredId(ctx, "move")
	if err != nil {
		return MoveResult{}, err
	}

	state := events.Load(ctx.Cfg.EventsPath)
	item, found := state.Find(id)
	if !found {
		return MoveResult{}, errs.Userf("unknown item id: %s", id)
	}

	parent, err := destination(ctx, state, item)
	if err != nil {
		return MoveResult{}, err
	}

	result := MoveResult{Id: item.Id, Title: item.Title}
	if parent != nil {
		result.Parent, result.ParentTitle = &parent.Id, &parent.Title
	}

	result.Ts, err = events.Append(
		ctx.Cfg.EventsPath,
		events.ParentEvent{Ev: "update", Id: item.Id, Parent: result.Parent},
		ctx.Now,
	)
	if err != nil {
		return MoveResult{}, err
	}
	return result, nil
}

func renderMove(data MoveResult, ctx *cli.Ctx) string {
	if data.ParentTitle == nil {
		return fmt.Sprintf("moved %s  %s  to the top level", data.Id, data.Title)
	}
	return fmt.Sprintf("moved %s  %s  under %s", data.Id, data.Title, *data.ParentTitle)
}

// destination reads where the item is being moved to: the item -p named, or nil for --top. A path
// is rejected rather than filed as an origin, since move reparents and a path is not a place in the
// tree; an item cannot be moved onto itself, nor onto anything already below it.
func destination(ctx *cli.Ctx, state events.State, item events.Item) (*events.Item, error) {
	requested, _ := ctx.Flags.String("project")
	requested = strings.TrimSpace(requested)
	top := ctx.Flags.Bool("top")

	switch {
	case top && requested != "":
		return nil, errs.Userf("move takes -p or --top, not both")
	case top:
		return nil, nil
	case requested == "":
		return nil, errs.Userf("move requires a destination: -p <fragment> or --top")
	}

	target, err := tree.Resolve(requested, state, ctx.Cwd)
	if err != nil {
		return nil, err
	}
	if target.Origin != nil {
		return nil, errs.Userf("move takes a parent, not a path: %s", requested)
	}

	parent, found := state.Find(*target.Parent)
	switch {
	case !found:
		return nil, errs.Userf("unknown item id: %s", *target.Parent)
	case parent.Id == item.Id:
		return nil, errs.Userf("an item cannot sit under itself: %s  %s", item.Id, item.Title)
	case hasDescendant(state, item.Id, parent.Id):
		return nil, errs.Userf(
			"an item cannot sit under its own descendant: %s  %s is below %s  %s",
			parent.Id, parent.Title, item.Id, item.Title,
		)
	}
	return &parent, nil
}

// hasDescendant reports whether target sits anywhere below root, walking down through
// State.Children. An id already visited is not walked twice, so a log holding a loop answers rather
// than spinning.
func hasDescendant(state events.State, root, target string) bool {
	seen := map[string]bool{root: true}
	for queue := []string{root}; len(queue) > 0; {
		current := queue[0]
		queue = queue[1:]
		for _, child := range state.Children(current) {
			if child.Id == target {
				return true
			}
			if seen[child.Id] {
				continue
			}
			seen[child.Id] = true
			queue = append(queue, child.Id)
		}
	}
	return false
}
