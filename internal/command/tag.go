package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// TagResult is the item as it sits after the write, echoed back so --json callers need no follow-up
// read. Tags is the set the item now carries, not the one it carried before.
type TagResult struct {
	Id    string   `json:"id"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Ts    string   `json:"ts"`
}

// runTag edits the tags on an item: --tag adds, --remove takes off, --off clears. The command names
// a change rather than a set because it cannot show the caller what the item already carries, so a
// replacement typed blind would drop whatever it had forgotten about. The app's own key replaces
// instead, since there the current set is in the input being edited.
//
// The event carries the resulting set whatever the change was: the log records what an item's tags
// became, not the delta that got them there, so the fold reads one line rather than replaying every
// edit behind it.
func runTag(ctx *cli.Ctx) (TagResult, error) {
	item, err := targetItem(ctx, "tag")
	if err != nil {
		return TagResult{}, err
	}

	added := events.NormalizeTags(ctx.Flags.List("tag"))
	removed := events.NormalizeTags(ctx.Flags.List("remove"))
	off := ctx.Flags.Bool("off")

	// --off says what the set is and the other two say how to change it, which are two different
	// intentions. Neither flag at all states none, and is refused rather than read as a clear: a bare
	// `waid tag <id>` is a half-typed command far more often than a request to wipe the set.
	switch {
	case off && len(added)+len(removed) > 0:
		return TagResult{}, errs.Userf("--off clears every tag, so it cannot be combined with --tag or --remove")
	case !off && len(added)+len(removed) == 0:
		return TagResult{}, errs.Userf("tag requires --tag, --remove or --off")
	}

	tags := []string{}
	if !off {
		// The removals are applied first so that a tag named to both flags survives: --tag was asked
		// for explicitly, and a command that answered it with an absence would be answering the wrong
		// half of what it was told.
		for _, tag := range item.Tags {
			if !slices.Contains(removed, tag) {
				tags = append(tags, tag)
			}
		}
		tags = events.NormalizeTags(append(tags, added...))
	}

	ts, err := events.Append(
		ctx.Cfg.EventsPath,
		events.TagsEvent{Ev: "update", Id: item.Id, Tags: tags},
		ctx.Now,
	)
	if err != nil {
		return TagResult{}, err
	}
	return TagResult{Id: item.Id, Title: item.Title, Tags: tags, Ts: ts}, nil
}

func renderTag(data TagResult, _ *cli.Ctx) string {
	if len(data.Tags) == 0 {
		return fmt.Sprintf("untagged %s  %s", data.Id, data.Title)
	}
	return fmt.Sprintf("tagged %s  %s  %s", data.Id, data.Title, strings.Join(data.Tags, ", "))
}
