// Package triage holds the writes a detected signal is retired by. The `promote` and `dismiss`
// commands and the app's Scan tab all go through it, so a signal triaged from either surface lands
// in the log as the same events.
package triage

import (
	"time"

	"github.com/brentkeller/waid/internal/detect"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/ids"
)

// PromotedTag is the tag every promoted item carries, so a detected loop stays distinguishable from
// a declared one.
const PromotedTag = "promoted"

// Promotion is the item a signal became. Its fields are the ones `promote --json` echoes back.
type Promotion struct {
	Id     string
	Key    string
	Title  string
	Origin *string
}

// Promote writes the two events a promotion is made of: an add carrying the signal's repo as the
// item's origin and the promoted tag, then a dismiss of the signal's key so the loop is not reported
// twice. The item lands at top level: the repo it came from is provenance, not a place in the tree,
// so filing it is a separate sweep and promotion stays one keystroke. title is passed separately so a
// caller that let the user edit it can use the edited text; state only supplies the ids already
// taken.
func Promote(path string, state events.State, generate ids.Generator, signal detect.Signal, title string, now time.Time) (Promotion, error) {
	id, err := state.NewId(generate)
	if err != nil {
		return Promotion{}, err
	}

	if _, err := events.Append(path, events.AddEvent{
		Ev:        "add",
		Id:        id,
		Title:     title,
		Status:    events.StatusOpen,
		Parent:    nil,
		Origin:    signal.Project,
		Session:   nil,
		Tags:      []string{PromotedTag},
		WaitingOn: nil,
	}, now); err != nil {
		return Promotion{}, err
	}
	if err := Dismiss(path, signal.Key, now); err != nil {
		return Promotion{}, err
	}

	return Promotion{Id: id, Key: signal.Key, Title: title, Origin: signal.Project}, nil
}

// Dismiss hides a detected signal by its key. The key is not validated against detection: a signal
// only has to have existed once to be worth silencing.
func Dismiss(path, key string, now time.Time) error {
	_, err := events.Append(path, events.DismissEvent{Ev: "dismiss", Key: key}, now)
	return err
}
