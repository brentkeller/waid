package command

import (
	"fmt"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/tree"
)

// AddResult is the item as recorded, echoed back so --json callers need no follow-up read. Parent
// and Origin are both here because -p resolves to one or the other, and a caller cannot tell which
// from the value it passed.
type AddResult struct {
	Id        string        `json:"id"`
	Title     string        `json:"title"`
	Status    events.Status `json:"status"`
	Parent    *string       `json:"parent"`
	Origin    *string       `json:"origin"`
	Tags      []string      `json:"tags"`
	WaitingOn *string       `json:"waitingOn"`
	Session   *string       `json:"session"`
	Created   string        `json:"created"`
}

func runAdd(ctx *cli.Ctx) (AddResult, error) {
	title := strings.TrimSpace(strings.Join(ctx.Args, " "))
	if title == "" {
		return AddResult{}, errs.Userf("add requires a title")
	}

	state := events.Load(ctx.Cfg.EventsPath)
	target, err := filingTarget(ctx, state)
	if err != nil {
		return AddResult{}, err
	}

	waitingOn := optional(ctx, "waiting-on")
	status := events.StatusOpen
	if waitingOn != nil {
		status = events.StatusWaiting
	}

	// The log holds an empty array rather than null when no tags were given.
	tags := ctx.Flags.List("tag")
	if tags == nil {
		tags = []string{}
	}
	session := optional(ctx, "session")

	id, err := newId(ctx, state)
	if err != nil {
		return AddResult{}, err
	}

	ts, err := events.Append(ctx.Cfg.EventsPath, events.AddEvent{
		Ev:        "add",
		Id:        id,
		Title:     title,
		Status:    status,
		Parent:    target.Parent,
		Origin:    target.Origin,
		Session:   session,
		Tags:      tags,
		WaitingOn: waitingOn,
		Heading:   ctx.Flags.Bool("heading"),
	}, ctx.Now)
	if err != nil {
		return AddResult{}, err
	}

	return AddResult{
		Id:        id,
		Title:     title,
		Status:    status,
		Parent:    target.Parent,
		Origin:    target.Origin,
		Tags:      tags,
		WaitingOn: waitingOn,
		Session:   session,
		Created:   ts,
	}, nil
}

func renderAdd(data AddResult, ctx *cli.Ctx) string {
	return fmt.Sprintf("added %s  %s", data.Id, data.Title)
}

// filingTarget reads where an item is to be filed: --parent names the parent by id, -p by a title
// fragment or as a path. The two are exclusive, since each is a complete answer on its own.
func filingTarget(ctx *cli.Ctx, state events.State) (tree.Target, error) {
	requested, _ := ctx.Flags.String("project")
	id, byId := ctx.Flags.String("parent")
	switch {
	case !byId:
		return tree.Resolve(requested, state, ctx.Cwd)
	case strings.TrimSpace(requested) != "":
		return tree.Target{}, errs.Userf("-p and --parent both name where to file: pass one")
	}
	return tree.ResolveId(id, state)
}

// newId draws an id no item in the log already holds.
func newId(ctx *cli.Ctx, state events.State) (string, error) {
	return state.NewId(ctx.Ids)
}

// optional reads a flag that is stored as null when it was not passed at all.
func optional(ctx *cli.Ctx, name string) *string {
	value, passed := ctx.Flags.String(name)
	if !passed {
		return nil
	}
	return &value
}
