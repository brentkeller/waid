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
npm install        # devDependencies only — typescript and @types/node
npm link           # exposes `waid` on PATH
```

Requires Node >= 24.

### No build step

The sources run as written. `bin/waid.ts` is the package's `bin` entry and Node strips the types
at load time — there is no compile, no `dist/`, and no watch process.

This survives `npm link` only because of how Node resolves symlinks. Node refuses to strip types
for files that genuinely live under `node_modules`, but `npm link` symlinks the package in, and
realpath resolution puts the entry back at `C:\dev\waid` before the check runs. If you ever see
`ERR_UNSUPPORTED_NODE_MODULES_TYPE_STRIPPING`, the package was copied into `node_modules` rather
than linked.

`test/smoke.test.ts` spawns `bin/waid.ts` as a real child process, so this stays proven.

Without linking, every command below works as `node C:\dev\waid\bin\waid.ts <command>`.

## Commands

| Command | What it does |
| --- | --- |
| `waid sync [--full]` | Rebuild the derived session cache. Incremental unless `--full`. |
| `waid today [--date YYYY-MM-DD]` | Sessions and item activity for a day, grouped by project. |
| `waid week [--last]` | Rollup by project for this week, or the previous one. |
| `waid loops [-p <project>] [-i]` | Declared open items, then detected signals. The default view. |
| `waid scan [-p <project>] [-i]` | Detected signals only. |
| `waid list [--status s] [--project p] [--tag t] [--all]` | Declared items. Hides done unless `--all` or `--status done`. |
| `waid add "<title>" [-p <project>] [--waiting-on <who>] [--tag <t>] [--session <id>]` | Record an item. |
| `waid done <id>` / `waid reopen <id>` | Close or reopen an item. |
| `waid note <id> "<text>"` | Append a note to an item. |
| `waid show <id>` | Full item with its notes and event history. |
| `waid promote <key>` | Turn a detected signal into a tracked item, and dismiss the signal. |
| `waid dismiss <key>` | Hide a detected signal without tracking it. |
| `waid doctor` | Validate config, log integrity, gh auth, cache freshness, repo counts. |

### Global flags

| Flag | Effect |
| --- | --- |
| `-i`, `--interactive` | Triage the rows in place. `loops` and `scan` only. |
| `--json` | Print a single JSON document to stdout. Degradations go to stderr. |
| `--no-sync` | Skip the implicit session sync. |
| `--waid-home <path>` | Override `$WAID_HOME` for one invocation. |
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

They rank by kind in that order, then oldest first. `promote` and `dismiss` both take the key.
GitHub signals need `ghUser` set and an authenticated `gh`; without them the git signals still
work and a note explains the gap.

Each signal renders as three columns — the key, what it is about, and where and when:

```
DETECTED  (4 shown, 1 dismissed)

  review:DevResults/DevResults#6886  Add SharedDashboards role to non-owners  @Copilot · edit-shared-dashboards · 15w
  pr:DevResults/DevResults#7110      Migrate BudgetBreakdown chart to D3      migrate-aspx-charts · open · 5w
  ahead:C:\dev\waid:cli-ui           1 commit ahead                           cli-ui · 4m
  dirty:C:\dev\bkc-my                13 uncommitted files                     day-cards-p3-outliner · 2w
```

### Interactive triage (`-i`)

`waid loops -i` and `waid scan -i` open the same screen the command would have printed, with a
cursor on it. Marking is separate from applying: keys stage a decision in the gutter, and **nothing
is written until `Enter`**. `q`, `Esc` and end-of-input cancel the whole batch and write nothing,
which is a success — declining to triage is a valid outcome, not a failure.

| Key | Effect |
| --- | --- |
| `j` `k` `↑` `↓` | Move the cursor. Headings are skipped. |
| `g` / `G` | First / last row. |
| `p` | Promote a detected signal. |
| `d` | Dismiss a detected signal. |
| `x` | Mark a declared item done. |
| `w` | Mark a declared item waiting on someone, opening the editor for the name. |
| `e` | Edit the mark's text — a promote title, or who an item waits on. |
| `u` | Unmark the row. Pressing the same action key twice does the same. |
| `Enter` | Apply every mark and exit. |
| `q`, `Esc` | Cancel; write nothing. |
| `Ctrl-C` | Cancel; exit `130`. |
| `?` | Toggle the full key table. |

`p` and `d` only apply to detected signals, `x` and `w` only to declared items; the wrong key on the
wrong row stages nothing and says why in the footer. While the editor is open every key belongs to
it, so `Enter` saves the field rather than applying the batch, and `Esc` reverts it.

Applying writes the same events the individual commands write, in row order, and prints one receipt
line per mark once the terminal is restored — so the trace lands in the real scrollback. **Apply
does not re-detect.** Every decision is applied against the rows as they were shown: a signal that
disappeared while you were deciding still promotes, rather than failing on a race.

`-i` is a user error, exit `1`, in three cases, all checked before any detection runs: combined with
`--json`, on a command that has no interactive screen (`list does not support -i`), or when stdin or
stdout is not a terminal. A screen with nothing selectable does not open the picker — it prints
exactly what the plain command prints and exits `0`.

### Sessions

Read commands refresh `cache/sessions.json` implicitly when it is missing or more than 5 minutes
old, so `sync` is rarely needed by hand. `--no-sync` suppresses it. A sync that fails is not fatal:
the command runs on the cached sessions and says so.

Sessions are harvested from Claude Code transcripts under `claudeDir` — title, project, prompt
count, and timestamps. Nothing is sent anywhere.

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
npm test           # node --test
npm run typecheck  # tsc --noEmit
```

Tests are the real thing wherever possible: `git.test.ts` drives temp repos, `smoke.test.ts`
spawns the binary. Only `gh` and the clock are faked.
