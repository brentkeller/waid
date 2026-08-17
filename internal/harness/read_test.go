package harness_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brentkeller/waid/internal/harness"
)

// detectionHome is the golden fixture with GitHub signals switched on, so the recorded gh response
// reaches detection. The scan roots stay empty: a real repo's dirty count and HEAD date are not
// something a comparison can depend on.
func detectionHome(t testing.TB) string {
	t.Helper()

	home := harness.GoldenHome(t)
	harness.PatchConfig(t, home, func(settings map[string]any) {
		settings["ghUser"] = "octocat"
	})
	return home
}

// ghFixture is the recorded GitHub response both builds are served instead of querying gh.
func ghFixture(t testing.TB) string {
	t.Helper()

	return filepath.Join(harness.RepoRoot(t), "internal", "gh", "testdata", "fixture.json")
}

// goldenReadInvocations are the read commands run against the fixture home, in both the machine and
// the human shape, since the two are views of the same value and either one can break alone.
func goldenReadInvocations() map[string]harness.Invocation {
	return map[string]harness.Invocation{
		"list":                          {Args: []string{"list"}},
		"list json":                     {Args: []string{"list", "--json"}},
		"list all":                      {Args: []string{"list", "--all"}},
		"list all json":                 {Args: []string{"list", "--all", "--json"}},
		"list by status":                {Args: []string{"list", "--status", "done", "--json"}},
		"list by tag":                   {Args: []string{"list", "--tag", "bug", "--json"}},
		"list by project":               {Args: []string{"list", "-p", "program files"}},
		"list by partial project json":  {Args: []string{"list", "-p", "waid", "--json"}},
		"list unknown status":           {Args: []string{"list", "--status", "nope"}},
		"list unknown status json":      {Args: []string{"list", "--status", "nope", "--json"}},
		"list unknown project":          {Args: []string{"list", "-p", "nope", "--json"}},
		"show":                          {Args: []string{"show", "a1b2"}},
		"show json":                     {Args: []string{"show", "a1b2", "--json"}},
		"show a reopened item":          {Args: []string{"show", "e5f6"}},
		"show without notes json":       {Args: []string{"show", "c3d4", "--json"}},
		"show unknown id":               {Args: []string{"show", "zzzz"}},
		"show unknown id json":          {Args: []string{"show", "zzzz", "--json"}},
		"show without an id":            {Args: []string{"show"}},
		"today":                         {Args: []string{"today"}},
		"today json":                    {Args: []string{"today", "--json"}},
		"today with item activity":      {Args: []string{"today", "--date", "2026-08-16"}},
		"today with activity json":      {Args: []string{"today", "--date", "2026-08-16", "--json"}},
		"today on an empty day":         {Args: []string{"today", "--date", "2026-08-01"}},
		"today on an empty day json":    {Args: []string{"today", "--date", "2026-08-01", "--json"}},
		"today with a bad date":         {Args: []string{"today", "--date", "yesterday"}},
		"today with a bad date json":    {Args: []string{"today", "--date", "yesterday", "--json"}},
		"today with an impossible date": {Args: []string{"today", "--date", "2026-02-31"}},
		"week":                          {Args: []string{"week"}},
		"week json":                     {Args: []string{"week", "--json"}},
		"week last":                     {Args: []string{"week", "--last"}},
		"week last json":                {Args: []string{"week", "--last", "--json"}},
		"doctor":                        {Args: []string{"doctor"}},
		"doctor json":                   {Args: []string{"doctor", "--json"}},
		"sync":                          {Args: []string{"sync"}},
		"sync json":                     {Args: []string{"sync", "--json"}},
		"sync full json":                {Args: []string{"sync", "--full", "--json"}},
		"transcript":                    {Args: []string{"transcript", goldenSessionId}},
		"transcript json":               {Args: []string{"transcript", goldenSessionId, "--json"}},
		"transcript unknown id":         {Args: []string{"transcript", "no-such-session"}},
		"transcript unknown id json":    {Args: []string{"transcript", "no-such-session", "--json"}},
		"transcript without an id":      {Args: []string{"transcript"}},
	}
}

