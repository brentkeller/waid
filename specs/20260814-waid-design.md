# waid — "What Am I Doing?" — Design

**Date:** 2026-08-14
**Status:** Approved design, pending implementation plan

## Purpose

A local tool for tracking what I'm doing across agent sessions, projects, and tasks on this
machine. It answers three questions:

1. **What was each agent session about?** — derived automatically from Claude Code transcripts.
2. **What do I need to track?** — tasks, bugs, and deferred work I declare as I go.
3. **What open loops do I have?** — declared items plus detected signals from git and GitHub.

It also produces daily and weekly summaries of work done.

## Non-goals

- Not a project management tool. No sprints, assignees, estimates, or burndown.
- Not a sync service. Single machine, local files. Sharing is a git concern, not waid's.
- Not a replacement for `.ralph/todo.md` or in-repo TODOs. waid tracks *my* loops, not a
  repo's backlog.

## Context: what already exists on this machine

Measured on 2026-08-14, and the numbers drove several decisions:

| Fact | Value | Consequence |
|---|---|---|
| Claude Code transcripts | 468 `.jsonl` across 17 project dirs | Harvest is worth automating; must be incremental. |
| Projects with `sessions-index.json` | 5 of 17 | **The index is unreliable.** Harvest reads transcripts directly. |
| Largest transcript | 2.5 MB | Stream line-by-line; never load whole files eagerly. |
| Git repos under `C:\dev` | 122 | Naive detection is unusable. |
| Repos with dirty worktrees | 67 | Detection must be gated by recent activity. |
| Local branches, all repos | 297 | Same. |
| Repos with commits in last 30 days | 10 | The activity gate reduces ~360 raw signals to a usable handful. |
| `gh` CLI | authed as `brentkeller` | GitHub signals are available without extra setup. |
| Node | v24.11.0 | Target runtime; stdlib only. |

### Harvestable fields in a transcript

Every session `.jsonl` contains:

- An **`ai-title`** record — `{"type":"ai-title","aiTitle":"Fix budget chart legend overflow issue","sessionId":"..."}`.
  Emitted repeatedly as the title is refined; the **last one wins**.
- **User message records** carrying `timestamp`, `cwd`, `gitBranch`, `sessionId`, `isSidechain`,
  and `isMeta`.

That is everything the session layer needs. `sessions-index.json`, where present, may be used as
a shortcut but is never required and is never trusted over the transcript.

## 1. Storage and configuration

Root directory comes from the `WAID_HOME` environment variable, defaulting to `~/.waid`.
Whether that directory is a git repo is the user's choice; waid neither requires nor creates one.

```
$WAID_HOME/
  events.jsonl          # authored — the source of truth, append-only
  config.json           # settings
  cache/
    sessions.json       # derived from transcripts, disposable
    gh.json             # short-TTL GitHub response cache
  .gitignore            # ignores cache/
```

`waid` creates the directory and a default `config.json` on first run.

### config.json

```jsonc
{
  "scanRoots": ["C:\\dev"],           // where to look for git repos
  "claudeDir": "C:\\Users\\brent\\.claude",
  "activeWindowDays": 30,             // recency gate for detection
  "ghUser": "brentkeller",            // for "PRs I authored" / "awaiting my review"
  "ghCacheTtlMinutes": 15,
  "scanMaxDepth": 4                   // repo discovery depth under each scan root
}
```

Missing keys fall back to these defaults. `ghUser` defaults to the output of `gh api user -q .login`
on first run; if `gh` is unavailable it is left null and GitHub signals are skipped.

## 2. Data model — items as a fold over an event log

There is **one authored record type: the item.** Tasks, bugs, and open loops are the same
thing viewed through different filters. A bug noticed while testing is an item with
`status: "open"`; it appears in `waid loops` until closed.

### Why an append-only log

Multiple Claude Code sessions run in parallel on this machine. Two agents doing read-modify-write
against a mutable JSON file will clobber each other's changes. Two agents **appending lines** to
a JSONL file will not — a single `appendFileSync` of a sub-4KB line is atomic enough in practice
on Windows, and no reader is blocked. The log also yields free history: when a loop opened, when
it moved to `waiting`, when it closed.

### Event schema

Each line of `events.jsonl` is one JSON object. Common fields: `ts` (ISO 8601 UTC), `ev`, `id`.

