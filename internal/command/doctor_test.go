package command_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/command"
	"github.com/brentkeller/waid/internal/events"
)

// doctorHome is a home with no scan roots, so the repo section costs nothing and never depends on
// what happens to be checked out on the machine running the tests.
func doctorHome(t *testing.T) string {
	t.Helper()

	home := makeHome(t)
	seedConfig(t, home, `{ "scanRoots": [] }`)
	return home
}

// seedConfig writes config.json verbatim, so a test can control both the keys it carries and the
// order they appear in.
func seedConfig(t *testing.T, home, raw string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(raw+"\n"), 0o644); err != nil {
		t.Fatalf("writing config.json: %v", err)
	}
}

// seedLog writes the log directly, so the line numbers the report names are deterministic.
func seedRawLog(t *testing.T, home string, lines ...string) {
	t.Helper()

	raw := ""
	for _, line := range lines {
		raw += line + "\n"
	}
	if err := os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(raw), 0o644); err != nil {
		t.Fatalf("writing the log: %v", err)
	}
}

// doctor runs the command and decodes its report.
func doctor(t *testing.T, home string) command.DoctorResult {
	t.Helper()

	run := waid(t, home, "doctor", "--json")
	if run.code != cli.ExitOK {
		t.Fatalf("doctor exited %d: %s", run.code, run.err)
	}

	var data command.DoctorResult
	run.decode(t, &data)
	return data
}

func TestACleanHomeReportsOkWithZeroProblems(t *testing.T) {
	home := doctorHome(t)

	data := doctor(t, home)

	if !data.Ok {
		t.Errorf("ok = false, want true")
	}
	if data.Home != home {
		t.Errorf("home = %q, want %q", data.Home, home)
	}
	if !data.Config.Valid || len(data.Config.UnknownKeys) != 0 {
		t.Errorf("config = %+v, want a valid one with no unknown keys", data.Config)
	}
	if want := filepath.Join(home, "config.json"); data.Config.Path != want {
		t.Errorf("config.path = %q, want %q", data.Config.Path, want)
	}
	if want := filepath.Join(home, "events.jsonl"); data.Log.Path != want {
		t.Errorf("log.path = %q, want %q", data.Log.Path, want)
	}
	if data.Log.Lines != 0 || data.Log.Items != 0 || len(data.Log.Problems) != 0 {
		t.Errorf("log = %+v, want an empty one", data.Log)
	}
}

func TestAHomeWithItemsCountsLinesAndItems(t *testing.T) {
	home := doctorHome(t)
	seedRawLog(t, home,
		`{"ts":"2026-08-10T09:00:00.000Z","ev":"add","id":"k3f9","title":"One"}`,
		`{"ts":"2026-08-11T09:00:00.000Z","ev":"add","id":"m7qz","title":"Two"}`,
		`{"ts":"2026-08-12T09:00:00.000Z","ev":"close","id":"k3f9"}`,
	)

	data := doctor(t, home)

	if data.Log.Lines != 3 || data.Log.Items != 2 {
		t.Errorf("log = %d lines, %d items; want 3 and 2", data.Log.Lines, data.Log.Items)
	}
	if len(data.Log.Problems) != 0 {
		t.Errorf("problems = %+v, want none", data.Log.Problems)
	}
	if !data.Ok {
		t.Errorf("ok = false, want true")
	}
}

func TestUnusableLinesAreReportedWithLineNumbersAndExitZero(t *testing.T) {
	home := doctorHome(t)
	seedRawLog(t, home,
		`{"ts":"2026-08-10T09:00:00.000Z","ev":"add","id":"k3f9","title":"One"}`,
		`{ not json at all`,
		`{"ts":"2026-08-12T09:00:00.000Z","ev":"close","id":"nope"}`,
	)

	data := doctor(t, home)

	if data.Ok {
		t.Errorf("ok = true, want false")
	}
	if data.Log.Lines != 3 || data.Log.Items != 1 {
		t.Errorf("log = %d lines, %d items; want 3 and 1", data.Log.Lines, data.Log.Items)
	}
	if len(data.Log.Problems) != 2 {
		t.Fatalf("problems = %+v, want two", data.Log.Problems)
	}
	if got := data.Log.Problems[0]; got.Line != 2 || got.Reason != events.ReasonUnparseable || got.Id != nil || got.Ev != nil {
		t.Errorf("first problem = %+v", got)
	}
	second := data.Log.Problems[1]
	if second.Line != 3 || second.Reason != events.ReasonUnknownId {
		t.Errorf("second problem = %+v", second)
	}
	if second.Id == nil || *second.Id != "nope" || second.Ev == nil || *second.Ev != "close" {
		t.Errorf("second problem lost its id or ev: %+v", second)
	}
}

