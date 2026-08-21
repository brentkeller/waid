package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/brentkeller/waid/internal/cli"
	"github.com/brentkeller/waid/internal/config"
	"github.com/brentkeller/waid/internal/events"
	"github.com/brentkeller/waid/internal/gh"
	"github.com/brentkeller/waid/internal/render"
	"github.com/brentkeller/waid/internal/repos"
	"github.com/brentkeller/waid/internal/sessions"
	"github.com/brentkeller/waid/internal/tree"
)

// doctorLabelWidth is the width of the label column every doctor line starts with.
const doctorLabelWidth = 10

// DoctorConfig is what config.json looks like from the outside: where it is, whether it can be
// read, and which of its keys waid does not recognise.
type DoctorConfig struct {
	Path string `json:"path"`
	// Valid is false only when config.json is unreadable or not a JSON object.
	Valid bool `json:"valid"`
	// UnknownKeys are keys stored in config.json that waid does not recognise — usually typos.
	UnknownKeys []string `json:"unknownKeys"`
}

// DoctorLog is the event log's size and integrity.
type DoctorLog struct {
	Path     string           `json:"path"`
	Lines    int              `json:"lines"`
	Items    int              `json:"items"`
	Problems []events.Problem `json:"problems"`
}

// DoctorSessionsCache is the freshness of cache/sessions.json, read without rebuilding it.
type DoctorSessionsCache struct {
	Exists     bool    `json:"exists"`
	SyncedAt   *string `json:"syncedAt"`
	AgeMinutes *int    `json:"ageMinutes"`
}

// DoctorGhCache is the freshness of cache/gh.json.
type DoctorGhCache struct {
	// AgeMinutes is null when cache/gh.json is absent, unreadable, or carries an unusable timestamp.
	AgeMinutes *int `json:"ageMinutes"`
}

// DoctorCache is both derived caches.
type DoctorCache struct {
	Sessions DoctorSessionsCache `json:"sessions"`
	Gh       DoctorGhCache       `json:"gh"`
}

// DoctorGh is whether gh can be reached, and as whom.
type DoctorGh struct {
	Available bool `json:"available"`
	// User is the detected login, falling back to the configured one when gh cannot be reached.
	User *string `json:"user"`
}

// DoctorRepos is what detection has to work with: the repos found under the scan roots, and how
// many are live.
type DoctorRepos struct {
	Roots      []string `json:"roots"`
	Discovered int      `json:"discovered"`
	Active     int      `json:"active"`
}

// DoctorResult is the whole report. Fields are declared in the order Node emits them, which is the
// order --json renders.
type DoctorResult struct {
	Home   string       `json:"home"`
	Config DoctorConfig `json:"config"`
	Log    DoctorLog    `json:"log"`
	Cache  DoctorCache  `json:"cache"`
	Gh     DoctorGh     `json:"gh"`
	Repos  DoctorRepos  `json:"repos"`
	// Ok is false when something needs attention; the command still exits 0, since this is a report.
	Ok bool `json:"ok"`
}

func runDoctor(ctx *cli.Ctx) (DoctorResult, error) {
	cfg := ctx.Cfg
	inspected := inspectConfig(cfg.ConfigPath)
	state := events.Load(cfg.EventsPath)
	problems := slices.Concat(state.Problems, hierarchyProblems(state))
	detected := config.DetectGhUser()

	user := detected
	if user == nil {
		user = cfg.GhUser
	}

	return DoctorResult{
		Home:   cfg.Home,
		Config: inspected,
		Log: DoctorLog{
			Path:     cfg.EventsPath,
			Lines:    len(events.ReadLines(cfg.EventsPath)),
			Items:    len(state.Items),
			Problems: problems,
		},
		Cache: DoctorCache{
			Sessions: inspectSessionsCache(cfg.SessionsCachePath, ctx.Now),
			Gh:       DoctorGhCache{AgeMinutes: gh.CacheAgeMinutes(cfg, ctx.Now)},
		},
		Gh:    DoctorGh{Available: detected != nil, User: user},
		Repos: inspectRepos(ctx),
		Ok:    inspected.Valid && len(inspected.UnknownKeys) == 0 && len(problems) == 0,
	}, nil
}

// hierarchyProblems reports the items internal/tree had to lift to the top level. The fold is
// per-line and never sees the whole graph, so an unresolvable parent id and a looping ancestor
// chain can only be found once every item is in hand.
func hierarchyProblems(state events.State) []events.Problem {
	_, anomalies := tree.Build(state)

	problems := []events.Problem{}
	for _, anomaly := range anomalies {
		reason := events.ReasonUnknownParent
		if anomaly.Kind == tree.ParentCycle {
			reason = events.ReasonParentCycle
		}
		problems = append(problems, events.Problem{Reason: reason, Id: &anomaly.Id})
	}
	return problems
}

// inspectRepos counts what detection would see. The sessions are read from the cache rather than
// synced, since syncing here would make the freshness this same report prints meaningless.
func inspectRepos(ctx *cli.Ctx) DoctorRepos {
	discovered := repos.Discover(ctx.Cfg)
	cached := sessions.Load(ctx.Cfg).Sessions
	active := repos.Active(ctx.Cfg, cached, repos.ActiveOptions{
		Now:   ctx.Now,
		Git:   ctx.Git,
		Repos: discovered,
	})

	roots := ctx.Cfg.ScanRoots
	if roots == nil {
		roots = []string{}
	}
	return DoctorRepos{Roots: roots, Discovered: len(discovered), Active: len(active)}
}