| `ev` | Additional fields | Meaning |
|---|---|---|
| `add` | `title`, `status`, `project?`, `session?`, `tags?`, `waitingOn?` | Create an item. |
| `update` | any of `title`, `status`, `project`, `tags`, `waitingOn` | Patch an item. Only supplied fields change. |
| `note` | `text` | Append a timestamped note. |
| `close` | — | Shorthand for `update` with `status: "done"`. |
| `reopen` | — | Shorthand for `update` with `status: "open"`; clears `waitingOn`. |
| `dismiss` | `key` (instead of `id`) | Permanently hide a detected signal. |
| `undismiss` | `key` | Un-hide it. |

```jsonl
{"ts":"2026-08-14T18:22:01.004Z","ev":"add","id":"k3f9","title":"Chart legend overflows at 4+ series","project":"C:\\dev\\dr\\devresults","status":"open","session":"34782fc3-e623-4a17-940a-7d90a1b58f6b","tags":["bug"]}
{"ts":"2026-08-14T19:02:55.881Z","ev":"note","id":"k3f9","text":"repro only in Firefox"}
{"ts":"2026-08-15T14:10:02.113Z","ev":"update","id":"k3f9","status":"waiting","waitingOn":"design review"}
{"ts":"2026-08-19T09:31:44.207Z","ev":"close","id":"k3f9"}
```

### Item shape after folding

```jsonc
{
  "id": "k3f9",
  "title": "Chart legend overflows at 4+ series",
  "status": "open",                 // open | waiting | done
  "waitingOn": null,                // free text, only meaningful when status is waiting
  "project": "C:\\dev\\dr\\devresults",  // absolute path, or null
  "session": "34782fc3-…",          // session that created it, or null
  "tags": ["bug"],
  "notes": [{"ts":"…","text":"repro only in Firefox"}],
  "created": "2026-08-14T18:22:01.004Z",
  "updated": "2026-08-19T09:31:44.207Z"
}
```

### Fold rules

- Process events in file order. File order is authoritative; `ts` is display metadata and is
  **not** used for ordering, since parallel agents can produce out-of-order timestamps.
- An `update`/`note`/`close`/`reopen` for an unknown `id` is skipped, and the line number is
  reported by `waid doctor`. It never aborts the fold.
- `created` is the `ts` of the `add`; `updated` is the `ts` of the most recent event.
- Unknown `ev` values and unparseable lines are skipped and counted, never fatal.

### IDs

Four characters from Crockford base32 (`0-9a-hjkmnp-tv-z`, excluding `i`, `l`, `o`, `u`),
generated with `crypto.randomInt`. That is ~1M combinations, which for a few thousand lifetime
items keeps collisions negligible. On `add`, waid folds the log first and retries generation if
the id already exists.

### Deliberately deferred

Priorities, due dates, sub-items, and item-to-item links. If the item list becomes a dumping
ground in practice, add filtering first; revisit structure only if filtering proves insufficient.

## 3. Session harvest

`waid sync` builds `cache/sessions.json` from Claude Code transcripts.

**Discovery.** Glob `$claudeDir/projects/*/*.jsonl`. Each file is one session, named by session id.

**Extraction.** Stream each file line by line, parsing each line as JSON and ignoring failures.
Collect:

- `title` — `aiTitle` of the **last** `ai-title` record. If none, the first 80 characters of the
  first non-meta user message. If that is also missing, `"(untitled)"`.
- `project` — `cwd` of the first non-sidechain user record, falling back to decoding the parent
  directory name. Note that the directory-name slug is lossy (`C--dev-dr-devresults` cannot be
  reliably reversed into `C:\dev\dr\devresults` vs `C:\dev\dr-devresults`), which is exactly why
  `cwd` is preferred and the slug is a last resort.
- `branch` — `gitBranch` of the last user record that has one.
- `started` / `ended` — first and last `timestamp` across all records.
- `prompts` — count of user records where `isSidechain` is falsy **and** `isMeta` is falsy.
  Sidechain records are excluded from the count but still contribute to `started`/`ended`, so a
  session's real span stays accurate.

**Incrementality.** `cache/sessions.json` stores, per file, `{path, mtimeMs, size}` alongside the
extracted record. On sync, a file whose `mtimeMs` and `size` both match the cache is not re-read.
`--full` forces a complete rebuild. Deleting `cache/` is always safe.

