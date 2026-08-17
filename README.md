# waid — "what am I doing?"

A local open-loop tracker over an append-only event log. It answers three questions:

1. **What was each agent session about?** — derived automatically from Claude Code transcripts.
2. **What do I need to track?** — items I declare as I go.
3. **What open loops do I have?** — declared items plus detected signals from git and GitHub.

Single machine, local files, zero runtime dependencies.

## Install

```
git clone <this repo> C:\dev\waid
cd C:\dev\waid
go install ./cmd/waid
```

Requires Go >= 1.26. `go install` puts `waid` in `$GOBIN` (`%USERPROFILE%\go\bin` by default), which
has to be on `PATH`. `waid --version` confirms which binary is resolving.

### No build step to manage

The binary is the whole distribution. There is no dependency tree to install — `go.mod` requires
nothing — no config to generate, and nothing to watch or recompile while using it. Reinstalling
after a change is the same one command, and `go run ./cmd/waid <command>` works from the source tree
without installing at all.

`internal/harness/smoke_test.go` builds and spawns the binary as a real child process, so the
installed path stays proven.

## Commands

| Command | What it does |
| --- | --- |
| `waid sync [--full]` | Rebuild the derived session cache. Incremental unless `--full`. |
| `waid today [--date YYYY-MM-DD]` | Sessions and item activity for a day, grouped by project. |
| `waid week [--last]` | Rollup by project for this week, or the previous one. |
| `waid loops [-p <project>]` | Declared open items, then detected signals. The default view. |
| `waid scan [-p <project>]` | Detected signals only. |
| `waid list [--status s] [--project p] [--tag t] [--all]` | Declared items. Hides done unless `--all` or `--status done`. |
| `waid add "<title>" [-p <project>] [--waiting-on <who>] [--tag <t>] [--session <id>]` | Record an item. |
| `waid done <id>` / `waid reopen <id>` | Close or reopen an item. |
| `waid note <id> "<text>"` | Append a note to an item. |
| `waid show <id>` | Full item with its notes and event history. |
| `waid transcript <id>` | A session's turns, oldest first. |
| `waid promote <key>` | Turn a detected signal into a tracked item, and dismiss the signal. |
| `waid dismiss <key>` | Hide a detected signal without tracking it. |
| `waid undismiss <key>` | Restore a dismissed signal so detection reports it again. |
| `waid doctor` | Validate config, log integrity, gh auth, cache freshness, repo counts. |

### Global flags

| Flag | Effect |
| --- | --- |
| `--json` | Print a single JSON document to stdout. Degradations go to stderr. |
| `--no-sync` | Skip the implicit session sync. |
| `--waid-home <path>` | Override `$WAID_HOME` for one invocation. |
| `--version` | Print the version. |
| `-h`, `--help` | Show usage. |

Exit codes are `0` success, `1` user error (unknown id, unresolvable project, bad flag), `2`
internal error with a stack on stderr.

### Items

An item has a 4-character Crockford base32 id, a title, a status (`open`, `waiting`, `done`), an
optional project, tags, and an optional `waitingOn`. `--waiting-on` implies `waiting` status;
`reopen` clears it.

`-p` / `--project` accepts an absolute path, `.` for the current directory, or any substring that
matches exactly one known project. A substring matching none or several is a user error, and the
candidates are listed.

### Signals

`loops` and `scan` detect four kinds of open loop across the repos under `scanRoots`, gated to
those with activity in the last `activeWindowDays`:

| Kind | Key | Meaning |
| --- | --- | --- |
| `review` | `review:<owner/repo>#<n>` | A PR awaiting your review. |
| `pr` | `pr:<owner/repo>#<n>` | A PR you authored that is still open. |
| `ahead` | `ahead:<repo>:<branch>` | Unpushed commits. |
| `dirty` | `dirty:<repo>` | Uncommitted files. |

