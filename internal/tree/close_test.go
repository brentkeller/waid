package tree

import (
	"errors"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// closeLine closes an item, which is how a fixture gets a done descendant.
func closeLine(id string) string {
	return `{"ts":"2026-08-14T10:00:00.000Z","ev":"close","id":"` + id + `"}`
}

func TestGuardCloseAllowsALeaf(t *testing.T) {
	state := events.Fold([]string{
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
	})

	if err := GuardClose(state, "kid1"); err != nil {
		t.Errorf("closing a leaf = %v, want it allowed", err)
	}
}

func TestGuardCloseAllowsAParentWhoseDescendantsAreAllDone(t *testing.T) {
	state := events.Fold([]string{
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
		addLine("gkid", "Grandchild", "kid1"),
		closeLine("kid1"),
		closeLine("gkid"),
	})

	if err := GuardClose(state, "root"); err != nil {
		t.Errorf("closing a finished parent = %v, want it allowed", err)
	}
}

// The guard reaches the whole subtree, not just the direct children: a grandchild left open is
// still a loose end under the item being closed.
func TestGuardCloseRefusesWhileAGrandchildIsOpen(t *testing.T) {
	state := events.Fold([]string{
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
		addLine("gkid", "Grandchild", "kid1"),
		closeLine("kid1"),
	})

	err := GuardClose(state, "root")
	if err == nil {
		t.Fatal("closing a parent with an open grandchild was allowed")
	}
	var user *errs.UserError
	if !errors.As(err, &user) {
		t.Fatalf("err is %T, want *errs.UserError", err)
	}
	if !strings.Contains(user.Message, "root") {
		t.Errorf("message = %q, want it to name the item being closed", user.Message)
	}
	if got, want := user.Candidates, []string{"gkid  Grandchild"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("candidates = %q, want %q", got, want)
	}
}

// The listing is id and title, since two items may share a title and the id is what closes one.
// Closing reads status and nothing else: a heading holding open work is refused like any other
// parent.
func TestGuardCloseRefusesAHeadingHoldingOpenWork(t *testing.T) {
	state := events.Fold([]string{
		headingLine("head", "Project", ""),
		addLine("kid1", "Open child", "head"),
	})

	if err := GuardClose(state, "head"); err == nil {
		t.Errorf("closing a heading over open work = nil, want it refused")
	}
}

// Being a shelf is not a reason to keep an item: an empty heading closes like any other leaf.
func TestGuardCloseAllowsAnEmptyHeading(t *testing.T) {
	state := events.Fold([]string{headingLine("head", "Project", "")})

	if err := GuardClose(state, "head"); err != nil {
		t.Errorf("closing an empty heading = %v, want it allowed", err)
	}
}

func TestGuardCloseListsEveryOpenDescendantByIdAndTitle(t *testing.T) {
	state := events.Fold([]string{
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
		addLine("kid2", "Second child", "root"),
		addLine("gkid", "Grandchild", "kid1"),
	})

	var user *errs.UserError
	if err := GuardClose(state, "root"); !errors.As(err, &user) {
		t.Fatalf("err is %v, want *errs.UserError", err)
	}
	want := []string{"kid1  First child", "gkid  Grandchild", "kid2  Second child"}
	if got := user.Candidates; !equalStrings(got, want) {
		t.Errorf("candidates = %q, want %q", got, want)
	}
	if !strings.Contains(user.Message, "3 items") {
		t.Errorf("message = %q, want it to count the open descendants", user.Message)
	}
}

// A waiting descendant is not a finished one.
func TestGuardCloseCountsAWaitingDescendantAsOpen(t *testing.T) {
	state := events.Fold([]string{
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
		`{"ts":"2026-08-14T10:00:00.000Z","ev":"update","id":"kid1","status":"waiting","waitingOn":"maria"}`,
	})

	if err := GuardClose(state, "root"); err == nil {
		t.Error("closing a parent with a waiting child was allowed")
	}
}

// A log holding a loop still answers: the walk visits each id once rather than spinning.
func TestGuardCloseTerminatesOnACycle(t *testing.T) {
	state := events.Fold([]string{
		addLine("aaaa", "First", "bbbb"),
		addLine("bbbb", "Second", "aaaa"),
	})

	if err := GuardClose(state, "aaaa"); err == nil {
		t.Error("closing an item caught in a cycle was allowed")
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// A run closing a parent alongside the descendants holding it open is what several ids are for, so
// the batch's own members do not hold each other open.
func TestGuardClosesAllowsAParentClosedWithItsOpenDescendants(t *testing.T) {
	state := events.Fold([]string{
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
		addLine("gkid", "Grandchild", "kid1"),
	})

	if err := GuardCloses(state, []string{"root", "kid1", "gkid"}); err != nil {
		t.Errorf("closing the whole subtree = %v, want it allowed", err)
	}
}

// A descendant left out of the batch still holds its ancestor open.
func TestGuardClosesRefusesWhenADescendantIsLeftOut(t *testing.T) {
	state := events.Fold([]string{
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
		addLine("gkid", "Grandchild", "kid1"),
	})

	var user *errs.UserError
	if err := GuardCloses(state, []string{"root", "kid1"}); !errors.As(err, &user) {
		t.Fatalf("closing all but the grandchild = %v, want a user error", err)
	}
	if !strings.Contains(strings.Join(user.Candidates, "\n"), "gkid") {
		t.Errorf("candidates = %q, want the grandchild named", user.Candidates)
	}
}

// The batch is judged as a whole, so an id later in the list is guarded too.
func TestGuardClosesGuardsEveryId(t *testing.T) {
	state := events.Fold([]string{
		addLine("leaf", "Leaf", ""),
		addLine("root", "Root", ""),
		addLine("kid1", "First child", "root"),
	})

	if err := GuardCloses(state, []string{"leaf", "root"}); err == nil {
		t.Error("closing a leaf then a held-open parent = nil, want a refusal")
	}
}
