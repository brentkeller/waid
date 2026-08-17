package harness_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/harness"
)

// realHomeInvocations is every read command run against a copy of the real data directory, where the
// notes carry characters no synthetic fixture thought to include, the project paths hold spaces, and
// the transcripts are the ones actually on this machine.
//
// The ids and project names the invocations address are read out of the real log rather than
// written down here, so the set keeps working as items are added and closed.
func realHomeInvocations(t testing.TB) map[string]harness.Invocation {
	t.Helper()

	fixture := ghFixture(t)
	id := realItemId(t)
	project := realProjectName(t)

	invocations := map[string]harness.Invocation{
		"list":                  {Args: []string{"list"}},
		"list json":             {Args: []string{"list", "--json"}},
		"list all":              {Args: []string{"list", "--all"}},
		"list all json":         {Args: []string{"list", "--all", "--json"}},
		"list by tag":           {Args: []string{"list", "--tag", "promoted", "--json"}},
		"show":                  {Args: []string{"show", id}},
		"show json":             {Args: []string{"show", id, "--json"}},
		"show unknown id":       {Args: []string{"show", "zzzz"}},
		"today":                 {Args: []string{"today"}},
		"today json":            {Args: []string{"today", "--json"}},
		"week":                  {Args: []string{"week"}},
		"week json":             {Args: []string{"week", "--json"}},
		"week last json":        {Args: []string{"week", "--last", "--json"}},
		"doctor":                {Args: []string{"doctor"}},
		"doctor json":           {Args: []string{"doctor", "--json"}},
		"sync":                  {Args: []string{"sync"}},
		"sync json":             {Args: []string{"sync", "--json"}},
		"scan":                  {Args: []string{"scan"}, GhFixture: fixture},
		"scan json":             {Args: []string{"scan", "--json"}, GhFixture: fixture},
		"loops":                 {Args: []string{"loops"}, GhFixture: fixture},
		"loops json":            {Args: []string{"loops", "--json"}, GhFixture: fixture},
		"loops unknown project": {Args: []string{"loops", "-p", "no such project", "--json"}, GhFixture: fixture},
	}
	if project != "" {
		invocations["list by project"] = harness.Invocation{Args: []string{"list", "-p", project}}
		invocations["list by project json"] = harness.Invocation{Args: []string{"list", "-p", project, "--json"}}
	}
	return invocations
}

func TestReadCommandsMatchNodeOnTheRealHome(t *testing.T) {
	for name, invocation := range realHomeInvocations(t) {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, harness.RealHomePinned, invocation)
		})
	}
}

// The detection commands again, this time discovering and probing the real working trees under the
// configured scan roots. Opt-in: see harness.RealHomeLiveRepos.
func TestDetectionMatchesNodeAgainstLiveRepos(t *testing.T) {
	fixture := ghFixture(t)
	invocations := map[string]harness.Invocation{
		"scan":       {Args: []string{"scan"}, GhFixture: fixture},
		"scan json":  {Args: []string{"scan", "--json"}, GhFixture: fixture},
		"loops":      {Args: []string{"loops"}, GhFixture: fixture},
		"loops json": {Args: []string{"loops", "--json"}, GhFixture: fixture},
		"doctor":     {Args: []string{"doctor"}},
	}

	for name, invocation := range invocations {
		t.Run(name, func(t *testing.T) {
			harness.CompareRead(t, harness.RealHomeLiveRepos, invocation)
		})
	}
}

// realItemId is the id show is pointed at: the first item carrying notes, since an item with notes
// exercises more of the rendering than one without.
func realItemId(t testing.TB) string {
	t.Helper()

	items := realState(t).Items
	if len(items) == 0 {
		t.Skipf("the real log at %s holds no items", harness.RealHomePath())
	}
	for _, item := range items {
		if len(item.Notes) > 0 {
			return item.Id
		}
	}
	return items[0].Id
}

// realProjectName is the leaf directory of the first project an item names, which is what a person
// types after -p. It is empty when no item names a project.
func realProjectName(t testing.TB) string {
	t.Helper()

	for _, item := range realState(t).Items {
		if item.Project != nil && *item.Project != "" {
			return filepath.Base(*item.Project)
		}
	}
	return ""
}

// realState folds the real log where it lies. It is only ever read: the copies the comparison runs
// against are made by the harness.
func realState(t testing.TB) events.State {
	t.Helper()

	path := filepath.Join(harness.RealHomePath(), "events.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no real data directory at %s", harness.RealHomePath())
	}
	return events.Load(path)
}
