package command

import (
	"slices"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// UndismissResult is the key as restored, echoed back so --json callers need no follow-up read.
type UndismissResult struct {
	Key         string `json:"key"`
	Undismissed bool   `json:"undismissed"`
}

// runUndismiss restores a key hidden by dismiss, so the signal is reported again the next time
// detection finds it. The key has to be dismissed now: undismissing something that was never hidden
// would append an event that folds to nothing, and the likeliest cause is a mistyped key.
func runUndismiss(ctx *cli.Ctx) (UndismissResult, error) {
	key, err := requiredKey(ctx, "undismiss")
	if err != nil {
		return UndismissResult{}, err
	}

	state := events.Load(ctx.Cfg.EventsPath)
	if !slices.Contains(state.Dismissed, key) {
		return UndismissResult{}, errs.Userf("unknown dismissed key: %s", key)
	}

	if _, err := events.Append(ctx.Cfg.EventsPath, events.UndismissEvent{Ev: "undismiss", Key: key}, ctx.Now); err != nil {
		return UndismissResult{}, err
	}
	return UndismissResult{Key: key, Undismissed: true}, nil
}

func renderUndismiss(data UndismissResult, _ *cli.Ctx) string {
	return "undismissed " + data.Key
}