// Every reason the fold can report reaches the report, so a log waid cannot fully apply is legible
// whatever went wrong with it.
func TestEveryProblemReasonReachesTheReport(t *testing.T) {
	home := doctorHome(t)
	seedRawLog(t, home,
		`{"ts":"2026-08-16T09:00:00.000Z","ev":"add","id":"a1b2","title":"Seed"}`,
		`{"ts":"2026-08-16T09:01:00.000Z","ev":"add","id":`,
		`["not","an","object"]`,
		`{"ts":"2026-08-16T09:02:00.000Z","ev":"add","title":"no id"}`,
		`{"ts":"2026-08-16T09:03:00.000Z","ev":"add","id":"a1b2","title":"again"}`,
		`{"ts":"2026-08-16T09:04:00.000Z","ev":"note","id":"zzzz","text":"nobody"}`,
		`{"ts":"2026-08-16T09:05:00.000Z","ev":"note","id":"a1b2"}`,
		`{"ts":"2026-08-16T09:06:00.000Z","ev":"update","id":"a1b2","status":"nonsense"}`,
		`{"ts":"2026-08-16T09:07:00.000Z","ev":"dismiss"}`,
		`{"ts":"2026-08-16T09:08:00.000Z","ev":"frobnicate","id":"a1b2"}`,
	)

	data := doctor(t, home)

	seen := map[events.ProblemReason]int{}
	for _, problem := range data.Log.Problems {
		seen[problem.Reason] = problem.Line
	}
	for _, reason := range []events.ProblemReason{
		events.ReasonUnparseable,
		events.ReasonNotAnObject,
		events.ReasonAddMissingField,
		events.ReasonDuplicateId,
		events.ReasonUnknownId,
		events.ReasonNoteMissingText,
		events.ReasonBadStatus,
		events.ReasonMissingKey,
		events.ReasonUnknownEv,
	} {
		if _, found := seen[reason]; !found {
			t.Errorf("no %s problem in %+v", reason, data.Log.Problems)
		}
	}
	if data.Ok {
		t.Errorf("ok = true, want false")
	}
}

func TestUnknownConfigKeysAreListedInFileOrderAndDropOk(t *testing.T) {
	home := doctorHome(t)
	seedConfig(t, home, `{ "scanRoots": [], "scanDepth": 4, "ghUsr": "someone" }`)

	data := doctor(t, home)

	if got := strings.Join(data.Config.UnknownKeys, ","); got != "scanDepth,ghUsr" {
		t.Errorf("unknownKeys = %q, want the file's own order", got)
	}
	if !data.Config.Valid {
		t.Errorf("valid = false; an unknown key is a typo, not an unreadable file")
	}
	if data.Ok {
		t.Errorf("ok = true, want false")
	}
}

func TestGhReportsUnavailableWhenDetectionIsSkipped(t *testing.T) {
	home := doctorHome(t)
	seedConfig(t, home, `{ "scanRoots": [], "ghUser": "brentkeller" }`)

	data := doctor(t, home)

	if data.Gh.Available {
		t.Errorf("available = true, want false")
	}
	if data.Gh.User == nil || *data.Gh.User != "brentkeller" {
		t.Errorf("user = %v, want the configured login", data.Gh.User)
	}
}

