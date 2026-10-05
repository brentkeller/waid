package tree

import (
	"fmt"

	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// GuardClose reports whether an item may be closed, refusing while anything filed beneath it is
// still open. Both `waid done` and the app's `d` call it: two surfaces that disagreed about when a
// parent may close would be worse than either rule on its own.
//
// Refusal was chosen over a cascading close, so the point of a heading holds — a closed parent
// provably has no loose ends left under it. The error names every open descendant by id and title,
// since closing them is what the refusal asks for and the ids are what closes them.
func GuardClose(state events.State, id string) error {
	return GuardCloses(state, []string{id})
}

// GuardCloses is GuardClose over a set of ids closed together. The ids in the set do not hold each
// other open: closing the children first is what a list of ids is for, so a run naming a parent
// alongside the descendants beneath it is allowed, while one leaving a descendant out is not.
//
// The whole set is judged before anything is written, so a refusal over one id cannot leave the
// others closed behind it.
func GuardCloses(state events.State, ids []string) error {
	closing := make(map[string]bool, len(ids))
	for _, id := range ids {
		closing[id] = true
	}

	for _, id := range ids {
		open := []events.Item{}
		for _, descendant := range OpenDescendants(state, id) {
			if !closing[descendant.Id] {
				open = append(open, descendant)
			}
		}
		if len(open) == 0 {
			continue
		}
		return &errs.UserError{
			Message:    fmt.Sprintf("cannot close %s: %s still open beneath it", id, count(len(open))),
			Candidates: labels(open),
		}
	}
	return nil
}

// OpenDescendants returns every item under id that is not closed, depth first in log order. An id
// nothing holds has none.
//
// Each id is visited once, so a log whose parents loop answers rather than spinning — internal/tree
// reports the loop itself, and the guard only has to terminate in its presence.
func OpenDescendants(state events.State, id string) []events.Item {
	open := []events.Item{}
	seen := map[string]bool{id: true}

	var walk func(parent string)
	walk = func(parent string) {
		for _, child := range state.Children(parent) {
			if seen[child.Id] {
				continue
			}
			seen[child.Id] = true
			if child.Status != events.StatusDone {
				open = append(open, child)
			}
			walk(child.Id)
		}
	}
	walk(id)
	return open
}

// count phrases a number of items with the verb that agrees with it.
func count(n int) string {
	if n == 1 {
		return "1 item is"
	}
	return fmt.Sprintf("%d items are", n)
}
