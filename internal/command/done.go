package command

import (
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/tree"
)

// ClosedItem is one closed item, echoed back so --json callers need no follow-up read.
type ClosedItem struct {
	Id     string        `json:"id"`
	Title  string        `json:"title"`
	Status events.Status `json:"status"`
	Ts     string        `json:"ts"`
}

// DoneResult is every item the run closed, in the order the ids were given. It is an array whatever
// the count, so a caller reading the payload does not branch on how many ids it passed.
type DoneResult []ClosedItem

func runDone(ctx *cli.Ctx) (DoneResult, error) {
	state := events.Load(ctx.Cfg.EventsPath)
	items, err := targetItemsIn(state, ctx, "done")
	if err != nil {
		return DoneResult{}, err
	}

	// A heading closes only once the work beneath it is finished (§5). The guard is shared with the
	// app so the two surfaces cannot disagree about when that is, and it judges the whole run before
	// a line is written so a refusal cannot leave part of the list closed.
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Id)
	}
	if err := tree.GuardCloses(state, ids); err != nil {
		return DoneResult{}, err
	}

	closed := make(DoneResult, 0, len(items))
	for _, item := range items {
		ts, err := events.Append(ctx.Cfg.EventsPath, events.CloseEvent{Ev: "close", Id: item.Id}, ctx.Now)
		if err != nil {
			return DoneResult{}, err
		}
		closed = append(closed, ClosedItem{Id: item.Id, Title: item.Title, Status: events.StatusDone, Ts: ts})
	}
	return closed, nil
}

func renderDone(data DoneResult, ctx *cli.Ctx) string {
	lines := make([]string, 0, len(data))
	for _, item := range data {
		lines = append(lines, "closed "+item.Id+"  "+item.Title)
	}
	return strings.Join(lines, "\n")
}
