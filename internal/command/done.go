package command

import (
	"fmt"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
)

// DoneResult is the closed item, echoed back so --json callers need no follow-up read.
type DoneResult struct {
	Id     string        `json:"id"`
	Title  string        `json:"title"`
	Status events.Status `json:"status"`
	Ts     string        `json:"ts"`
}

func runDone(ctx *cli.Ctx) (DoneResult, error) {
	item, err := targetItem(ctx, "done")
	if err != nil {
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