func TestTheSessionCacheDegradesToNotBuiltBeforeAnySync(t *testing.T) {
	home := doctorHome(t)

	data := doctor(t, home)

	if data.Cache.Sessions.Exists || data.Cache.Sessions.SyncedAt != nil || data.Cache.Sessions.AgeMinutes != nil {
		t.Errorf("sessions cache = %+v, want a not-built one", data.Cache.Sessions)
	}
	if data.Cache.Gh.AgeMinutes != nil {
		t.Errorf("gh cache age = %v, want none", data.Cache.Gh.AgeMinutes)
	}
}

func TestABuiltSessionCacheIsReportedWithItsAge(t *testing.T) {
	home := doctorHome(t)
	if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
		t.Fatalf("creating the cache directory: %v", err)
	}
	cache := `{"version":2,"syncedAt":"2026-08-17T11:30:00.000Z","sessions":[]}`
	if err := os.WriteFile(filepath.Join(home, "cache", "sessions.json"), []byte(cache), 0o644); err != nil {
		t.Fatalf("writing the cache: %v", err)
	}
	t.Setenv(cli.EnvNow, "2026-08-17T12:00:00.000Z")

	data := doctor(t, home)

	if !data.Cache.Sessions.Exists {
		t.Fatalf("exists = false, want true")
	}
	if data.Cache.Sessions.SyncedAt == nil || *data.Cache.Sessions.SyncedAt != "2026-08-17T11:30:00.000Z" {
		t.Errorf("syncedAt = %v", data.Cache.Sessions.SyncedAt)
	}
	if data.Cache.Sessions.AgeMinutes == nil || *data.Cache.Sessions.AgeMinutes != 30 {
		t.Errorf("ageMinutes = %v, want 30", data.Cache.Sessions.AgeMinutes)
	}
}

