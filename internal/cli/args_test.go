package cli

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseArgvSplitsCommandPositionalsAndValueFlags(t *testing.T) {
	parsed, err := ParseArgv([]string{"add", "fix", "the", "thing", "-p", "C:\\dev\\waid", "--session", "s1"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if parsed.Command != "add" {
		t.Fatalf("command = %q, want %q", parsed.Command, "add")
	}
	if want := []string{"fix", "the", "thing"}; !reflect.DeepEqual(parsed.Args, want) {
		t.Fatalf("args = %q, want %q", parsed.Args, want)
	}
	if got, _ := parsed.Flags.String("project"); got != "C:\\dev\\waid" {
		t.Fatalf("project = %q, want %q", got, "C:\\dev\\waid")
	}
	if got, _ := parsed.Flags.String("session"); got != "s1" {
		t.Fatalf("session = %q, want %q", got, "s1")
	}
}

func TestParseArgvBooleanFlagDoesNotSwallowNextArgument(t *testing.T) {
	parsed, err := ParseArgv([]string{"list", "--all", "leftover"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if parsed.Command != "list" {
		t.Fatalf("command = %q, want %q", parsed.Command, "list")
	}
	if !parsed.Flags.Bool("all") {
		t.Fatal("--all did not parse as true")
	}
	if want := []string{"leftover"}; !reflect.DeepEqual(parsed.Args, want) {
		t.Fatalf("args = %q, want %q", parsed.Args, want)
	}
}

func TestParseArgvEveryBooleanFlagParsesWithoutValue(t *testing.T) {
	parsed, err := ParseArgv([]string{"sync", "--json", "--no-sync", "--full", "--last", "--all", "--version", "-h"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	for _, name := range []string{"json", "no-sync", "full", "last", "all", "version", "help"} {
		if !parsed.Flags.Bool(name) {
			t.Fatalf("--%s did not parse as true", name)
		}
	}
	if len(parsed.Args) != 0 {
		t.Fatalf("args = %q, want none", parsed.Args)
	}
}

func TestParseArgvInlineValueBinds(t *testing.T) {
	parsed, err := ParseArgv([]string{"list", "--project=C:\\dev\\waid", "--status=waiting"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if got, _ := parsed.Flags.String("project"); got != "C:\\dev\\waid" {
		t.Fatalf("project = %q, want %q", got, "C:\\dev\\waid")
	}
	if got, _ := parsed.Flags.String("status"); got != "waiting" {
		t.Fatalf("status = %q, want %q", got, "waiting")
	}
}

func TestParseArgvBooleanFlagTakesInlineFalse(t *testing.T) {
	parsed, err := ParseArgv([]string{"list", "--json=false", "--all=true"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if parsed.Flags.Bool("json") {
		t.Fatal("--json=false parsed as true")
	}
	if !parsed.Flags.Bool("all") {
		t.Fatal("--all=true parsed as false")
	}
}

func TestParseArgvRepeatedTagsAccumulate(t *testing.T) {
	parsed, err := ParseArgv([]string{"add", "title", "-t", "bug", "--tag", "ui", "--tag=perf"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if want := []string{"bug", "ui", "perf"}; !reflect.DeepEqual(parsed.Flags.List("tag"), want) {
		t.Fatalf("tag = %q, want %q", parsed.Flags.List("tag"), want)
	}
}

func TestParseArgvSingleTagIsStillAList(t *testing.T) {
	parsed, err := ParseArgv([]string{"add", "title", "-t", "bug"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if want := []string{"bug"}; !reflect.DeepEqual(parsed.Flags.List("tag"), want) {
		t.Fatalf("tag = %q, want %q", parsed.Flags.List("tag"), want)
	}
}

func TestParseArgvDoubleDashEndsFlagParsing(t *testing.T) {
	parsed, err := ParseArgv([]string{"note", "k3f9", "--", "--not-a-flag", "-p"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if parsed.Command != "note" {
		t.Fatalf("command = %q, want %q", parsed.Command, "note")
	}
	if want := []string{"k3f9", "--not-a-flag", "-p"}; !reflect.DeepEqual(parsed.Args, want) {
		t.Fatalf("args = %q, want %q", parsed.Args, want)
	}
	if count := parsed.Flags.count(); count != 0 {
		t.Fatalf("flags carried %d entries, want none", count)
	}
}

func TestParseArgvLoneDashIsAPositional(t *testing.T) {
	parsed, err := ParseArgv([]string{"note", "k3f9", "-"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if want := []string{"k3f9", "-"}; !reflect.DeepEqual(parsed.Args, want) {
		t.Fatalf("args = %q, want %q", parsed.Args, want)
	}
}

func TestParseArgvValueFlagWithoutValueIsAUserError(t *testing.T) {
	for _, argv := range [][]string{
		{"add", "title", "-p"},
		{"add", "title", "-p", "--json"},
	} {
		_, err := ParseArgv(argv)

		var user *UserError
		if !errors.As(err, &user) {
			t.Fatalf("ParseArgv(%q) returned %v, want a UserError", argv, err)
		}
		if user.Message != "flag --project requires a value" {
			t.Fatalf("message = %q, want the expanded flag name", user.Message)
		}
	}
}

func TestParseArgvEmptyArgvYieldsNoCommand(t *testing.T) {
	parsed, err := ParseArgv(nil)
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if parsed.Command != "" {
		t.Fatalf("command = %q, want empty", parsed.Command)
	}
	if len(parsed.Args) != 0 {
		t.Fatalf("args = %q, want none", parsed.Args)
	}
	if count := parsed.Flags.count(); count != 0 {
		t.Fatalf("flags carried %d entries, want none", count)
	}
}

func TestParseArgvFlagsMayPrecedeTheCommand(t *testing.T) {
	parsed, err := ParseArgv([]string{"--json", "loops", "-p", "."})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if parsed.Command != "loops" {
		t.Fatalf("command = %q, want %q", parsed.Command, "loops")
	}
	if !parsed.Flags.Bool("json") {
		t.Fatal("--json did not parse as true")
	}
	if got, _ := parsed.Flags.String("project"); got != "." {
		t.Fatalf("project = %q, want %q", got, ".")
	}
}

func TestParseArgvDropsInteractive(t *testing.T) {
	parsed, err := ParseArgv([]string{"loops", "-i", "picker"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	// `-i` is no longer a boolean flag, so it reads as an unknown value flag rather than a picker.
	if parsed.Flags.Bool("interactive") {
		t.Fatal("-i still parses as the interactive boolean")
	}
	if got, _ := parsed.Flags.String("i"); got != "picker" {
		t.Fatalf("i = %q, want it treated as an unrecognised value flag", got)
	}
}

func TestFlagAccessorsReportAbsence(t *testing.T) {
	parsed, err := ParseArgv([]string{"list"})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	if value, ok := parsed.Flags.String("project"); ok || value != "" {
		t.Fatalf("String(project) = %q, %v, want an absent flag", value, ok)
	}
	if got := parsed.Flags.List("tag"); len(got) != 0 {
		t.Fatalf("List(tag) = %q, want none", got)
	}
	if parsed.Flags.Bool("all") {
		t.Fatal("Bool(all) is true for an unpassed flag")
	}
}

func TestFlagStringDistinguishesEmptyFromAbsent(t *testing.T) {
	parsed, err := ParseArgv([]string{"list", "--project="})
	if err != nil {
		t.Fatalf("ParseArgv returned %v", err)
	}

	value, ok := parsed.Flags.String("project")
	if !ok || value != "" {
		t.Fatalf("String(project) = %q, %v, want an empty value that is present", value, ok)
	}
}
