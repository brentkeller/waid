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
	"sync":       cli.Module[SyncResult]{Run: runSync, Render: renderSync},
	"doctor":     cli.Module[DoctorResult]{Run: runDoctor, Render: renderDoctor},
	"add":        cli.Module[AddResult]{Run: runAdd, Render: renderAdd},
	"done":       cli.Module[DoneResult]{Run: runDone, Render: renderDone},
	"move":       cli.Module[MoveResult]{Run: runMove, Render: renderMove},
	"reopen":     cli.Module[ReopenResult]{Run: runReopen, Render: renderReopen},
	"note":       cli.Module[NoteResult]{Run: runNote, Render: renderNote},
	"list":       cli.Module[ListResult]{Run: runList, Render: renderList},
	"show":       cli.Module[ShowResult]{Run: runShow, Render: renderShow},
	"today":      cli.Module[TodayResult]{Run: runToday, Render: renderToday, NeedsSessions: true},
	"transcript": cli.Module[TranscriptResult]{Run: runTranscript, Render: renderTranscript, NeedsSessions: true},
	"week":       cli.Module[WeekResult]{Run: runWeek, Render: renderWeek, NeedsSessions: true},
	"scan":       cli.Module[ScanResult]{Run: runScan, Render: renderScan, NeedsSessions: true},
	"loops":      cli.Module[LoopsResult]{Run: runLoops, Render: renderLoops, NeedsSessions: true},
	"dismiss":    cli.Module[DismissResult]{Run: runDismiss, Render: renderDismiss},
	"undismiss":  cli.Module[UndismissResult]{Run: runUndismiss, Render: renderUndismiss},
	"promote":    cli.Module[PromoteResult]{Run: runPromote, Render: renderPromote, NeedsSessions: true},
	"ui":         cli.Module[UiResult]{Run: runUi, Render: renderUi},
}

// requiredId reads the item id a mutation operates on from the first positional.
func requiredId(ctx *cli.Ctx, command string) (string, error) {
	ids, err := requiredIds(ctx, command)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// requiredIds reads the item ids a mutation operates on from the positionals, in the order given.
func requiredIds(ctx *cli.Ctx, command string) ([]string, error) {
	ids := []string{}
	for _, arg := range ctx.Args {
		if id := strings.TrimSpace(arg); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, errs.Userf("%s requires an item id", command)
	}
	return ids, nil
}

// requireItem looks an item up in a folded log, rejecting the input when the id is unknown.
func requireItem(state events.State, id string) (events.Item, error) {
	item, found := state.Find(id)
	if !found {
		return events.Item{}, errs.Userf("unknown item id: %s", id)
	}
	return item, nil
}

// targetItem resolves the item a mutation taking nothing but an id operates on.
func targetItem(ctx *cli.Ctx, command string) (events.Item, error) {
	return targetItemIn(events.Load(ctx.Cfg.EventsPath), ctx, command)
}

// targetItemIn is targetItem against a log the caller has already folded, for the mutations that
// ask the log a further question about the item before writing.
func targetItemIn(state events.State, ctx *cli.Ctx, command string) (events.Item, error) {
	id, err := requiredId(ctx, command)
	if err != nil {
		return events.Item{}, err
	}
	return requireItem(state, id)
}

// targetItemsIn is targetItemIn for a mutation taking several ids. Every id is resolved before the
// caller writes anything, so an unknown id anywhere in the list cannot leave a half-finished
// mutation behind.
func targetItemsIn(state events.State, ctx *cli.Ctx, command string) ([]events.Item, error) {
	ids, err := requiredIds(ctx, command)
	if err != nil {
		return nil, err
	}

	items := make([]events.Item, 0, len(ids))
	for _, id := range ids {
		item, err := requireItem(state, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