func TestDoctorMutatesNothing(t *testing.T) {
	home := doctorHome(t)
	seedRawLog(t, home,
		`{"ts":"2026-08-10T09:00:00.000Z","ev":"add","id":"k3f9","title":"One"}`,
		`{ not json at all`,
	)

	eventsPath := filepath.Join(home, "events.jsonl")
	configPath := filepath.Join(home, "config.json")
	waid(t, home, "doctor")
	before := map[string]string{eventsPath: readFile(t, eventsPath), configPath: readFile(t, configPath)}

	doctor(t, home)

	for path, want := range before {
		if got := readFile(t, path); got != want {
			t.Errorf("%s changed:\n%q\n%q", path, want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "cache", "sessions.json")); !os.IsNotExist(err) {
		t.Errorf("doctor built the session cache the report describes")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(raw)
}

func TestHumanDoctorReportsEverySectionAndListsProblems(t *testing.T) {
	home := doctorHome(t)
	seedRawLog(t, home,
		`{"ts":"2026-08-10T09:00:00.000Z","ev":"add","id":"k3f9","title":"One"}`,
		`{ not json at all`,
	)

	run := waid(t, home, "doctor")
	if run.code != cli.ExitOK {
		t.Fatalf("doctor exited %d: %s", run.code, run.err)
	}

	if first := strings.Split(run.out, "\n")[0]; first != "DOCTOR" {
		t.Errorf("first line = %q, want DOCTOR", first)
	}
	for _, pattern := range []string{
		`\n {2}home {2,}`,
		`\n {2}config {2,}`,
		`\n {2}log {2,}`,
		`\n {2}cache {2,}`,
		`\n {2}gh {2,}`,
		`\n {2}repos {2,}`,
		`PROBLEMS`,
		`line 2 {2,}unparseable`,
	} {
		if !regexp.MustCompile(pattern).MatchString(run.out) {
			t.Errorf("no /%s/ in:\n%s", pattern, run.out)
		}
	}
}

func TestHumanDoctorOnACleanHomeSaysItIsOk(t *testing.T) {
	home := doctorHome(t)

	run := waid(t, home, "doctor")
	if run.code != cli.ExitOK {
		t.Fatalf("doctor exited %d: %s", run.code, run.err)
	}

	if strings.Contains(run.out, "PROBLEMS") {
		t.Errorf("a clean home listed problems:\n%s", run.out)
	}
	if !regexp.MustCompile(`\bok\b`).MatchString(run.out) {
		t.Errorf("no verdict in:\n%s", run.out)
	}
}

// treeLog holds one orphan and one two-node cycle, neither of which the per-line fold can see.
func seedTreeLog(t *testing.T, home string) {
	t.Helper()

	seedRawLog(t, home,
		`{"ts":"2026-08-18T09:00:00.000Z","ev":"add","id":"aaaa","title":"Anchor"}`,
		`{"ts":"2026-08-18T09:01:00.000Z","ev":"add","id":"bbbb","title":"Below","parent":"aaaa"}`,
		`{"ts":"2026-08-18T09:02:00.000Z","ev":"add","id":"lost","title":"Orphan","parent":"gone"}`,
		`{"ts":"2026-08-18T09:03:00.000Z","ev":"update","id":"aaaa","parent":"bbbb"}`,
	)
}

func TestOrphansAndCyclesAreReportedAsProblems(t *testing.T) {
	home := doctorHome(t)
	seedTreeLog(t, home)

	data := doctor(t, home)

	orphans := []string{}
	cycles := []string{}
	for _, problem := range data.Log.Problems {
		switch problem.Reason {
		case events.ReasonUnknownParent:
			orphans = append(orphans, derefId(problem))
		case events.ReasonParentCycle:
			cycles = append(cycles, derefId(problem))
		}
	}

	if want := []string{"lost"}; !reflect.DeepEqual(orphans, want) {
		t.Errorf("unknown-parent problems = %v, want %v", orphans, want)
	}
	if want := []string{"aaaa", "bbbb"}; !reflect.DeepEqual(cycles, want) {
		t.Errorf("parent-cycle problems = %v, want %v", cycles, want)
	}
	if data.Ok {
		t.Errorf("ok = true, want false")
	}
}

func TestACleanHierarchyNamesNoOrphansOrCycles(t *testing.T) {
	home := doctorHome(t)
	seedRawLog(t, home,
		`{"ts":"2026-08-18T09:00:00.000Z","ev":"add","id":"aaaa","title":"Anchor"}`,
		`{"ts":"2026-08-18T09:01:00.000Z","ev":"add","id":"bbbb","title":"Below","parent":"aaaa"}`,
	)

	data := doctor(t, home)

	if len(data.Log.Problems) != 0 {
		t.Errorf("problems = %+v, want none", data.Log.Problems)
	}
	if !data.Ok {
		t.Errorf("ok = false, want true")
	}
}

func TestHumanDoctorNamesOrphansAndCycles(t *testing.T) {
	home := doctorHome(t)
	seedTreeLog(t, home)

	run := waid(t, home, "doctor")
	if run.code != cli.ExitOK {
		t.Fatalf("doctor exited %d: %s", run.code, run.err)
	}

	for _, pattern := range []string{
		`PROBLEMS`,
		`unknown-parent {2,}id=lost`,
		`parent-cycle {2,}id=aaaa`,
		`parent-cycle {2,}id=bbbb`,
	} {
		if !regexp.MustCompile(pattern).MatchString(run.out) {
			t.Errorf("no /%s/ in:\n%s", pattern, run.out)
		}
	}
	if regexp.MustCompile(`line 0\b`).MatchString(run.out) {
		t.Errorf("a problem the fold never saw was given a line number:\n%s", run.out)
	}
}

func TestHumanDoctorOnACleanHierarchySaysItIsOk(t *testing.T) {
	home := doctorHome(t)
	seedRawLog(t, home,
		`{"ts":"2026-08-18T09:00:00.000Z","ev":"add","id":"aaaa","title":"Anchor"}`,
		`{"ts":"2026-08-18T09:01:00.000Z","ev":"add","id":"bbbb","title":"Below","parent":"aaaa"}`,
	)

	run := waid(t, home, "doctor")
	if strings.Contains(run.out, "PROBLEMS") {
		t.Errorf("a clean hierarchy listed problems:\n%s", run.out)
	}
}

func derefId(problem events.Problem) string {
	if problem.Id == nil {
		return ""
	}
	return *problem.Id
}
