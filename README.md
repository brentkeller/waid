# waid — "what am I doing?"

A local open-loop tracker over an append-only event log. It answers three questions:

1. **What was each agent session about?** — derived automatically from Claude Code transcripts.
2. **What do I need to track?** — items I declare as I go.
3. **What open loops do I have?** — declared items plus detected signals from git and GitHub.

Single machine, local files, one binary. `waid ui` puts all three on one screen.

## Install

```
git clone <this repo> C:\dev\waid
cd C:\dev\waid
go install ./cmd/waid
```

Requires Go >= 1.26. `go install` puts `waid` in `$GOBIN` (`%USERPROFILE%\go\bin` by default), which
has to be on `PATH`. `waid --version` confirms which binary is resolving.

### No build step to manage

The binary is the whole distribution. There is nothing to install alongside it — `go install`
vendors what `go.mod` requires, which is Bubble Tea and Lipgloss for the app and nothing else — no
config to generate, and nothing to watch or recompile while using it. Reinstalling
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
| `waid loops [-p <parent>] [--origin <frag>] [--headings]` | Declared open items as a tree, then detected signals. The default view. Hides empty headings unless `--headings`. |
| `waid scan [-p <project>]` | Detected signals only. |
| `waid list [--status s] [--origin <frag>] [--tag t] [--all] [--headings]` | Declared items as a tree. Hides done unless `--all` or `--status done`, and empty headings unless `--headings`. |
| `waid add "<title>" [-p <parent>] [--waiting-on <who>] [--tag <t>] [--session <id>] [--heading]` | Record an item, under a parent or with an origin. `--heading` creates it as a heading. |
| `waid done <id>...` / `waid reopen <id>` | Close one or more items, or reopen one. |
| `waid move <id> -p <parent>` / `waid move <id> --top` | Refile an item under another, or move it to the top level. |
| `waid heading <id>` / `waid heading <id> --off` | Mark an item a heading — a landmark that heads a level whether or not anything is filed under it — or unmark it. |
| `waid note <id> "<text>"` | Append a note to an item. |
| `waid show <id>` | Full item with its notes and event history. |
| `waid transcript <id>` | A session's turns, oldest first. |
| `waid promote <key>` | Turn a detected signal into a tracked item, and dismiss the signal. |
| `waid dismiss <key>` | Hide a detected signal without tracking it. |
| `waid undismiss <key>` | Restore a dismissed signal so detection reports it again. |
| `waid doctor` | Validate config, log integrity, gh auth, cache freshness, repo counts. |
| `waid ui` | The terminal app: loops, repos and agents in one screen. |

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
optional parent, an optional origin path, tags, and an optional `waitingOn`. `--waiting-on` implies
`waiting` status; `reopen` clears it.

Items form a tree: any item can be the parent of another, so a heading is an item like any other and
`list` and `loops` print the nesting. `waid move` reparents one, rejecting itself and anything
already beneath it. An item refuses to close while an item under it is still open, which is why
`waid done` takes several ids — the children and their heading close in one invocation.

`-p` names a **parent**: any case-insensitive substring matching exactly one item's title. A
substring matching none or several is a user error, and the candidates are listed. An absolute path,
or `.` for the current directory, is recorded as the item's `origin` instead and leaves it at the top
level — where the work came from rather than where it is filed, which is what `promote` has always
recorded and what keeps `waid add "<title>" -p <project path>` working unchanged. `move` takes a
parent only; a path there is a user error rather than a silent no-op.

`--origin <fragment>` filters `list` and `loops` by that recorded path, and resolves the same way
project paths always have. `-p` on `scan` still names a repo, since detection is keyed by path.

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

The interactive picker (`-i`) that used to stage marks on this screen is gone, superseded by the
app's [Repos tab](#the-app-waid-ui), which triages with the same two keys and more context.

### Sessions

Read commands refresh `cache/sessions.json` implicitly when it is missing or more than 5 minutes
old, so `sync` is rarely needed by hand. `--no-sync` suppresses it. A sync that fails is not fatal:
the command runs on the cached sessions and says so. The app syncs on the same terms: incrementally
when it opens and after a resume, and fully on `r`.

Sessions are harvested from Claude Code transcripts under `claudeDir` — title, project, prompt
count, and timestamps. Nothing is sent anywhere.

`waid transcript <id>` reads the turns back out of one session's transcript, oldest first. The id is
the 36-character session id `today` and `week` print, but any unambiguous prefix of it resolves; a
prefix matching several sessions is a user error and the candidates are listed. Turns come from the
transcript file on disk rather than the cache, so a session whose file has since moved or been
deleted is a user error too.

## The app (`waid ui`)

The same data the commands print, in a long-running screen that folds, filters, and writes as you
go. It runs on the alternate screen and needs a real terminal: with stdout redirected it is a user
error rather than a fallback to text. The window title follows the live tab — `waid · Loops`,
`waid · Repos`, `waid · Agents` — so the app is picked out of a row of terminal tabs by what it
is showing.