They rank by kind in that order, then oldest first. `promote`, `dismiss` and `undismiss` all take
the key. GitHub signals need `ghUser` set and an authenticated `gh`; without them the git signals
still work and a note explains the gap.

Each signal renders as three columns — the key, what it is about, and where and when:

```
DETECTED  (4 shown, 1 dismissed)

  review:DevResults/DevResults#6886  Add SharedDashboards role to non-owners  @Copilot · edit-shared-dashboards · 15w
  pr:DevResults/DevResults#7110      Migrate BudgetBreakdown chart to D3      migrate-aspx-charts · open · 5w
  ahead:C:\dev\waid:cli-ui           1 commit ahead                           cli-ui · 4m
  dirty:C:\dev\bkc-my                13 uncommitted files                     day-cards-p3-outliner · 2w
```

### Triaging signals

Triage is one command per decision: `waid promote <key>` to track the signal, `waid dismiss <key>`
to hide it, `waid undismiss <key>` to bring it back. `undismiss` only accepts a key that is
dismissed now; anything else is a user error, since the likeliest cause is a typo. The keys are
meant to be copied out of the `DETECTED` block verbatim.

The interactive picker (`-i`) that used to stage marks on this screen is gone. It is superseded by
the app's Scan tab, which serves the same job with the same keys and more context. Until that
lands, triage means copying a key into a second command.

### Sessions

Read commands refresh `cache/sessions.json` implicitly when it is missing or more than 5 minutes
old, so `sync` is rarely needed by hand. `--no-sync` suppresses it. A sync that fails is not fatal:
the command runs on the cached sessions and says so.

Sessions are harvested from Claude Code transcripts under `claudeDir` — title, project, prompt
count, and timestamps. Nothing is sent anywhere.

`waid transcript <id>` reads the turns back out of one session's transcript, oldest first. The id is
the 36-character session id `today` and `week` print, but any unambiguous prefix of it resolves; a
prefix matching several sessions is a user error and the candidates are listed. Turns come from the
transcript file on disk rather than the cache, so a session whose file has since moved or been
deleted is a user error too.

## `WAID_HOME`

The data directory, resolved as `--waid-home`, then `$WAID_HOME`, then `C:\data\waid`.

```
C:\data\waid
  config.json        settings; created on first run, never overwritten
  events.jsonl       the append-only log — the only durable state
  cache/             derived, disposable, gitignored
    sessions.json    harvested transcripts
    gh.json          cached gh search responses
  .gitignore         carries `cache/`
```

The home is itself a git repo, which is how the log gets history and backup. First run is
idempotent against an existing one: it creates only what is missing and never rewrites
`config.json`, `events.jsonl`, or existing `.gitignore` entries.

`events.jsonl` is the source of truth. Everything under `cache/` can be deleted and rebuilt.

### Config keys

`config.json` is seeded on first run and read on every invocation. Missing keys fall back to the
defaults below; unknown keys are reported by `waid doctor`.

| Key | Default | Meaning |
| --- | --- | --- |
| `scanRoots` | `["C:\\dev"]` | Directories walked for git repos. |
| `scanMaxDepth` | `4` | How deep to walk, counting the root as 0. |
| `claudeDir` | `~/.claude` | Where Claude Code transcripts live. |
| `activeWindowDays` | `30` | A repo is "active" if a session or commit falls inside this window. |
| `ghUser` | detected via `gh` | GitHub login. `null` turns GitHub signals off. |
| `ghCacheTtlMinutes` | `15` | How long `cache/gh.json` is served before refetching. |

Discovery skips `node_modules`, `bin`, `obj`, and `.git`, and never descends into a repo.

## Development

```
go test ./...
go vet ./...
```

Tests are the real thing wherever possible: `internal/git` drives temp git repos,
`internal/harness` builds the binary and runs it as a child process against fixture homes and
against a copy of the real `C:\data\waid`. Only `gh` and the clock are faked.