// inspectConfig re-reads config.json for the keys Load discards, without ever writing to it.
func inspectConfig(configPath string) DoctorConfig {
	keys, ok := configKeys(configPath)
	if !ok {
		return DoctorConfig{Path: configPath, Valid: false, UnknownKeys: []string{}}
	}

	known := config.KnownKeys()
	unknown := []string{}
	for _, key := range keys {
		if !slices.Contains(known, key) {
			unknown = append(unknown, key)
		}
	}
	return DoctorConfig{Path: configPath, Valid: true, UnknownKeys: unknown}
}

// configKeys returns the top-level keys config.json carries, in the order the file holds them, and
// whether the file is a JSON object at all. Order is read from the document rather than a decoded
// map, so an unknown key is reported where the author will find it.
func configKeys(configPath string) ([]string, bool) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, false
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}

	keys := []string{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		key, isKey := token.(string)
		if !isKey {
			return nil, false
		}
		keys = append(keys, key)

		// The value is skipped whole, whatever shape it has; only the keys matter here.
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
	}

	if _, err := decoder.Token(); err != nil {
		return nil, false
	}
	// Anything after the object means the file is not one document, which JSON.parse rejects too.
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	return keys, true
}

// inspectSessionsCache reports the derived session cache without building it. A missing or
// unreadable file is simply "not built" rather than a problem.
func inspectSessionsCache(cachePath string, now time.Time) DoctorSessionsCache {
	raw, err := os.ReadFile(cachePath)
	if err != nil {
		return DoctorSessionsCache{}
	}

	var parsedFile any
	if err := json.Unmarshal(raw, &parsedFile); err != nil {
		return DoctorSessionsCache{}
	}

	envelope, _ := parsedFile.(map[string]any)
	syncedAt, isString := envelope["syncedAt"].(string)
	if !isString {
		return DoctorSessionsCache{Exists: true}
	}
	at, parsed := render.ParseTime(syncedAt)
	if !parsed {
		return DoctorSessionsCache{Exists: true, SyncedAt: &syncedAt}
	}

	minutes := max(0, int(now.Sub(at)/time.Minute))
	return DoctorSessionsCache{Exists: true, SyncedAt: &syncedAt, AgeMinutes: &minutes}
}

func renderDoctor(data DoctorResult, ctx *cli.Ctx) string {
	lines := []string{"DOCTOR", ""}

	lines = append(lines, doctorField("home", data.Home))
	configLine := data.Config.Path
	if !data.Config.Valid {
		configLine += "  UNREADABLE"
	}
	lines = append(lines, doctorField("config", configLine))
	if len(data.Config.UnknownKeys) > 0 {
		lines = append(lines, doctorField("", "unknown keys: "+strings.Join(data.Config.UnknownKeys, ", ")))
	}

	lines = append(lines, doctorField("log", data.Log.Path))
	lines = append(lines, doctorField("", fmt.Sprintf(
		"%s, %s, %s",
		plural(data.Log.Lines, "line"), plural(data.Log.Items, "item"), plural(len(data.Log.Problems), "problem"),
	)))
	lines = append(lines, doctorField("cache", sessionsSummary(data.Cache.Sessions, ctx.Now)))
	lines = append(lines, doctorField("", ghCacheSummary(data.Cache.Gh)))
	lines = append(lines, doctorField("gh", ghSummary(data.Gh)))
	lines = append(lines, doctorField("repos", fmt.Sprintf("%d discovered, %d active", data.Repos.Discovered, data.Repos.Active)))
	if len(data.Repos.Roots) > 0 {
		lines = append(lines, doctorField("", "roots: "+strings.Join(data.Repos.Roots, ", ")))
	}

	if len(data.Log.Problems) > 0 {
		lines = append(lines, "", "PROBLEMS", "")
		for _, problem := range data.Log.Problems {
			lines = append(lines, "  "+problemLine(problem))
		}
	}

	verdict := "ok"
	if !data.Ok {
		verdict = "needs attention"
	}
	return strings.Join(append(lines, "", verdict), "\n")
}

func doctorField(label, value string) string {
	return "  " + render.Pad(label, doctorLabelWidth) + value
}

func sessionsSummary(cache DoctorSessionsCache, now time.Time) string {
	if !cache.Exists {
		return "sessions not built"
	}
	if cache.SyncedAt == nil {
		return "sessions built, sync time unknown"
	}
	return "sessions synced " + render.RelTime(*cache.SyncedAt, now)
}

func ghCacheSummary(cache DoctorGhCache) string {
	if cache.AgeMinutes == nil {
		return "gh not fetched"
	}
	return fmt.Sprintf("gh fetched %dm ago", *cache.AgeMinutes)
}

func ghSummary(state DoctorGh) string {
	if state.Available {
		return "available as " + derefOr(state.User, "")
	}
	if state.User == nil {
		return "unavailable"
	}
	return "unavailable (configured as " + *state.User + ")"
}

func problemLine(problem events.Problem) string {
	detail := []string{}
	if problem.Ev != nil {
		detail = append(detail, *problem.Ev)
	}
	if problem.Id != nil {
		detail = append(detail, "id="+*problem.Id)
	}

	suffix := ""
	if len(detail) > 0 {
		suffix = "  " + strings.Join(detail, "  ")
	}

	// A problem found across the whole log rather than on one line carries no line number.
	location := ""
	if problem.Line > 0 {
		location = fmt.Sprintf("line %d", problem.Line)
	}
	return render.Pad(location, doctorLabelWidth) + string(problem.Reason) + suffix
}

func derefOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}
