package command

import (
	"fmt"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/triage"
)

// PromoteResult is the item the signal became, echoed back so --json callers need no follow-up read.
// Fields are declared in the order Node emits them, which is the order --json renders.
type PromoteResult struct {
	Id     string  `json:"id"`
	Key    string  `json:"key"`
	Title  string  `json:"title"`
	Origin *string `json:"origin"`
}

// runPromote turns a detected signal into a declared item, then dismisses the key so the loop is not
// reported twice. The signal must still be detectable — an already-dismissed or vanished key is
// rejected rather than guessed at, since the item's title and origin can only come from the live
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
			// PromoteResult is Promotion with the --json tags, so the write's own result converts.
			promoted, err := triage.Promote(ctx.Cfg.EventsPath, state, ctx.Ids, signal, signal.Title, ctx.Now)
			return PromoteResult(promoted), err
		}
	}
	return PromoteResult{}, errs.Userf("unknown signal key: %s", key)
}

func renderPromote(data PromoteResult, _ *cli.Ctx) string {
	return fmt.Sprintf("promoted %s  %s", data.Id, data.Title)
}
