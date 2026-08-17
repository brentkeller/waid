package command

import (
	"fmt"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// PromoteResult is the item the signal became, echoed back so --json callers need no follow-up read.
// Fields are declared in the order Node emits them, which is the order --json renders.
type PromoteResult struct {
	Id      string  `json:"id"`
	Key     string  `json:"key"`
	Title   string  `json:"title"`
	Project *string `json:"project"`
}

// promotedTag is the tag every promoted item carries, so a detected loop stays distinguishable from
// a declared one.
const promotedTag = "promoted"

// runPromote turns a detected signal into a declared item, then dismisses the key so the loop is not
// reported twice. The signal must still be detectable — an already-dismissed or vanished key is
// rejected rather than guessed at, since the item's title and project can only come from the live
// signal.
func runPromote(ctx *cli.Ctx) (PromoteResult, error) {
	key, err := requiredKey(ctx, "promote")
	if err != nil {
		return PromoteResult{}, err
	}

	state := events.Load(ctx.Cfg.EventsPath)
	detected := detect.Signals(ctx.Cfg, detectionDeps(ctx, state))

	for _, signal := range detected.Signals {
		if signal.Key == key {
			return promoteSignal(ctx, state, signal, signal.Title)
		}
	}
	return PromoteResult{}, errs.Userf("unknown signal key: %s", key)
}

// promoteSignal writes the two events a promotion is made of: an add carrying the signal's project
// and the promoted tag, then a dismiss of the signal's key so the loop is not reported twice. title
// is passed separately so a caller that let the user edit it can use the edited text; state only
// supplies the ids already taken.
func promoteSignal(ctx *cli.Ctx, state events.State, signal detect.Signal, title string) (PromoteResult, error) {
	id, err := newId(ctx, state)
	if err != nil {
		return PromoteResult{}, err
	}

	if _, err := events.Append(ctx.Cfg.EventsPath, events.AddEvent{
		Ev:        "add",
		Id:        id,
		Title:     title,
		Status:    events.StatusOpen,
		Project:   signal.Project,
		Session:   nil,
		Tags:      []string{promotedTag},
		WaitingOn: nil,
	}, ctx.Now); err != nil {
		return PromoteResult{}, err
	}
	if _, err := events.Append(ctx.Cfg.EventsPath, events.DismissEvent{Ev: "dismiss", Key: signal.Key}, ctx.Now); err != nil {
		return PromoteResult{}, err
	}

	return PromoteResult{Id: id, Key: signal.Key, Title: title, Project: signal.Project}, nil
}

func renderPromote(data PromoteResult, _ *cli.Ctx) string {
	return fmt.Sprintf("promoted %s  %s", data.Id, data.Title)
}
