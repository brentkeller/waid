package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brentkeller/waid/internal/errs"
)

type capture struct {
	out bytes.Buffer
	err bytes.Buffer
}

func (c *capture) io() Io {
	return Io{Out: &c.out, Err: &c.err}
}

// probeResult is what the test commands produce, and what --json is expected to print verbatim.
type probeResult struct {
	Count int    `json:"count"`
	Title string `json:"title"`
}

// testHome returns a data directory the run may create and write, with every environment seam a
// developer's own shell might carry cleared.
func testHome(t *testing.T) string {
	t.Helper()

	t.Setenv("WAID_SKIP_GH_DETECT", "1")
	t.Setenv("WAID_HOME", "")
	t.Setenv(EnvNow, "")
	t.Setenv(EnvIds, "")
	return t.TempDir()
}

// argv prefixes a command line with the home override every dispatch test runs against.
func argv(home string, args ...string) []string {
	return append([]string{"--waid-home", home}, args...)
}

func TestRunPrintsUsageWithoutACommand(t *testing.T) {
	c := &capture{}
	home := testHome(t)

	code := Run(argv(home), c.io(), Registry{})

	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if got := c.out.String(); got != Usage {
		t.Fatalf("stdout = %q, want the usage text", got)
	}
	if c.err.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", c.err.String())
	}
}

func TestRunPrintsUsageForHelp(t *testing.T) {
	c := &capture{}
	home := testHome(t)

	code := Run(argv(home, "list", "--help"), c.io(), Registry{})

	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if got := c.out.String(); got != Usage {
		t.Fatalf("stdout = %q, want the usage text", got)
	}
}

// The usage text is the command surface, so it must not offer the picker the port dropped.
func TestUsageDoesNotOfferInteractive(t *testing.T) {
	if strings.Contains(Usage, "-i") || strings.Contains(Usage, "interactive") {
		t.Fatalf("usage still mentions the interactive picker:\n%s", Usage)
	}
}

func TestRunPrintsVersionWithoutTouchingTheHome(t *testing.T) {
	c := &capture{}
	home := filepath.Join(testHome(t), "unborn")

	code := Run(argv(home, "--version"), c.io(), Registry{})

	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if got, want := c.out.String(), "waid "+Version+"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("stat %s = %v, want the home left uncreated", home, err)
	}
}

func TestRunSeedsTheHome(t *testing.T) {
	c := &capture{}
	home := filepath.Join(testHome(t), "fresh")

	if code := Run(argv(home), c.io(), Registry{}); code != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", code, c.err.String())
	}

	for _, name := range []string{"config.json", "events.jsonl"} {
		if _, err := os.Stat(filepath.Join(home, name)); err != nil {
			t.Errorf("stat %s: %v", name, err)
		}
	}
}

func TestRunReportsAnUnknownCommand(t *testing.T) {
	c := &capture{}
	home := testHome(t)

	code := Run(argv(home, "nope"), c.io(), Registry{})

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

// A command reporting its own missing argument exits 1 and prints nothing to stdout, so a caller
// cannot mistake the failure for an empty result.
func TestRunReportsAMissingArgument(t *testing.T) {
	c := &capture{}
	home := testHome(t)
	registry := Registry{"done": Module[probeResult]{
		Run: func(ctx *Ctx) (probeResult, error) {
			if len(ctx.Args) == 0 {
				return probeResult{}, errs.Userf("done requires an item id")
			}
			return probeResult{Count: 1}, nil
		},
		Render: func(data probeResult, ctx *Ctx) string { return "done" },
	}}

	code := Run(argv(home, "done"), c.io(), registry)

	if code != ExitUser {
		t.Fatalf("exit code = %d, want %d", code, ExitUser)
	}
	if got, want := c.err.String(), "done requires an item id\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if c.out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", c.out.String())
	}
}

// A flag left without its value fails before the command runs, and before --json is known, so the
// report is the human one either way.
func TestRunReportsAFlagMissingItsValue(t *testing.T) {
	c := &capture{}
	home := testHome(t)

	code := Run(argv(home, "list", "--origin"), c.io(), Registry{})

	if code != ExitUser {
		t.Fatalf("exit code = %d, want %d", code, ExitUser)
	}
	if got, want := c.err.String(), "flag --origin requires a value\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

// --json prints the command's result and nothing else: the same value render would have been given,
// indented two spaces, with `&` left raw.
func TestJsonPrintsTheResultVerbatim(t *testing.T) {
	c := &capture{}
	home := testHome(t)
	registry := Registry{"probe": Module[probeResult]{
		Run: func(ctx *Ctx) (probeResult, error) {
			return probeResult{Count: 2, Title: "loops & signals"}, nil
		},
		Render: func(data probeResult, ctx *Ctx) string { return "rendered" },
	}}

	code := Run(argv(home, "probe", "--json"), c.io(), registry)

	if code != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", code, c.err.String())
	}
	want := "{\n  \"count\": 2,\n  \"title\": \"loops & signals\"\n}\n"
	if got := c.out.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// Notes are degradations, not results, so --json keeps stdout a single document and puts them on
// stderr; the human path prints them beneath the rendered text instead.
func TestNotesAreReportedBesideTheResult(t *testing.T) {
	registry := Registry{"probe": Module[probeResult]{
		Run: func(ctx *Ctx) (probeResult, error) {
			ctx.Note("gh unavailable; showing git signals only")
			return probeResult{Count: 1}, nil
		},
		Render: func(data probeResult, ctx *Ctx) string { return "one signal\n\n" },
	}}

	asJson := &capture{}
	home := testHome(t)
	if code := Run(argv(home, "probe", "--json"), asJson.io(), registry); code != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", code, asJson.err.String())
	}
	if got, want := asJson.err.String(), "gh unavailable; showing git signals only\n"; got != want {
		t.Errorf("json stderr = %q, want %q", got, want)
	}
	if strings.Contains(asJson.out.String(), "gh unavailable") {
		t.Errorf("json stdout = %q, want the note kept off it", asJson.out.String())
	}

	human := &capture{}
	if code := Run(argv(home, "probe"), human.io(), registry); code != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", code, human.err.String())
	}
	want := "one signal\ngh unavailable; showing git signals only\n"
	if got := human.out.String(); got != want {
		t.Errorf("human stdout = %q, want %q", got, want)
	}
	if human.err.Len() != 0 {
		t.Errorf("human stderr = %q, want empty", human.err.String())
	}
}

