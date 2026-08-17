// Package command holds one module per waid command, each pairing the data it produces with the
// text that data renders as.
package command

import (
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// Registry is the dispatch table: every command waid answers to is listed here, and anything absent
// from it is an unknown command.
var Registry = cli.Registry{
	"add":    cli.Module[AddResult]{Run: runAdd, Render: renderAdd},
	"done":   cli.Module[DoneResult]{Run: runDone, Render: renderDone},
	"reopen": cli.Module[ReopenResult]{Run: runReopen, Render: renderReopen},
	"note":   cli.Module[NoteResult]{Run: runNote, Render: renderNote},
	"list":   cli.Module[ListResult]{Run: runList, Render: renderList},
	"show":   cli.Module[ShowResult]{Run: runShow, Render: renderShow},
	"today":  cli.Module[TodayResult]{Run: runToday, Render: renderToday, NeedsSessions: true},
	"week":   cli.Module[WeekResult]{Run: runWeek, Render: renderWeek, NeedsSessions: true},
}

// requiredId reads the item id a mutation operates on from the first positional.
func requiredId(ctx *cli.Ctx, command string) (string, error) {
	id := ""
	if len(ctx.Args) > 0 {
		id = strings.TrimSpace(ctx.Args[0])
	}
	if id == "" {
		return "", errs.Userf("%s requires an item id", command)
	}
	return id, nil
}

// requireItem looks an item up by id, rejecting the input when the id is unknown.
func requireItem(ctx *cli.Ctx, id string) (events.Item, error) {
	item, found := events.Load(ctx.Cfg.EventsPath).Find(id)
	if !found {
		return events.Item{}, errs.Userf("unknown item id: %s", id)
	}
	return item, nil
}

// targetItem resolves the item a mutation taking nothing but an id operates on.
func targetItem(ctx *cli.Ctx, command string) (events.Item, error) {
	id, err := requiredId(ctx, command)
	if err != nil {
		return events.Item{}, err
	}
	return requireItem(ctx, id)
}
