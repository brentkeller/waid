package tree

import (
	"errors"
	"reflect"
	"testing"

	"github.com/brentkeller/waid/internal/errs"
	"github.com/brentkeller/waid/internal/events"
)

// logOf folds a log of adds so a resolver test states the titles it resolves against.
func logOf(pairs ...string) events.State {
	lines := []string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		lines = append(lines, addLine(pairs[i], pairs[i+1], ""))
	}
	return events.Fold(lines)
}

var titles = logOf(
	"aaaa", "DevResults",
	"bbbb", "Bulk imports to a background worker",
	"cccc", "book flights",
)

func TestResolveMapsEmptyInputToNeither(t *testing.T) {
	for _, input := range []string{"", "   "} {
		got, err := Resolve(input, titles, `C:\cwd`)
		if err != nil {
			t.Fatalf("Resolve(%q) returned %v", input, err)
		}
		if got.Parent != nil || got.Origin != nil {
			t.Fatalf("Resolve(%q) = %+v, want neither", input, got)
		}
	}
}

func TestResolveTakesAnAbsolutePathAsAnOrigin(t *testing.T) {
	cases := map[string]string{
		`C:\dev\waid\`: `C:\dev\waid`,
		"/var/tmp/x/":  "/var/tmp/x",
		"D:/data/repo": "D:/data/repo",
	}

	for input, want := range cases {
		got, err := Resolve(input, titles, `C:\cwd`)
		if err != nil {
			t.Fatalf("Resolve(%q) returned %v", input, err)
		}
		if got.Origin == nil || *got.Origin != want {
			t.Fatalf("Resolve(%q) origin = %v, want %q", input, got.Origin, want)
		}
		if got.Parent != nil {
			t.Fatalf("Resolve(%q) parent = %q, want none", input, *got.Parent)
		}
	}
}

func TestResolveMapsDotToTheCwdAsAnOrigin(t *testing.T) {
	cases := []struct{ input, cwd, want string }{
		{".", `C:\dev\elsewhere\`, `C:\dev\elsewhere`},
		{" . ", "/home/brent/", "/home/brent"},
	}

	for _, c := range cases {
		got, err := Resolve(c.input, titles, c.cwd)
		if err != nil {
			t.Fatalf("Resolve(%q, %q) returned %v", c.input, c.cwd, err)
		}
		if got.Origin == nil || *got.Origin != c.want {
			t.Fatalf("Resolve(%q, %q) origin = %v, want %q", c.input, c.cwd, got.Origin, c.want)
		}
		if got.Parent != nil {
			t.Fatalf("Resolve(%q, %q) parent = %q, want none", c.input, c.cwd, *got.Parent)
		}
	}
}

func TestResolveMatchesATitleFragmentCaseInsensitively(t *testing.T) {
	cases := map[string]string{
		"DevResults": "aaaa",
		"devresults": "aaaa",
		"BULK":       "bbbb",
		"flights":    "cccc",
	}

	for input, want := range cases {
		got, err := Resolve(input, titles, `C:\cwd`)
		if err != nil {
			t.Fatalf("Resolve(%q) returned %v", input, err)
		}
		if got.Parent == nil || *got.Parent != want {
			t.Fatalf("Resolve(%q) parent = %v, want %q", input, got.Parent, want)
		}
		if got.Origin != nil {
			t.Fatalf("Resolve(%q) origin = %q, want none", input, *got.Origin)
		}
	}
}

func TestResolveRejectsAFragmentMatchingNothing(t *testing.T) {
	_, err := Resolve("nope", titles, `C:\cwd`)

	var userErr *errs.UserError
	if !errors.As(err, &userErr) {
		t.Fatalf("Resolve returned %v, want a UserError", err)
	}
	if want := "no item matches: nope"; userErr.Message != want {
		t.Fatalf("message = %q, want %q", userErr.Message, want)
	}
}

func TestResolveRejectsAFragmentMatchingSeveralAndListsThem(t *testing.T) {
	_, err := Resolve("b", titles, `C:\cwd`)

	var userErr *errs.UserError
	if !errors.As(err, &userErr) {
		t.Fatalf("Resolve returned %v, want a UserError", err)
	}
	if want := "ambiguous parent: b"; userErr.Message != want {
		t.Fatalf("message = %q, want %q", userErr.Message, want)
	}
	want := []string{"bbbb  Bulk imports to a background worker", "cccc  book flights"}
	if !reflect.DeepEqual(userErr.Candidates, want) {
		t.Fatalf("candidates = %q, want %q", userErr.Candidates, want)
	}
}

func TestResolveRejectsAnyFragmentWhenTheLogIsEmpty(t *testing.T) {
	_, err := Resolve("waid", events.Fold(nil), `C:\cwd`)

	var userErr *errs.UserError
	if !errors.As(err, &userErr) {
		t.Fatalf("Resolve returned %v, want a UserError", err)
	}
}