**Cache shape.**

```jsonc
{
  "version": 1,
  "syncedAt": "2026-08-14T18:30:00.000Z",
  "sessions": [
    {
      "id": "34782fc3-e623-4a17-940a-7d90a1b58f6b",
      "title": "Fix budget chart legend overflow issue",
      "project": "C:\\dev\\dr\\devresults\\devresults",
      "branch": "migrate-aspx-charts",
      "started": "2026-07-21T19:45:17.299Z",
      "ended": "2026-07-21T21:03:44.812Z",
      "prompts": 38,
      "_file": {"path": "…", "mtimeMs": 1786729993265, "size": 2583041}
    }
  ]
}
```

Sessions with zero counted prompts are retained in the cache but hidden from `today`/`week`
output, so trivial `/clear`-and-exit sessions do not pad the summaries.

**Implicit sync.** Read commands (`today`, `week`, `loops`, `scan`) run a sync automatically if
`cache/sessions.json` is older than 5 minutes. `--no-sync` skips it.

## 4. Detection

Detected signals are **evidence, not loops.** They render in a separate section beneath declared
items and never merge into them silently. This is the difference between a list you trust and a
list you learn to ignore.

### The activity gate

A repo is scanned only if it is *active*: a Claude Code session (from the harvest) has a `project`
path inside it within `activeWindowDays`, **or** its `HEAD` commit is within `activeWindowDays`.
On this machine that reduces 122 repos to roughly 10–17, and detection stays fast enough to run
inline on every `waid loops`.

Repo discovery walks each `scanRoots` entry to `scanMaxDepth`, looking for `.git`, and skips
`node_modules`, `bin`, `obj`, and `.git` internals.

### Signals

| Signal | Key format | Source | Detail shown |
|---|---|---|---|
| PR awaiting my review | `review:owner/repo#123` | `gh search prs --review-requested=@me --state=open` | title, author, age |
| PR I authored, still open | `pr:owner/repo#123` | `gh search prs --author=@me --state=open` | title, review state, age |
| Branch ahead of remote | `ahead:<repo>:<branch>` | `git rev-list --count @{u}..HEAD` | branch, commits ahead |
| Uncommitted work | `dirty:<repo>` | `git status --porcelain` | file count, branch |

Ranked in that order — a colleague blocked on my review outranks my own uncommitted scratch work.

### GitHub calls

Both `gh` queries are account-wide, not per-repo, so detection costs two calls total regardless of
repo count. Responses are cached in `cache/gh.json` for `ghCacheTtlMinutes`. If `gh` is missing,
unauthenticated, or errors, GitHub signals are omitted and a single dim line notes it — git-based
signals still work offline.

### Dismissal and promotion

- `waid dismiss <key>` appends a `dismiss` event. Dismissed keys never reappear.
- `waid promote <key>` appends an `add` event seeded from the signal — title from the PR title or
  a generated description, `project` from the repo path, `tags: ["promoted"]` — then appends a
  `dismiss` for the key so it does not show up twice.

Since keys embed PR numbers and branch names, a dismissal is naturally scoped: dismissing
`pr:owner/repo#123` says nothing about `#124`.

## 5. CLI surface

Installed globally by `npm link` from `C:\dev\waid`, exposing `waid`. Node stdlib only — no
runtime dependencies.

```
waid sync [--full]                  Rebuild the derived session cache
waid today [--date YYYY-MM-DD]      Sessions + item activity for a day
waid week [--last]                  Rollup by project for this week (or last)
waid loops [-p <project>]           Declared open/waiting items, then detected signals
waid scan [-p <project>]            Detected signals only
waid list [--status s] [--project p] [--tag t] [--all]
waid add "<title>" [-p <project>] [--waiting-on <who>] [--tag <t>] [--session <id>]
waid done <id>                      waid reopen <id>
waid note <id> "<text>"
waid show <id>                      Full item with notes and history
waid promote <key>                  waid dismiss <key>
waid doctor                         Validate config, log integrity, gh auth, cache freshness
```

**Global flags:** `--json` on every command, `--no-sync`, `--waid-home <path>` (overrides the env
var, mainly for tests).

**Conventions.**
- `-p` accepts an absolute path, a partial path, or `.` for the current directory. A partial that
  matches more than one known project is an error listing the candidates.
