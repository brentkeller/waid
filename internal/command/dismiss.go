package command

import (
	"slices"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/triage"
)

// DismissResult is the key as hidden, echoed back so --json callers need no follow-up read. Fields
// are declared in the order Node emits them, which is the order --json renders.
type DismissResult struct {
	Key       string `json:"key"`
	Dismissed bool   `json:"dismissed"`
	// Already reports whether the key was already hidden, in which case nothing was written.
	Already bool `json:"already"`
}

// runDismiss hides a detected signal for good. The key is not validated against detection: a signal
// only has to have existed once to be worth silencing, and detection is expensive enough that
// requiring a live match would make dismissing an intermittent signal a matter of timing.
func runDismiss(ctx *cli.Ctx) (DismissResult, error) {
	key, err := requiredKey(ctx, "dismiss")
	if err != nil {
		return DismissResult{}, err
	}

	state := events.Load(ctx.Cfg.EventsPath)
	if slices.Contains(state.Dismissed, key) {
		return DismissResult{Key: key, Dismissed: true, Already: true}, nil
	}

	if err := triage.Dismiss(ctx.Cfg.EventsPath, key, ctx.Now); err != nil {
		return DismissResult{}, err
	}
	return DismissResult{Key: key, Dismissed: true, Already: false}, nil
}

func renderDismiss(data DismissResult, _ *cli.Ctx) string {
	if data.Already {
		return "already dismissed " + data.Key
	}
	return "dismissed " + data.Key
}

// requiredKey reads the signal key a command operates on from the first positional.
func requiredKey(ctx *cli.Ctx, command string) (string, error) {
	key := ""
	if len(ctx.Args) > 0 {
		key = strings.TrimSpace(ctx.Args[0])
	}
	if key == "" {
		return "", errs.Userf("%s requires a signal key", command)
	}
	return key, nil
}
