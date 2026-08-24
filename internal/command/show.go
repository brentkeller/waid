package command

import (
	"fmt"
	"strings"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/render"
)

// HistoryEntry is one log line touching the item, located by its 1-based line number.
type HistoryEntry struct {
	Ts   string `json:"ts"`
	Ev   string `json:"ev"`
	Line int    `json:"line"`
}

// ShowResult is one item with every log line naming its id, in file order.
type ShowResult struct {
	Item    events.Item    `json:"item"`
	History []HistoryEntry `json:"history"`
}

const labelWidth = 9

func runShow(ctx *cli.Ctx) (ShowResult, error) {
	item, err := targetItem(ctx, "show")
	if err != nil {
		return ShowResult{}, err
	}

	history := []HistoryEntry{}
	for _, record := range events.ReadRecords(ctx.Cfg.EventsPath) {
		if text(record.Value["id"]) != item.Id {
			continue
		}
		history = append(history, HistoryEntry{
			Ts:   text(record.Value["ts"]),
			Ev:   text(record.Value["ev"]),
			Line: record.Line,
		})
	}

	return ShowResult{Item: item, History: history}, nil
}

func renderShow(data ShowResult, ctx *cli.Ctx) string {
	item := data.Item
	status := string(item.Status)
	if item.WaitingOn != nil {
		status = fmt.Sprintf("%s ← %s", status, *item.WaitingOn)
	}
	if item.Heading {
		status = fmt.Sprintf("%s  heading", status)
	}

	tags := "-"
	if len(item.Tags) > 0 {
		tags = strings.Join(item.Tags, ", ")
	}

	lines := []string{
		fmt.Sprintf("%s  %s", item.Id, item.Title),
		"",
		field("status", status),
		field("origin", orDash(item.Origin)),
		field("tags", tags),
		field("session", orDash(item.Session)),
		field("created", stamp(item.Created, ctx)),
		field("updated", stamp(item.Updated, ctx)),
	}

	if len(item.Notes) > 0 {
		lines = append(lines, "", "  notes")
		for _, note := range item.Notes {
			lines = append(lines, fmt.Sprintf("    %s  %s", render.LocalYmdISO(note.Ts), note.Text))
		}
	}

	lines = append(lines, "", "  history")
	for _, entry := range data.History {
		lines = append(lines, fmt.Sprintf("    %4d  %s  %s", entry.Line, render.LocalYmdISO(entry.Ts), entry.Ev))
	}

	return strings.Join(lines, "\n")
}

func field(label, value string) string {
	return "  " + render.Pad(label, labelWidth) + value
}

func stamp(ts string, ctx *cli.Ctx) string {
	return fmt.Sprintf("%s  (%s)", render.LocalYmdISO(ts), render.RelTime(ts, ctx.Now))
}

func orDash(value *string) string {
	if value == nil {
		return "-"
	}
	return *value
}

// text reads an untrusted field as a string, reading anything else as absent.
func text(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}
