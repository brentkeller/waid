package command

import (
	"fmt"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
)

// ReopenResult is the reopened item. reopen also clears waitingOn, so a waiting item comes back
// plain open.
type ReopenResult struct {
	Id     string        `json:"id"`
	Title  string        `json:"title"`
	Status events.Status `json:"status"`
	Ts     string        `json:"ts"`
}

func runReopen(ctx *cli.Ctx) (ReopenResult, error) {
	item, err := targetItem(ctx, "reopen")
	if err != nil {
		return ReopenResult{}, err
	}

	ts, err := events.Append(ctx.Cfg.EventsPath, events.ReopenEvent{Ev: "reopen", Id: item.Id}, ctx.Now)
	if err != nil {
		return ReopenResult{}, err
	}
	return ReopenResult{Id: item.Id, Title: item.Title, Status: events.StatusOpen, Ts: ts}, nil
}

func renderReopen(data ReopenResult, ctx *cli.Ctx) string {
	return fmt.Sprintf("reopened %s  %s", data.Id, data.Title)
}