- Human output is the default and is designed to be skimmed: grouped, aligned, one item per line.
- `--json` prints a single JSON document to stdout with nothing else, for agents.
- Exit codes: `0` success, `1` user error (unknown id, ambiguous project), `2` internal error.
- Writes go to `events.jsonl` only. Nothing else on the machine is modified.

**Sample `waid loops` output**

```
OPEN LOOPS

  C:\dev\dr\devresults
    k3f9  open      Chart legend overflows at 4+ series          [bug]     5d
    m7qz  waiting   Approve report-template copy                 ← Dan     2d

  C:\dev\waid
    p2vn  open      Decide whether events.jsonl gets its own repo          1d

DETECTED  (3 shown, 2 dismissed)

  review:DevResults/DevResults#8812   Awaiting your review · @tmoore · 3d
  pr:DevResults/DevResults#8790       Yours, changes requested · 6d
  dirty:C:\dev\dr\inl-prt             14 uncommitted files on inl-prt-fixes

  waid promote <key> to track · waid dismiss <key> to hide
```

## 6. Agent integration

Three pieces, in the order they should be added.

### 6.1 Global CLAUDE.md snippet

Kept to roughly five lines, since `~/.claude/CLAUDE.md` loads in every session on this machine:

```markdown
## Tracking work (waid)
When I report a bug, defer work, or get blocked on someone else, record it:
`waid add "<title>" -p <project path> [--waiting-on <who>] [--tag bug] --json`
When something is resolved, run `waid done <id> --json`. Always pass `--json`.
Don't record routine steps you complete within the session — only work that outlives it.
```

The last line is the important one. Without it the log fills with in-session noise and stops being
a list of open loops.

### 6.2 `/waid` skill

At `~/.claude/skills/waid/SKILL.md`, alongside the twelve existing personal skills. Covers the
flows that need more judgment than a one-line CLAUDE.md rule:

- **Wrap up** — review this session and propose items for anything left unfinished, for approval
  before writing.
- **Triage** — walk detected signals, promoting or dismissing each.
- **Review** — read back the week and propose closures for items evidently finished.

### 6.3 SessionStart hook (stage 5)

A hook in `settings.json` running `waid loops -p <cwd> --json` and injecting the result as
`additionalContext`, so every new session opens knowing what is outstanding in that project. It
must fail silently and fast — a broken or slow waid must never block a session from starting.

## 7. Testing

`node:test` with `node --test`. No test dependencies.

- **Pure functions, unit-tested against fixtures:** the event fold (including malformed lines,
  unknown ids, unknown `ev` values), the transcript parser (missing `ai-title`, sidechain-only
  sessions, truncated final line, CRLF), signal ranking, and project-path matching.
- **Integration tests:** each CLI command against a temp `WAID_HOME`, asserting on `--json` output.
- **Fixtures:** small hand-written transcript `.jsonl` files committed under `test/fixtures/`,
  including one truncated mid-line to prove the parser survives a session being written *while*
  waid reads it.
- **Not tested:** live `gh` calls. The GitHub client is a seam that tests inject a fake for.

## 8. Implementation stages

Each stage is independently useful and independently shippable.

| Stage | Contents | Done when |
|---|---|---|
| 1 | Store, event log, fold, ids, config, `add`/`done`/`reopen`/`note`/`show`/`list`/`loops` (declared only), `doctor` | I can capture and close items from the CLI |
| 2 | Transcript harvest, incremental cache, `sync`/`today`/`week` | `waid week` summarizes my real work with no input from me |
| 3 | Repo discovery, activity gate, four signals, `scan`/`promote`/`dismiss`, detected section in `loops` | `waid loops` surfaces real loops I had forgotten |
| 4 | CLAUDE.md snippet, `/waid` skill | Agents record items without me asking |
| 5 (later) | SessionStart hook, nightly digest agent | Loops appear in context automatically; digests write themselves |

Stage 5 is explicitly a nice-to-have follow-up, not part of the initial build.

## 9. Open questions

1. **Does `$WAID_HOME` become its own git repo?** Leaning yes, for history and backup, but the
   decision can wait until stage 1 is in use. Nothing in the design depends on it.
2. **Does `events.jsonl` ever need compaction?** Not at the projected volume. Revisit only if fold
   time becomes noticeable, and only then via a `waid compact` that rewrites closed items older
   than N months into an archive file.
