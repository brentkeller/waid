package command

import (
	"fmt"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
)

// HeadingResult is the item as it sits after the mark, echoed back so --json callers need no
// follow-up read. Heading is the value the item now holds, not the one it held before.
type HeadingResult struct {
	Id      string `json:"id"`
	Title   string `json:"title"`
	Heading bool   `json:"heading"`
	Ts      string `json:"ts"`
}

// runHeading marks an item a heading, or unmarks it with --off. The event is written whatever the
// item already held: the log is a history of what was asked for and the fold is idempotent, so a
// command that sometimes wrote and sometimes did not would leave undo guessing.
func runHeading(ctx *cli.Ctx) (HeadingResult, error) {
	item, err := targetItem(ctx, "heading")
	if err != nil {
		return HeadingResult{}, err
	}

	heading := !ctx.Flags.Bool("off")
	ts, err := events.Append(
		ctx.Cfg.EventsPath,
		events.HeadingEvent{Ev: "update", Id: item.Id, Heading: heading},
		ctx.Now,
	)
	if err != nil {
		return HeadingResult{}, err
	}
	return HeadingResult{Id: item.Id, Title: item.Title, Heading: heading, Ts: ts}, nil
}

func renderHeading(data HeadingResult, _ *cli.Ctx) string {
	if !data.Heading {
		return fmt.Sprintf("unmarked %s  %s", data.Id, data.Title)
	}
	return fmt.Sprintf("marked %s  %s  a heading", data.Id, data.Title)
}
