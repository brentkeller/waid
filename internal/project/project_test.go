package project

import (
	"errors"
	"reflect"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/events"
)

func item(id string, project *string) events.Item {
	return events.Item{
		Id:      id,
		Title:   "title",
		Status:  events.StatusOpen,
		Project: project,
		Tags:    []string{},
		Notes:   []events.Note{},
		Created: "2026-08-14T00:00:00.000Z",
		Updated: "2026-08-14T00:00:00.000Z",
	}
}

var known = []string{`C:\dev\waid`, `C:\dev\dr\devresults`, "/home/brent/scratch"}

func TestNormalizePathTrimsAndStripsTrailingSeparators(t *testing.T) {
	cases := map[string]string{
		`  C:\dev\waid  `:       `C:\dev\waid`,
		`C:\dev\waid\`:          `C:\dev\waid`,
		`C:\dev\waid\\`:         `C:\dev\waid`,
		"/home/brent/scratch/":  "/home/brent/scratch",
		"/home/brent/scratch//": "/home/brent/scratch",
	}

	for input, want := range cases {
		if got := NormalizePath(input); got != want {
			t.Fatalf("NormalizePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizePathLeavesBareRootsAlone(t *testing.T) {
	cases := map[string]string{
		`C:\`:   `C:\`,
		"C:/":   "C:/",
		"C:":    "C:",
		"/":     "/",
		"  /  ": "/",
		`\\`:    `\\`,
		`C:\\\`: `C:\\\`,
	}

	for input, want := range cases {
		if got := NormalizePath(input); got != want {
			t.Fatalf("NormalizePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestKnownProjectsUnionsItemAndSessionProjectsDroppingNulls(t *testing.T) {
	items := []events.Item{
		item("0001", ptr(`C:\dev\waid`)),
		item("0002", nil),
		item("0003", ptr(`C:\dev\dr`)),
	}
	sessions := []string{`C:\dev\other`, ""}

	want := []string{`C:\dev\waid`, `C:\dev\dr`, `C:\dev\other`}
	if got := KnownProjects(items, sessions); !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownProjects = %q, want %q", got, want)
	}
}

func TestKnownProjectsDeduplicatesPreservingFirstSeenOrder(t *testing.T) {
	items := []events.Item{
		item("0001", ptr(`C:\dev\waid`)),
		item("0002", ptr(`C:\dev\dr`)),
		item("0003", ptr(`C:\dev\waid`)),
	}

	want := []string{`C:\dev\waid`, `C:\dev\dr`}
	if got := KnownProjects(items, []string{`C:\dev\dr`}); !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownProjects = %q, want %q", got, want)
	}
}

func TestKnownProjectsWorksWithoutSessions(t *testing.T) {
	items := []events.Item{item("0001", ptr(`C:\dev\waid`))}

	want := []string{`C:\dev\waid`}
	if got := KnownProjects(items, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownProjects = %q, want %q", got, want)
	}
}

func TestKnownProjectsIsEmptyWithoutProjects(t *testing.T) {
	got := KnownProjects(nil, nil)
	if len(got) != 0 {
		t.Fatalf("KnownProjects = %q, want none", got)
	}
}

func TestResolveMapsEmptyInputToNothing(t *testing.T) {
	for _, input := range []string{"", "   "} {
		got, err := Resolve(input, known, `C:\cwd`)
		if err != nil {
			t.Fatalf("Resolve(%q) returned %v", input, err)
		}
		if got != nil {
			t.Fatalf("Resolve(%q) = %q, want nil", input, *got)
		}
	}
}

func TestResolveMapsDotToTheNormalizedCwd(t *testing.T) {
	cases := []struct{ input, cwd, want string }{
		{".", `C:\dev\elsewhere\`, `C:\dev\elsewhere`},
		{" . ", "/home/brent/", "/home/brent"},
	}

	for _, c := range cases {
		got, err := Resolve(c.input, known, c.cwd)
		if err != nil {
			t.Fatalf("Resolve(%q, %q) returned %v", c.input, c.cwd, err)
		}
		if got == nil || *got != c.want {
			t.Fatalf("Resolve(%q, %q) = %v, want %q", c.input, c.cwd, got, c.want)
		}
	}
}

func TestResolveTakesAbsolutePathsAsGiven(t *testing.T) {
	cases := map[string]string{
		`C:\dev\brand-new\`: `C:\dev\brand-new`,
		"/var/tmp/x/":       "/var/tmp/x",
		"D:/data/repo":      "D:/data/repo",
	}

	for input, want := range cases {
		got, err := Resolve(input, known, `C:\cwd`)
		if err != nil {
			t.Fatalf("Resolve(%q) returned %v", input, err)
		}
		if got == nil || *got != want {
			t.Fatalf("Resolve(%q) = %v, want %q", input, got, want)
		}
	}
}

func TestResolveMatchesAPartialCaseInsensitively(t *testing.T) {
	cases := map[string]string{
		"waid":       `C:\dev\waid`,
		"DEVRESULTS": `C:\dev\dr\devresults`,
		"scratch":    "/home/brent/scratch",
	}

	for input, want := range cases {
		got, err := Resolve(input, known, `C:\cwd`)
		if err != nil {
			t.Fatalf("Resolve(%q) returned %v", input, err)
		}
		if got == nil || *got != want {
			t.Fatalf("Resolve(%q) = %v, want %q", input, got, want)
		}
	}
}

func TestResolveRejectsAPartialMatchingNothing(t *testing.T) {
	_, err := Resolve("nope", known, `C:\cwd`)

	var userErr *cli.UserError
	if !errors.As(err, &userErr) {
		t.Fatalf("Resolve returned %v, want a UserError", err)
	}
	if want := "no known project matches: nope"; userErr.Message != want {
		t.Fatalf("message = %q, want %q", userErr.Message, want)
	}
}

func TestResolveRejectsAnAmbiguousPartialAndCarriesTheCandidates(t *testing.T) {
	_, err := Resolve("dev", known, `C:\cwd`)

	var userErr *cli.UserError
	if !errors.As(err, &userErr) {
		t.Fatalf("Resolve returned %v, want a UserError", err)
	}
	if want := "ambiguous project: dev"; userErr.Message != want {
		t.Fatalf("message = %q, want %q", userErr.Message, want)
	}
	want := []string{`C:\dev\waid`, `C:\dev\dr\devresults`}
	if !reflect.DeepEqual(userErr.Candidates, want) {
		t.Fatalf("candidates = %q, want %q", userErr.Candidates, want)
	}
}

func TestResolveRejectsAnyPartialWhenNothingIsKnown(t *testing.T) {
	_, err := Resolve("waid", nil, `C:\cwd`)

	var userErr *cli.UserError
	if !errors.As(err, &userErr) {
		t.Fatalf("Resolve returned %v, want a UserError", err)
	}
}

func TestGroupByProjectKeepsFirstSeenOrderWithOrphansLast(t *testing.T) {
	items := []events.Item{
		item("0001", ptr(`C:\dev\waid`)),
		item("0002", nil),
		item("0003", ptr(`C:\dev\dr`)),
		item("0004", ptr(`C:\dev\waid`)),
		item("0005", nil),
	}

	groups := GroupByProject(items)
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	if groups[0].Project == nil || *groups[0].Project != `C:\dev\waid` {
		t.Fatalf("groups[0].Project = %v, want C:\\dev\\waid", groups[0].Project)
	}
	if ids := itemIds(groups[0]); !reflect.DeepEqual(ids, []string{"0001", "0004"}) {
		t.Fatalf("groups[0] items = %q, want 0001 0004", ids)
	}
	if groups[1].Project == nil || *groups[1].Project != `C:\dev\dr` {
		t.Fatalf("groups[1].Project = %v, want C:\\dev\\dr", groups[1].Project)
	}
	if groups[2].Project != nil {
		t.Fatalf("groups[2].Project = %q, want nil", *groups[2].Project)
	}
	if ids := itemIds(groups[2]); !reflect.DeepEqual(ids, []string{"0002", "0005"}) {
		t.Fatalf("orphan items = %q, want 0002 0005", ids)
	}
}

func TestGroupByProjectOmitsTheOrphanGroupWhenEveryItemHasAProject(t *testing.T) {
	items := []events.Item{item("0001", ptr(`C:\dev\waid`))}

	groups := GroupByProject(items)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
}

func itemIds(group Group) []string {
	ids := make([]string, 0, len(group.Items))
	for _, item := range group.Items {
		ids = append(ids, item.Id)
	}
	return ids
}
