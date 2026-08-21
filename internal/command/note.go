package command

import (
	"fmt"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// NoteResult is the appended note, echoed back with the item it landed on.
type NoteResult struct {
	Id    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	Ts    string `json:"ts"`
}

func runNote(ctx *cli.Ctx) (NoteResult, error) {
	id, err := requiredId(ctx, "note")
	if err != nil {
		return NoteResult{}, err
	}

	text := strings.TrimSpace(strings.Join(ctx.Args[1:], " "))
	if text == "" {
		return NoteResult{}, errs.Userf("note requires text")
	}

	item, err := requireItem(events.Load(ctx.Cfg.EventsPath), id)
	if err != nil {
		return NoteResult{}, err
	}

	ts, err := events.Append(ctx.Cfg.EventsPath, events.NoteEvent{Ev: "note", Id: item.Id, Text: text}, ctx.Now)
	if err != nil {
		return NoteResult{}, err
	}
	return NoteResult{Id: item.Id, Title: item.Title, Text: text, Ts: ts}, nil
}

func renderNote(data NoteResult, ctx *cli.Ctx) string {
	return fmt.Sprintf("noted %s  %s", data.Id, data.Text)
}