// Rendering nothing prints nothing, rather than a bare newline.
func TestEmptyRenderPrintsNothing(t *testing.T) {
	c := &capture{}
	home := testHome(t)
	registry := Registry{"probe": Module[probeResult]{
		Run:    func(ctx *Ctx) (probeResult, error) { return probeResult{}, nil },
		Render: func(data probeResult, ctx *Ctx) string { return "" },
	}}

	if code := Run(argv(home, "probe"), c.io(), registry); code != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", code, c.err.String())
	}
	if c.out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", c.out.String())
	}
}

func TestCtxCarriesTheParsedCommandLine(t *testing.T) {
	c := &capture{}
	home := testHome(t)
	t.Setenv(EnvNow, "2026-08-17T12:00:00Z")

	var seen *Ctx
	registry := Registry{"probe": Module[probeResult]{
		Run: func(ctx *Ctx) (probeResult, error) {
			seen = ctx
			return probeResult{}, nil
		},
		Render: func(data probeResult, ctx *Ctx) string { return "" },
	}}

	if code := Run(argv(home, "probe", "abcd", "-p", "waid", "--tag", "bug"), c.io(), registry); code != ExitOK {
		t.Fatalf("exit code = %d, stderr = %q", code, c.err.String())
	}
	if seen == nil {
		t.Fatal("the command never ran")
	}
	if got := seen.Args; len(got) != 1 || got[0] != "abcd" {
		t.Errorf("args = %v, want [abcd]", got)
	}
	if got, _ := seen.Flags.String("project"); got != "waid" {
		t.Errorf("project = %q, want waid", got)
	}
	if got := seen.Flags.List("tag"); len(got) != 1 || got[0] != "bug" {
		t.Errorf("tags = %v, want [bug]", got)
	}
	if seen.Cfg.Home != home {
		t.Errorf("home = %q, want %q", seen.Cfg.Home, home)
	}
	if want := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC); !seen.Now.Equal(want) {
		t.Errorf("now = %s, want %s", seen.Now, want)
	}
	if cwd, _ := os.Getwd(); seen.Cwd != cwd {
		t.Errorf("cwd = %q, want %q", seen.Cwd, cwd)
	}
}

func TestUserErrorPrintsCandidates(t *testing.T) {
	c := &capture{}

	code := report(&errs.UserError{Message: "ambiguous project: dev", Candidates: []string{"C:\\dev\\waid", "C:\\dev\\web"}}, false, c.io())

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

	report(&errs.UserError{Message: "unknown command: nope"}, true, c.io())

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
	home := testHome(t)
	registry := Registry{"probe": Module[probeResult]{
		Run:    func(ctx *Ctx) (probeResult, error) { panic("exploded") },
		Render: func(data probeResult, ctx *Ctx) string { return "" },
	}}

	code := Run(argv(home, "probe"), c.io(), registry)

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
	home := testHome(t)

	code := Run(argv(home, "nope", "--json"), c.io(), Registry{})

	if code != ExitUser {
		t.Fatalf("exit code = %d, want %d", code, ExitUser)
	}
	want := `{"error":"unknown command: nope","candidates":null}` + "\n"
	if got := c.err.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

// A command failing after it started still reports in the caller's chosen encoding.
func TestRunReportsACommandErrorAsJson(t *testing.T) {
	c := &capture{}
	home := testHome(t)
	registry := Registry{"probe": Module[probeResult]{
		Run:    func(ctx *Ctx) (probeResult, error) { return probeResult{}, errs.Userf("no such item: zzzz") },
		Render: func(data probeResult, ctx *Ctx) string { return "" },
	}}

	code := Run(argv(home, "probe", "--json"), c.io(), registry)

	if code != ExitUser {
		t.Fatalf("exit code = %d, want %d", code, ExitUser)
	}
	want := `{"error":"no such item: zzzz","candidates":null}` + "\n"
	if got := c.err.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}
