package events

import (
	"encoding/json"
	"strings"
)

// Fold folds raw log lines into items and dismissed keys.
//
// Pure: it touches no filesystem and never panics. The input is untrusted JSON read from disk, so
// every field is narrowed before use and anything unusable becomes a Problem carrying the 1-based
// line number. File order is authoritative — ts is display metadata only, since parallel agents can
// append out-of-order timestamps.
func Fold(rawLines []string) State {
	items := map[string]*Item{}
	order := []string{}
	dismissed := []string{}
	problems := []Problem{}

	for index, rawLine := range rawLines {
		line := index + 1
		report := func(reason ProblemReason, id *string, ev *string) {
			problems = append(problems, Problem{Line: line, Reason: reason, Id: id, Ev: ev})
		}

		if strings.TrimSpace(rawLine) == "" {
			continue
		}

		var parsed any
		if err := json.Unmarshal([]byte(rawLine), &parsed); err != nil {
			report(ReasonUnparseable, nil, nil)
			continue
		}

		record, ok := parsed.(map[string]any)
		if !ok {
			report(ReasonNotAnObject, nil, nil)
			continue
		}

		ev := str(record["ev"])
		ts := ""
		if value := str(record["ts"]); value != nil {
			ts = *value
		}
		id := str(record["id"])

		switch derefOr(ev, "") {
		case "add":
			title := str(record["title"])
			if id == nil || title == nil {
				report(ReasonAddMissingField, id, ev)
				continue
			}
			if _, exists := items[*id]; exists {
				report(ReasonDuplicateId, id, ev)
				continue
			}
			status := readStatus(record, func() { report(ReasonBadStatus, id, ev) })
			if status == nil {
				status = ptrTo(StatusOpen)
			}
			items[*id] = &Item{
				Id:        *id,
				Title:     *title,
				Status:    *status,
				WaitingOn: str(record["waitingOn"]),
				Project:   str(record["project"]),
				Session:   str(record["session"]),
				Tags:      strArray(record["tags"]),
				Notes:     []Note{},
				Created:   ts,
				Updated:   ts,
			}
			order = append(order, *id)

		case "update", "note", "close", "reopen":
			var item *Item
			if id != nil {
				item = items[*id]
			}
			if item == nil {
				report(ReasonUnknownId, id, ev)
				continue
			}
			if !applyToItem(item, derefOr(ev, ""), record, ts, report) {
				continue
			}
			item.Updated = ts

		case "dismiss", "undismiss":
			key := str(record["key"])
			if key == nil {
				report(ReasonMissingKey, id, ev)
				continue
			}
			at := indexOf(dismissed, *key)
			if derefOr(ev, "") == "dismiss" {
				if at == -1 {
					dismissed = append(dismissed, *key)
				}
			} else if at != -1 {
				dismissed = append(dismissed[:at], dismissed[at+1:]...)
			}

		default:
			report(ReasonUnknownEv, id, ev)
		}
	}

	folded := make([]Item, 0, len(order))
	for _, id := range order {
		folded = append(folded, *items[id])
	}
	return State{Items: folded, Dismissed: dismissed, Problems: problems}
}

// applyToItem applies one event to an existing item, reporting whether anything was applied. A bad
// status is reported and ignored; the event's other fields still land.
func applyToItem(
	item *Item,
	ev string,
	record map[string]any,
	ts string,
	report func(reason ProblemReason, id *string, ev *string),
) bool {
	switch ev {
	case "note":
		text := str(record["text"])
		if text == nil {
			report(ReasonNoteMissingText, &item.Id, &ev)
			return false
		}
		item.Notes = append(item.Notes, Note{Ts: ts, Text: *text})
		return true

	case "close":
		item.Status = StatusDone
		return true

	case "reopen":
		item.Status = StatusOpen
		item.WaitingOn = nil
		return true
	}

	status := readStatus(record, func() { report(ReasonBadStatus, &item.Id, &ev) })
	if status != nil {
		item.Status = *status
	}
	if raw, present := record["title"]; present {
		if title := str(raw); title != nil {
			item.Title = *title
		}
	}
	if raw, present := record["project"]; present {
		item.Project = str(raw)
	}
	if raw, present := record["waitingOn"]; present {
		item.WaitingOn = str(raw)
	}
	if raw, present := record["tags"]; present {
		item.Tags = strArray(raw)
	}
	return true
}

// readStatus reads a status field, calling onBad for a present-but-invalid value.
func readStatus(record map[string]any, onBad func()) *Status {
	raw, present := record["status"]
	if !present || raw == nil {
		return nil
	}
	if value, isString := raw.(string); isString && IsStatus(value) {
		return ptrTo(Status(value))
	}
	onBad()
	return nil
}

// str narrows an untrusted value to a string, reporting anything else as absent.
func str(value any) *string {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return &text
}

// strArray narrows an untrusted value to the strings it holds, dropping every other element.
func strArray(value any) []string {
	tags := []string{}
	entries, ok := value.([]any)
	if !ok {
		return tags
	}
	for _, entry := range entries {
		if text, isString := entry.(string); isString {
			tags = append(tags, text)
		}
	}
	return tags
}

func indexOf(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}

func derefOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

func ptrTo[T any](value T) *T { return &value }
