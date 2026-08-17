package harness_test

import (
	"encoding/json"
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
	path := filepath.Join(home, "config.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	settings := map[string]any{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	settings["ghUser"] = "octocat"

	patched, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("encoding %s: %v", path, err)
	}
	if err := os.WriteFile(path, patched, 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return home
}

// ghFixture is the recorded GitHub response both builds are served instead of querying gh.
func ghFixture(t testing.TB) string {
	t.Helper()

	return filepath.Join(harness.RepoRoot(t), "internal", "gh", "testdata", "fixture.json")
}

// The read commands are diffed against Node by what they print, in both the machine and the human
// shape, since the two are views of the same value and either one can drift alone.
func TestReadCommandsMatchNode(t *testing.T) {
	invocations := map[string]harness.Invocation{
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
	}

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, harness.GoldenHome, invocation)
		})
	}
}

// Detection is diffed separately, because both builds have to be served the same recorded GitHub
// response and told to look at a GitHub user before either produces a signal at all.
func TestDetectionCommandsMatchNode(t *testing.T) {
	fixture := ghFixture(t)
	invocations := map[string]harness.Invocation{
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

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, detectionHome, invocation)
		})
	}
}

// The same two commands on the golden home as it stands, where no GitHub user is configured and the
// signals degrade to a note.
func TestDetectionCommandsMatchNodeWithoutAGhUser(t *testing.T) {
	invocations := map[string]harness.Invocation{
		"scan":       {Args: []string{"scan"}},
		"scan json":  {Args: []string{"scan", "--json"}},
		"loops":      {Args: []string{"loops"}},
		"loops json": {Args: []string{"loops", "--json"}},
	}

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, harness.GoldenHome, invocation)
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

func TestDoctorReportsUnknownConfigKeysLikeNode(t *testing.T) {
	for name, invocation := range map[string]harness.Invocation{
		"doctor":      {Args: []string{"doctor"}},
		"doctor json": {Args: []string{"doctor", "--json"}},
	} {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, typoConfigHome, invocation)
		})
	}
}

// The same commands against a copy of the real data directory, where the notes carry characters no
// synthetic fixture thought to include.
func TestReadCommandsMatchNodeOnTheRealHome(t *testing.T) {
	invocations := map[string]harness.Invocation{
		"list":      {Args: []string{"list"}},
		"list json": {Args: []string{"list", "--json"}},
		"list all":  {Args: []string{"list", "--all"}},
	}

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, harness.RealHome, invocation)
		})
	}
}
