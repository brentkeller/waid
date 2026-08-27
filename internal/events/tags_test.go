package events

import (
	"slices"
	"testing"
)

// NormalizeTags guards what reaches the log, so every surface that writes a tag set agrees on what
// one is: trimmed, never blank, never repeated, and never null.
func TestNormalizeTags(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "keeps the order it was given", in: []string{"ui", "bug"}, want: []string{"ui", "bug"}},
		{name: "trims the whitespace around a tag", in: []string{"  bug ", "\tui"}, want: []string{"bug", "ui"}},
		{name: "drops a blank", in: []string{"bug", "   ", ""}, want: []string{"bug"}},
		{name: "keeps the first of a repeat", in: []string{"bug", "ui", "bug"}, want: []string{"bug", "ui"}},
		{name: "repeats are compared after trimming", in: []string{"bug", " bug"}, want: []string{"bug"}},
		{name: "case is a difference, as it is on the command line", in: []string{"Bug", "bug"}, want: []string{"Bug", "bug"}},
		{name: "an empty set stays empty", in: []string{}, want: []string{}},
		{name: "nothing at all is an empty set, never null", in: nil, want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeTags(test.in)
			if got == nil {
				t.Fatal("NormalizeTags returned nil, want an empty set")
			}
			if !slices.Equal(got, test.want) {
				t.Errorf("NormalizeTags(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

// The result never shares a backing array with the input, so a caller holding the item's own tags
// cannot have them rewritten underneath it.
func TestNormalizeTagsDoesNotAliasItsInput(t *testing.T) {
	in := []string{"bug", "ui"}

	got := NormalizeTags(in)
	got[0] = "changed"

	if in[0] != "bug" {
		t.Errorf("input[0] = %q, want it left alone", in[0])
	}
}