```
╭───────────╮
│ Loops  12 │ Repos  8 │ Agents │                                    ⟳ scanning  5/9 repos
┴───────────┴──────────┴────────┴─────────────────────────────────────────────────────────
```

Three tabs, switched with `1` `2` `3` or `tab` / `shift-tab`:

| Tab | Shows | Acts |
| --- | --- | --- |
| Loops | Declared items as a tree, with a detail pane carrying `show`'s notes and history. | `x` done, `w` waiting, `e` retitle, `m` move, `n` note, `t` edit the tags, `y` copy the item id, `H` mark a heading, `h` reveal the headings holding nothing, `space` toggle the pane. |
| Repos | Detected signals as a project tree. | `p` promote, `d` dismiss, `o` open in browser, `e` rename what `p` just created. |
| Agents | Sessions by project for a day or a week, with a transcript preview. | `space` preview, `pgup`/`pgdn` scroll it, `home`/`end` jump to either end, `R` resume in Claude, `o` open the repo, `y` copy the session id, `d` pick a calendar day. |

`m` moves an item by navigation rather than by typing a title. It lifts the item and everything
under it out of the tree, leaving the tree as it will be after the move: `↑` `↓` walk it, `←` / `→`
fold and unfold, `enter` drops the item on the row under the cursor and `esc` abandons the move. The
picker opens on the item's current parent, with a `── top level ──` row above the roots for an item
that belongs under nothing. It opens with the roots and the rows filed directly under them on screen
and anything deeper folded away. Since the item's own subtree has left the tree, there is no
destination on screen the move could be refused for.

Keys that work everywhere: `↑` / `↓` to move, `g` / `G` for first and last, `enter`
to fold and `←` / `→` to collapse and expand the fold under the cursor, `/` to
filter (`esc` clears), `s` to cycle the tab's segmented row — Loops' statuses, Repos' kinds,
Agents' today/yesterday/week/last week plus whatever day `d` picked — `a` to add an item beside the
row under the cursor and `A` to add one under it, which is how a tier of the tree is made, `r` to
refresh — on Agents that re-reads every transcript on disk, so a session run since the app opened
shows up — `u` to undo, `?` for the key table, `q` or `ctrl-c` to quit. A key that belongs to
another tab says so in the footer rather than doing nothing quietly.

A prompt taking text — a title, a note or a set of tags — commits on `enter` and abandons on `esc`.
`e` and `t` open on what they are replacing, so a correction is a word of typing. A prompt holds the
keyboard while it is open, so its editing keys are the only ones bound: `←` / `→` move a character
and `ctrl-←` / `ctrl-→` — or `alt-←` / `alt-→`, or `alt-b` / `alt-f` — move a word, `ctrl-a` and
`ctrl-e` — or `home` and `end` — go to either end, `backspace` and `delete` take the character
behind and in front of the cursor, `ctrl-w` cuts the word behind it, `ctrl-k` cuts to the end and
`ctrl-u` cuts back to the start, which is how one starts from nothing.

Writes land on the keypress with no confirm step, so the footer's receipt is what says a press did
anything, and `u` reverses the last one — as another event, since the log is append-only. The stack
reaches 20 writes back; a note has no inverse and stops the offer. On quit the receipts are replayed
to the restored terminal, so a triage pass leaves a record in scrollback:

```
promoted 7k3m  Activity Compendium prototype
dismissed dirty:C:\dev\bkc-my
done     sga9  Design template + args persistence
```

Reads run off the update loop, so nothing blocks on git, `gh`, or the filesystem. A detection pass
that partly fails degrades the way `waid scan` does: the list keeps what was detected, the notes
explaining the gap sit below it, and a failed refresh shows in the tab bar beside the age of the
data still on screen.

`R` runs `claude --resume <id>` in the session's own directory, suspending the app for the length of
it. `y` copies through `clip`, `pbcopy`, or `wl-copy`/`xclip`/`xsel`, depending on the platform.

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

## Agent integration

`agent/` holds what agents read, and neither file is used from this repo — both are vendored out
to `~/.claude` and consumed there.

| Path | Consumed as | Vendored by |
| --- | --- | --- |
| `agent/skills/waid/SKILL.md` | `/waid` — wrap up, triage, review the week | the `skills` CLI |
| `agent/CLAUDE.snippet.md` | `~/.claude/WAID.md`, imported by `CLAUDE.md` | a copy |

Both are pulled from a pushed commit rather than a checkout, so a change to either has to land
here before it reaches a session. `brentkeller/dotfiles` is what pulls them, with `just waid`,
and installs them with `just install`.

`CLAUDE.snippet.md` loads in every session on the machine, so it stays around five lines. The
last of them — record only what outlives the session — is what keeps the log a list of open loops
instead of a transcript.

## Development

```
go test ./...
go vet ./...
```

Tests are the real thing wherever possible: `internal/git` drives temp git repos,
`internal/harness` builds the binary and runs it as a child process against fixture homes and
against a copy of the real `C:\data\waid`, and `internal/tui` drives the program through `teatest`
with golden frames at 80 and 140 columns. Only `gh` and the clock are faked.
