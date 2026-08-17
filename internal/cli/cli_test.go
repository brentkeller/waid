package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type capture struct {
	out bytes.Buffer
	err bytes.Buffer
}

func (c *capture) io() Io {
	return Io{Out: &c.out, Err: &c.err}
}

func TestRunSucceeds(t *testing.T) {
	c := &capture{}

	code := Run(nil, c.io())

	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if c.err.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", c.err.String())
	}
}

func TestRunReportsUserError(t *testing.T) {
	c := &capture{}

	code := Run([]string{"nope"}, c.io())

	if code != ExitUser {
		t.Fatalf("exit code = %d, want %d", code, ExitUser)
	}
	if got, want := c.err.String(), "unknown command: nope\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if c.out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", c.out.String())
	}
}

func TestUserErrorPrintsCandidates(t *testing.T) {
	c := &capture{}

	code := report(&UserError{Message: "ambiguous project: dev", Candidates: []string{"C:\\dev\\waid", "C:\\dev\\web"}}, false, c.io())

	if code != ExitUser {
		t.Fatalf("exit code = %d, want %d", code, ExitUser)
	}
	want := "ambiguous project: dev\n  C:\\dev\\waid\n  C:\\dev\\web\n"
	if got := c.err.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestUserErrorJson(t *testing.T) {
	c := &capture{}

	report(&UserError{Message: "unknown command: nope"}, true, c.io())

	want := `{"error":"unknown command: nope","candidates":null}` + "\n"
	if got := c.err.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunReportsInternalErrorWithStack(t *testing.T) {
	c := &capture{}

	code := report(errors.New("boom"), false, c.io())

	if code != ExitInternal {
		t.Fatalf("exit code = %d, want %d", code, ExitInternal)
	}
	stderr := c.err.String()
	if !strings.HasPrefix(stderr, "boom\n") {
		t.Fatalf("stderr = %q, want it to start with the message", stderr)
	}
	if !strings.Contains(stderr, "cli.TestRunReportsInternalErrorWithStack") {
		t.Fatalf("stderr = %q, want it to carry a stack", stderr)
	}
}

func TestInternalErrorJsonCarriesStack(t *testing.T) {
	c := &capture{}

	report(errors.New("boom"), true, c.io())

	stderr := c.err.String()
	if !strings.HasPrefix(stderr, `{"error":"boom","stack":"boom\n`) {
		t.Fatalf("stderr = %q, want a single JSON line carrying the stack", stderr)
	}
	if strings.Count(stderr, "\n") != 1 {
		t.Fatalf("stderr = %q, want a single line", stderr)
	}
}

func TestRunRecoversPanic(t *testing.T) {
	c := &capture{}

	code := Run([]string{"panic-probe"}, c.io(), func(argv []string, io Io, json *bool) error {
		panic("exploded")
	})

	if code != ExitInternal {
		t.Fatalf("exit code = %d, want %d", code, ExitInternal)
	}
	stderr := c.err.String()
	if !strings.HasPrefix(stderr, "exploded\n") {
		t.Fatalf("stderr = %q, want the panic value first", stderr)
	}
	if !strings.Contains(stderr, "cli.TestRunRecoversPanic") {
		t.Fatalf("stderr = %q, want the panicking stack", stderr)
	}
}

func TestRunReportsErrorsAsJsonWhenFlagParsed(t *testing.T) {
	c := &capture{}

	code := Run([]string{"nope", "--json"}, c.io())

	if code != ExitUser {
		t.Fatalf("exit code = %d, want %d", code, ExitUser)
	}
	want := `{"error":"unknown command: nope","candidates":null}` + "\n"
	if got := c.err.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}
