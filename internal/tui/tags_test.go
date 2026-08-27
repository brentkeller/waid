package tui

import (
	"strings"
	"testing"
)

// t opens the input on the tags the item already carries, so adding one is a word of typing rather
// than a retype of the set — the way e opens on the title it corrects.
func TestTagOpensOnTheTagsItCarries(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "j", "j", "t")

	if m.prompt.kind != promptTags {
		t.Fatalf("t opened prompt %d, want the tags prompt", m.prompt.kind)
	}
	if got, want := m.prompt.text(), "promoted"; got != want {
		t.Errorf("the prompt opened on %q, want the tags it edits %q", got, want)
	}
	if view := plain(m.View()); !strings.Contains(view, "tags nktt promoted") {
		t.Errorf("the prefilled tags are not in the footer:\n%s", view)
	}
}

// Committing replaces the set, and the row carries it without waiting for a re-read.
func TestTagRewritesTheSet(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "t", "ctrl+u", "bug, ui", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"nktt","tags":["bug","ui"]}`})

	view := plain(m.View())
	if row := rowFor(t, view, "nktt"); !strings.Contains(row, "[bug,ui]") {
		t.Errorf("the row is %q, want the tags it was given", row)
	}
	if !strings.Contains(view, "tagged nktt  bug, ui") {
		t.Errorf("the footer carries no receipt for the write:\n%s", view)
	}
}

// An empty answer clears the set rather than abandoning the edit: esc is what abandons, and clearing
// tags is a thing to want. It is the one prompt where a committed blank says something.
func TestTagCommittedEmptyClearsTheSet(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "t", "ctrl+u", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"nktt","tags":[]}`})

	view := plain(m.View())
	if row := rowFor(t, view, "nktt"); strings.Contains(row, "promoted") {
		t.Errorf("the row is %q, want the tag gone", row)
	}
	if !strings.Contains(view, "untagged nktt  Background workers") {
		t.Errorf("the footer carries no receipt for the clear:\n%s", view)
	}
}

// esc abandons the edit, which is the only way out that writes nothing.
func TestTagAbandonedWithEscWritesNothing(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "t", "ctrl+u", "bug", "esc")

	assertLog(t, path, nil)
	if _, written := m.lastReceipt(); written {
		t.Error("an abandoned tag edit left a receipt")
	}
}

// The typed answer is split on commas and tidied before it is written, so the log never holds a
// blank tag, a repeat, or one padded with the spaces that make a set readable to type.
func TestTagNormalisesWhatIsTyped(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "t", "ctrl+u", "  bug , ui ,, bug ", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{`{"ts":"` + ts + `","ev":"update","id":"nktt","tags":["bug","ui"]}`})
}

// The inverse puts back the set the write replaced.
func TestUndoOfATagRestoresTheSetItReplaced(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "t", "ctrl+u", "bug", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"nktt","tags":["bug"]}`,
		`{"ts":"` + ts + `","ev":"update","id":"nktt","tags":["promoted"]}`,
	})
	if row := rowFor(t, plain(m.View()), "nktt"); !strings.Contains(row, "[promoted]") {
		t.Errorf("the row is %q, want the tag the write replaced", row)
	}
}

// The inverse of tagging an item that carried none is an empty set, which the event can only say
// because it always writes the field.
func TestUndoOfATagRestoresAnItemThatCarriedNone(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "t", "bug", "enter", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"sga9","tags":["bug"]}`,
		`{"ts":"` + ts + `","ev":"update","id":"sga9","tags":[]}`,
	})
	if row := rowFor(t, plain(m.View()), "sga9"); strings.Contains(row, "bug") {
		t.Errorf("the row is %q, want the tag taken back off", row)
	}
}

// A row that is not an item has no tags to edit, and the footer says which row the key was pressed
// on rather than staying silent.
func TestTagOnANonItemRowIsInert(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "j", "j", "j", "t")

	assertLog(t, path, nil)
	if m.prompt.kind != promptNone {
		t.Error("t opened a prompt on a row that is not an item")
	}
	if !strings.Contains(plain(m.View()), "t tags an item") {
		t.Errorf("the footer says nothing about the inert key:\n%s", plain(m.View()))
	}
}
