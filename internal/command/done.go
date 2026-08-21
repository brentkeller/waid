package command

import (
	"fmt"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/tree"
)

// DoneResult is the closed item, echoed back so --json callers need no follow-up read.
type DoneResult struct {
	Id     string        `json:"id"`
	Title  string        `json:"title"`
	Status events.Status `json:"status"`
	Ts     string        `json:"ts"`
}

func runDone(ctx *cli.Ctx) (DoneResult, error) {
	state := events.Load(ctx.Cfg.EventsPath)
	item, err := targetItemIn(state, ctx, "done")
	if err != nil {
		return DoneResult{}, err
	}
	// A heading closes only once the work beneath it is finished (§5). The guard is shared with the
	// app so the two surfaces cannot disagree about when that is.
	if err := tree.GuardClose(state, item.Id); err != nil {
		return DoneResult{}, err
	}

	ts, err := events.Append(ctx.Cfg.EventsPath, events.CloseEvent{Ev: "close", Id: item.Id}, ctx.Now)
	if err != nil {
		return DoneResult{}, err
	}
	return DoneResult{Id: item.Id, Title: item.Title, Status: events.StatusDone, Ts: ts}, nil
}

func renderDone(data DoneResult, ctx *cli.Ctx) string {
	return fmt.Sprintf("closed %s  %s", data.Id, data.Title)
}