// goldenSessionId is a session the fixture transcript directory carries.
const goldenSessionId = "aaaa1111-2222-3333-4444-555555555555"

func TestReadCommandsRunOnTheFixtureHome(t *testing.T) {
	for name, invocation := range goldenReadInvocations() {
		t.Run(name, func(t *testing.T) {
			exercise(t, harness.GoldenHome(t), invocation)
		})
	}
}

// detectionInvocations are held separately, because the run has to be served the recorded GitHub
// response and told to look at a GitHub user before detection produces a signal at all.
func detectionInvocations(fixture string) map[string]harness.Invocation {
	return map[string]harness.Invocation{
		"scan":                        {Args: []string{"scan"}, GhFixture: fixture},
		"scan json":                   {Args: []string{"scan", "--json"}, GhFixture: fixture},
		"scan by project":             {Args: []string{"scan", "-p", "waid"}, GhFixture: fixture},
		"scan by project json":        {Args: []string{"scan", "-p", "waid", "--json"}, GhFixture: fixture},
		"scan unknown project":        {Args: []string{"scan", "-p", "nope"}, GhFixture: fixture},
		"scan unknown project json":   {Args: []string{"scan", "-p", "nope", "--json"}, GhFixture: fixture},
		"loops":                       {Args: []string{"loops"}, GhFixture: fixture},
		"loops json":                  {Args: []string{"loops", "--json"}, GhFixture: fixture},
		"loops by project":            {Args: []string{"loops", "-p", "waid"}, GhFixture: fixture},
		"loops by project json":       {Args: []string{"loops", "-p", "waid", "--json"}, GhFixture: fixture},
		"loops by empty project json": {Args: []string{"loops", "-p", "demo project", "--json"}, GhFixture: fixture},
	}
}

func TestDetectionCommandsRunOnTheFixtureHome(t *testing.T) {
	for name, invocation := range detectionInvocations(ghFixture(t)) {
		t.Run(name, func(t *testing.T) {
			exercise(t, detectionHome(t), invocation)
		})
	}
}

// The same two commands on the golden home as it stands, where no GitHub user is configured and the
// signals degrade to a note.
func TestDetectionCommandsRunWithoutAGhUser(t *testing.T) {
	invocations := map[string]harness.Invocation{
		"scan":       {Args: []string{"scan"}},
		"scan json":  {Args: []string{"scan", "--json"}},
		"loops":      {Args: []string{"loops"}},
		"loops json": {Args: []string{"loops", "--json"}},
	}

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			exercise(t, harness.GoldenHome(t), invocation)
		})
	}
}

// A home whose config.json carries keys waid does not recognise, which is the branch of doctor's
// report the clean fixture never reaches.
func typoConfigHome(t testing.TB) string {
	t.Helper()

	home := harness.GoldenHome(t)
	path := filepath.Join(home, "config.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	// Appended as text rather than re-encoded, so the keys stay in the order the report prints them.
	patched := strings.TrimRight(string(raw), " \r\n")
	patched = strings.TrimSuffix(patched, "}") + `,"scanDepth":4,"ghUsr":"someone"}`
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return home
}

func TestDoctorRunsOnAHomeWithUnknownConfigKeys(t *testing.T) {
	for name, invocation := range map[string]harness.Invocation{
		"doctor":      {Args: []string{"doctor"}},
		"doctor json": {Args: []string{"doctor", "--json"}},
	} {
		t.Run(name, func(t *testing.T) {
			out := exercise(t, typoConfigHome(t), invocation)

			for _, key := range []string{"scanDepth", "ghUsr"} {
				if !strings.Contains(out.Stdout, key) {
					t.Errorf("doctor does not report the unknown key %s:\n%s", key, out.Stdout)
				}
			}
		})
	}
}
