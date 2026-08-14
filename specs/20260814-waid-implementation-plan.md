# waid Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> This plan deliberately specifies **what each task must do and prove, not how**. It names files, exported signatures, and the exact test cases to write — implementations are the executor's job.

**Goal:** Build `waid`, a local Node CLI that tracks declared work items in an append-only event log, harvests Claude Code session transcripts, and surfaces detected git/GitHub open loops.

**Architecture:** One authored record type (the item), folded from `events.jsonl`. A disposable session cache derived from transcripts. Detection assembled inline from `git` and `gh` behind injectable seams. Every command module exports `run(ctx)` returning plain data and `render(data, ctx)` returning text, so `--json` and human output can never diverge.

**Tech Stack:** TypeScript run directly by Node v24.11.0's native type stripping — no build step, no `dist/`. ESM, Node stdlib only at runtime, `node:test` + `node:assert/strict`, `tsc --noEmit` as a separate typecheck gate.

**Spec:** `specs/20260814-waid-design.md`

## Global Constraints

- **Runtime:** Node `>=24`. Sources are `.ts` and execute as written; there is no compile output.
- **Dependencies:** `typescript` is the only entry in `devDependencies` and exists solely for `tsc --noEmit`. Zero runtime dependencies, zero test dependencies.
- **Module system:** ESM throughout, `"type": "module"`.
- **Erasable syntax only.** No `enum`, `namespace`, parameter properties, or decorators — Node throws `ERR_UNSUPPORTED_TYPESCRIPT_SYNTAX`. Use `as const` objects instead of enums.
- **Import specifiers carry the real extension** — `from './config.ts'`, never `./config` or `./config.js`.
- **Type-only imports use `import type`** — stripping is per-file and cannot infer erasability.
- **Test command is bare `node --test`.** Passing an explicit directory does not pick up `.ts` files on this Node version.
- **Data directory:** `$WAID_HOME` defaults to `C:\data\waid`, which already exists and is a git repo. `ensureHome` must be idempotent against a populated home — it never clobbers an existing `config.json`, `events.jsonl`, `.gitignore`, or `.git` — and `cache/` must end up gitignored.
- **Writes:** only `$WAID_HOME/events.jsonl`, `config.json`, `.gitignore`, and `cache/*`. Nothing else on the machine is modified.
- **`events.jsonl` is append-only.** No command ever rewrites or truncates it.
- **Fold ordering:** file order is authoritative; `ts` is display metadata and is never used for ordering.
- **Timestamps:** ISO 8601 UTC.
- **Exit codes:** `0` success, `1` user error, `2` internal error.
- **`--json`** prints a single JSON document to stdout and nothing else; errors go to stderr as JSON when `--json` is set.
- **Global flags on every command:** `--json`, `--no-sync`, `--waid-home <path>`.
- **Malformed input is never fatal.** Unparseable lines, unknown `ev` values, and events for unknown ids are skipped and counted.
- **Config defaults:** `scanRoots: ["C:\\dev"]`, `claudeDir: <homedir>/.claude`, `activeWindowDays: 30`, `ghUser: null`, `ghCacheTtlMinutes: 15`, `scanMaxDepth: 4`.

## File Structure

```
C:\dev\waid\
  package.json  tsconfig.json  .gitignore  .gitattributes  README.md
  bin/waid.ts               entry point → run(argv) → exit code
  src/
    types.ts                shared shapes: Config, Ctx, Item, WaidEvent, Problem,
                            Session, Signal — grown by Tasks 1, 3, 10, 18
    errors.ts               UserError (exit 1) vs everything else (exit 2)
    args.ts                 argv → {command, args, flags}
    config.ts               WAID_HOME resolution, defaults, first-run init
    ids.ts                  Crockford base32 id generation
    events.ts               pure fold: raw lines → items, dismissed keys, problems
    store.ts                event log read/append, item lookup
    projects.ts             known-project set, -p resolution
    format.ts               relative time, padding, local date/week bounds
    harvest.ts              pure transcript accumulator
    sessions.ts             transcript discovery, incremental cache, sync
    git.ts                  git subprocess wrappers
    repos.ts                repo discovery + activity gate
    gh.ts                   gh client seam + response cache
    detect.ts               signal assembly, ranking, dismissal filtering
    cli.ts                  dispatch, global flags, implicit sync, output, exit codes
    commands/
      add.ts done.ts reopen.ts note.ts show.ts list.ts loops.ts doctor.ts
      sync.ts today.ts week.ts scan.ts promote.ts dismiss.ts
  agent/
    CLAUDE.snippet.md       text to paste into ~/.claude/CLAUDE.md
    skills/waid/SKILL.md    source for ~/.claude/skills/waid/SKILL.md
  test/
    helpers.ts              temp WAID_HOME + in-process CLI runner
    fixtures/transcripts/*.jsonl
    *.test.ts
```

**`src/types.ts` is the plan's spine.** Every "Interfaces produced" block below is a type in this file, so the compiler — not prose — enforces that Task 18 and Task 21 agree on what a `Signal` is. Tasks add to it; no task redefines what an earlier one declared.

**Command module contract:** each exports `run(ctx: Ctx) => Promise<D>` and `render(data: D, ctx: Ctx) => string`; commands needing the session cache also export `needsSessions = true`. A `CommandModule<D>` type in `types.ts` expresses this, and the dispatch table in `cli.ts` is typed against it.

**Every task follows the same step rhythm:** write the listed tests → run them and confirm they fail for the right reason → implement → run them and confirm they pass → run `npm test` and `npm run typecheck` clean → commit with the given message.

---

## Phase 1 — Capture and close items from the CLI

*Spec stage 1. Done when: I can capture and close items from the CLI without any transcript or git involvement.*

### Task 1: Scaffold, TypeScript setup, and config

**Files:** create `package.json`, `tsconfig.json`, `.gitignore`, `.gitattributes`, `bin/waid.ts`, `src/types.ts`, `src/errors.ts`, `src/config.ts`; test `test/config.test.ts`.

**TypeScript setup — get this exactly right, every later task depends on it:**

- `package.json`: `"type": "module"`, `"bin": {"waid": "bin/waid.ts"}`, `"engines": {"node": ">=24"}`, `"devDependencies": {"typescript": "^5"}`, scripts `"test": "node --test"` and `"typecheck": "tsc --noEmit"`.
- `tsconfig.json` compiler options: `"module": "nodenext"`, `"moduleResolution": "nodenext"`, `"target": "esnext"`, `"lib": ["esnext"]`, `"types": ["node"]`, `"strict": true`, `"noEmit": true`, `"erasableSyntaxOnly": true`, `"allowImportingTsExtensions": true`, `"verbatimModuleSyntax": true`, `"noUncheckedIndexedAccess": true`. Include `bin`, `src`, `test`.
- `@types/node` is *not* installed — that would be a second dependency. If `"types": ["node"]` cannot resolve without it, add `@types/node` to `devDependencies` and note it; runtime dependencies stay at zero either way.
- `.gitattributes` marks `test/fixtures/**` as `-text` so line-ending fixtures stay byte-exact.

**Interfaces produced:**
- `src/types.ts`: `Config` (every default key plus derived `home`, `configPath`, `eventsPath`, `cacheDir`, `sessionsCachePath`, `ghCachePath`), `Flags`, `Ctx = {cfg, flags, args, cwd, now, sessions}`, `CommandModule<D>`.
- `UserError(message, opts?: {candidates?: string[] | null})` — the only error class mapping to exit 1.
- `DEFAULTS: Omit<Config, 'home' | 'configPath' | 'eventsPath' | 'cacheDir' | 'sessionsCachePath' | 'ghCachePath'>`.
- `resolveHome(override?: string): string` — override, else `WAID_HOME`, else `C:\data\waid`.
- `ensureHome(home: string, opts?: {detectUser?: () => string | null}): void` — idempotent first-run init.
- `loadConfig(home: string): Config`.
- `detectGhUser(): string | null` — `gh api user -q .login`, swallowing all failure; skipped when `WAID_SKIP_GH_DETECT` is set.

- [ ] **Step 1 — Write the failing tests.** Cover: `resolveHome` precedence across all three sources; `ensureHome` creates `cache/`, `.gitignore` containing `cache/`, an empty `events.jsonl`, and a `config.json` seeded with the detected gh user; `ensureHome` never overwrites an existing `config.json`; `loadConfig` fills missing keys from defaults and derives every path; `loadConfig` throws `UserError` naming `config.json` on invalid JSON; `ensureHome` against a pre-existing populated home (a git repo with `config.json`, a non-empty `events.jsonl`, and a `.gitignore`) leaves all of them byte-identical and only adds `cache/` to `.gitignore` if it is missing.
- [ ] **Step 2 — Run and confirm failure** (`node --test`, module not found).
- [ ] **Step 3 — Implement** the scaffold, `tsconfig.json`, `src/types.ts`, and `src/config.ts`. `bin/waid.ts` only imports `run` from `src/cli.ts` and sets `process.exitCode` — it holds no logic.
- [ ] **Step 4 — Run and confirm pass**, then `npm run typecheck` clean.
- [ ] **Step 5 — Prove the toolchain end to end** before building on it: `node bin/waid.ts` runs without a build step, and a deliberate `enum` anywhere in `src/` is rejected by `npm run typecheck` under `erasableSyntaxOnly`. Remove the `enum` afterwards.
- [ ] **Step 6 — Commit:** `feat: scaffold waid package with native TypeScript and config module`.

### Task 2: Item ids

**Files:** create `src/ids.ts`; test `test/ids.test.ts`.

**Interfaces produced:** `ALPHABET` (Crockford base32, 32 chars, no `i`/`l`/`o`/`u`); `newId(exists?: (id: string) => boolean): string` — 4 chars from `crypto.randomInt`, retries while `exists(id)`, throws after a bounded number of attempts.

- [ ] **Step 1 — Write the failing tests.** Cover: alphabet length and banned characters; 200 generated ids are 4 chars drawn only from the alphabet; `newId` retries exactly as long as `exists` returns true; `newId` throws rather than looping forever when `exists` always returns true.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement.**
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add Crockford base32 item ids`.

### Task 3: Event log — fold, read, append

**Files:** create `src/events.ts`, `src/store.ts`; extend `src/types.ts`; test `test/events.test.ts`, `test/store.test.ts`.

**Interfaces produced:**
- In `types.ts`: `Status = 'open' | 'waiting' | 'done'`; `Item = {id, title, status: Status, waitingOn: string | null, project: string | null, session: string | null, tags: string[], notes: {ts: string, text: string}[], created: string, updated: string}`; `WaidEvent` as a discriminated union on `ev` covering `add | update | note | close | reopen | dismiss | undismiss`; `Problem = {line: number, reason: ProblemReason, id?: string | null, ev?: string | null}`; `State = {items: Item[], dismissed: string[], problems: Problem[]}`.
- `STATUSES` — an `as const` tuple, **not** an enum.
- `foldEvents(rawLines: string[]): State`.
- `readEventLines(cfg)`, `readEvents(cfg): {line: number, ev: unknown}[]`, `appendEvent(cfg, event): WaidEvent` (stamps `ts` unless the caller supplied one), `loadState(cfg): State`, `requireItem(state, id): Item` (throws `UserError` on unknown id).

**`ProblemReason` union:** `'unparseable' | 'not-an-object' | 'add-missing-fields' | 'duplicate-id' | 'unknown-id' | 'note-missing-text' | 'bad-status' | 'missing-key' | 'unknown-ev'`.

Note the typing discipline here: the fold's *input* is untrusted `unknown` parsed from disk and must be narrowed before use — `WaidEvent` describes what waid writes, never what it assumes it read.

- [ ] **Step 1 — Write the failing fold tests.** Cover: `add`→`note`→`update`→`close` folds to one fully-populated item; `add` applies defaults for every omitted field; `update` patches only supplied fields and leaves the rest intact; `reopen` sets status open **and** clears `waitingOn`; a later line with an earlier `ts` still wins (file order authoritative) and sets `updated` to that earlier `ts`; `dismiss`/`undismiss` maintain the key set; a mixed batch of blank lines, unparseable JSON, a JSON array, an event for an unknown id, an unknown `ev`, a duplicate `add`, and a bad status yields the right items **and** the exact `(line, reason)` list.
- [ ] **Step 2 — Write the failing store tests.** Cover: `appendEvent` stamps a valid ISO `ts` and writes exactly one line per call; a caller-supplied `ts` is preserved; `readEvents` returns parsed events with 1-based line numbers; `loadState` folds what was appended and `requireItem` throws on an unknown id; `loadState` on a home with no `events.jsonl` returns empty arrays rather than throwing.
- [ ] **Step 3 — Run and confirm failure.**
- [ ] **Step 4 — Implement.** `foldEvents` is pure — it takes lines, touches no filesystem, and never throws.
- [ ] **Step 5 — Run tests and typecheck clean.**
- [ ] **Step 6 — Commit:** `feat: add append-only event log and fold`.

### Task 4: CLI skeleton — parsing, dispatch, exit codes

**Files:** create `src/args.ts`, `src/cli.ts`, `test/helpers.ts`, `test/args.test.ts`, `test/cli.test.ts`.

**Interfaces produced:**
- `parseArgv(argv: string[]): {command: string | null, args: string[], flags: Flags}`. Boolean flags: `json`, `no-sync`, `full`, `last`, `all`, `help`, `version`. Aliases: `-p`→`project`, `-t`→`tag`, `-h`→`help`. Repeatable into an array: `tag`.
- `run(argv: string[], io?: Io): Promise<number>` with `Io = {out(text: string): void, err(text: string): void}` so tests capture output in-process.
- `test/helpers.ts`: `makeHome(): string` (temp dir, sets `WAID_SKIP_GH_DETECT`) and `waid(home, argv): Promise<{code, out, err, json(): unknown}>`.

The `COMMANDS` dispatch table is typed `Record<string, CommandModule<unknown>>` and starts empty; each later task registers its command into it.

- [ ] **Step 1 — Write the failing arg tests.** Cover: command / positionals / value flags split correctly; a boolean flag does not swallow the following argument; `--flag=value` and repeated `-t`/`--tag` accumulate; `--` ends flag parsing; a value flag with no value throws `UserError`; empty argv yields a null command.
- [ ] **Step 2 — Write the failing CLI tests.** Cover: bare invocation prints the usage block (matching the spec's CLI surface) and exits 0; an unknown command exits 1 with `unknown command: …` on stderr; the same error under `--json` is a parseable JSON object on stderr; running any command initialises `WAID_HOME` (config and events file exist afterwards).
- [ ] **Step 3 — Run and confirm failure.**
- [ ] **Step 4 — Implement.** `run` resolves home → `ensureHome` → `loadConfig` → builds `ctx` → dispatches → renders (`--json` stringifies `data`, otherwise `render`). `UserError` → exit 1; anything else → stack on stderr, exit 2.
- [ ] **Step 5 — Run tests and typecheck clean.**
- [ ] **Step 6 — Commit:** `feat: add CLI arg parsing, dispatch and exit codes`.

### Task 5: Project resolution and `waid add`

**Files:** create `src/projects.ts`, `src/commands/add.ts`; test `test/projects.test.ts`, `test/add.test.ts`; modify `src/cli.ts` to register `add`.

**Interfaces produced:**
- `normalizePath(p: string): string` — trims and strips trailing separators, leaving bare roots (`C:\`, `/`) alone.
- `knownProjects(state: Pick<State, 'items'>, sessions?: Session[]): string[]` — union of item and session project paths, nulls dropped, order preserved.
- `resolveProject(input: string | undefined, known: string[], cwd: string): string | null` — empty/undefined → `null`; `.` → normalized `cwd`; absolute (POSIX or `C:\…`) → normalized as given; otherwise case-insensitive substring match against `known`, throwing `UserError` on zero matches and on multiple matches (carrying `candidates`).
- `add.run(ctx): Promise<AddResult>` where `AddResult = {id, title, status, project, tags, waitingOn, session, created}`.

**Behaviour:** title is the joined positionals; `--waiting-on` implies `status: "waiting"`; the id is generated against the ids already in the folded log.

- [ ] **Step 1 — Write the failing project tests.** Cover each `resolveProject` branch above including the ambiguous case asserting on `error.candidates`, plus `knownProjects` de-duplication and `normalizePath` root handling.
- [ ] **Step 2 — Write the failing add tests.** Cover: `add` with `-p`, `--tag`, `--session`, `--json` returns a complete record with a 4-char id; `--waiting-on` yields waiting status and a populated `waitingOn`; a partial `-p` resolves against a project used by an earlier `add`; `add` with no title exits 1; human output is a single `added <id>  <title>` line.
- [ ] **Step 3 — Run and confirm failure.**
- [ ] **Step 4 — Implement** and register the command.
- [ ] **Step 5 — Run tests and typecheck clean.**
- [ ] **Step 6 — Commit:** `feat: add project resolution and waid add`.

### Task 6: `done`, `reopen`, `note`

**Files:** create `src/commands/done.ts`, `reopen.ts`, `note.ts`; test `test/mutations.test.ts`; modify `src/cli.ts`.

**Interfaces produced:** `done.run → {id, title, status: 'done', ts}`; `reopen.run → {id, title, status: 'open', ts}`; `note.run → {id, title, text, ts}`. All three validate the id against the folded state before appending, so an unknown id never writes an event.

- [ ] **Step 1 — Write the failing tests.** Cover: `done` closes an item and it reappears as done under `list --all`; `reopen` on a waiting item clears `waitingOn` (asserted via `show`); `note` appends text visible in `show`; every mutation with an unknown id exits 1 with `unknown item id: …`; `note` with no text exits 1.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement** all three and register them.
- [ ] **Step 4 — Run.** The `list`/`show` assertions stay red until Task 7; confirm the unknown-id and missing-text cases pass now, and that typecheck is clean.
- [ ] **Step 5 — Commit:** `feat: add done, reopen and note commands`.

### Task 7: Formatting helpers, `show` and `list`

**Files:** create `src/format.ts`, `src/commands/show.ts`, `src/commands/list.ts`; test `test/format.test.ts`, `test/list.test.ts`; modify `src/cli.ts`.

**Interfaces produced:**
- `relTime(iso: string, now?: Date): string` — compact `3h` / `5d` / `2w` style, `just now` under a minute.
- `pad(text: string, width: number): string`, `localYmd(value: Date | string): string`, `dayBounds(ymd: string): {start: Date, end: Date}`, `weekBounds(date: Date, offsetWeeks?: number): {start: Date, end: Date}` with **Monday-start** weeks in local time.
- `show.run(ctx) → {item: Item, history: {ts, ev, line}[]}` — history is every log line mentioning the id, in file order.
- `list.run(ctx) → {items: Item[], filters: {status, project, tag, all}}` — default hides `done`; `--all` includes it; `--status`, `--project` (resolved), and `--tag` filter conjunctively.

- [ ] **Step 1 — Write the failing format tests.** Cover each `relTime` bucket boundary; `pad` truncation and padding; `localYmd` for a `Date` and an ISO string; `dayBounds` spanning exactly one local day; `weekBounds` starting Monday for a mid-week date, for a Sunday, and for `offsetWeeks: -1`.
- [ ] **Step 2 — Write the failing list/show tests.** Cover: `list` hides done items by default and `--all` reveals them; `--status`, `--tag`, and a partial `--project` each filter correctly; `show` on an unknown id exits 1; `show --json` returns the item with notes plus a history entry per event; human `list` output groups by project.
- [ ] **Step 3 — Run and confirm failure.**
- [ ] **Step 4 — Implement** and register both commands.
- [ ] **Step 5 — Run the full suite** — Task 6's deferred assertions must now pass too — and typecheck clean.
- [ ] **Step 6 — Commit:** `feat: add show, list and formatting helpers`.

### Task 8: `loops` — declared items only

**Files:** create `src/commands/loops.ts`; extend `src/types.ts` with `LoopsResult`; test `test/loops.test.ts`; modify `src/cli.ts`.

**Interfaces produced:** `LoopsResult = {groups: {project: string | null, items: Item[]}[], detected: Signal[], dismissedCount: number, notes: string[]}`. `Signal` is declared in Task 18; until then `detected` is typed as `Signal[]` against a placeholder declaration so the contract is fixed from day one and Phase 3 only fills it.

**Behaviour:** open and waiting items only, grouped by project (null project grouped last under `(no project)`), each group ordered oldest-updated first. `-p` narrows to one project.

- [ ] **Step 1 — Write the failing tests.** Cover: done items are excluded; grouping and ordering; `-p` narrowing; empty state renders a friendly "no open loops" line and exits 0; human output matches the spec's sample layout (`OPEN LOOPS` header, per-project indent, id / status / title / tags / relative age columns).
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement** and register.
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add waid loops for declared items`.

### Task 9: `doctor`

**Files:** create `src/commands/doctor.ts`; test `test/doctor.test.ts`; modify `src/cli.ts`.

**Interfaces produced:** `doctor.run(ctx) → {home, config: {path, valid, unknownKeys}, log: {path, lines, items, problems}, cache: {sessions: {exists, syncedAt, ageMinutes}}, gh: {available, user}, ok}`.

**Behaviour:** reports the fold's `problems` with line numbers, flags unknown config keys, and never mutates anything. Exits 0 even when it finds problems — it is a report, not a gate. The `cache` and `gh` sections degrade gracefully to `{exists: false}` / `{available: false}` in this phase and are filled out by Phases 2 and 3.

- [ ] **Step 1 — Write the failing tests.** Cover: a clean home reports `ok: true` and zero problems; a log seeded with an unparseable line and an event for an unknown id reports both with correct line numbers and still exits 0; an unknown config key is listed; the gh section reports unavailable under `WAID_SKIP_GH_DETECT`.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement** and register.
- [ ] **Step 4 — Run the full suite and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add waid doctor`.

---

## Phase 2 — Sessions summarise my real work with no input from me

*Spec stage 2. Done when: `waid week` summarises my actual work from transcripts alone.*

### Task 10: Transcript accumulator (pure)

**Files:** create `src/harvest.ts`; extend `src/types.ts` with `Session`; add `test/fixtures/transcripts/*.jsonl`; test `test/harvest.test.ts`.

**Interfaces produced:**
- In `types.ts`: `Session = {id, title, project: string | null, branch: string | null, started: string | null, ended: string | null, prompts: number}` and `CachedSession = Session & {_file: {path, mtimeMs, size}}`.
- `newAccumulator(): Accumulator`; `feedLine(acc: Accumulator, rawLine: string): void`; `finalize(acc: Accumulator, opts: {fallbackProject: string | null}): Session`. Pure and streaming — no filesystem access, one line at a time, so `sessions.ts` can pipe a read stream through it.
- `decodeProjectSlug(dirName: string): string` — best-effort, explicitly lossy, used only as a last resort.

**Extraction rules (from the spec):** title = `aiTitle` of the **last** `ai-title` record, else first 80 chars of the first non-meta user message, else `"(untitled)"`. Project = `cwd` of the first non-sidechain user record, else the decoded parent directory name. Branch = `gitBranch` of the last user record that has one. `started`/`ended` = first and last `timestamp` across **all** records including sidechains. `prompts` = count of user records where both `isSidechain` and `isMeta` are falsy.

Transcript records are third-party data: parse to `unknown` and narrow. Do not model the full Claude Code record schema — declare only the handful of fields listed above.

**Fixtures to commit:** a normal session with several `ai-title` records; a session with no `ai-title`; a sidechain-only session; a session whose final line is truncated mid-JSON; a CRLF session; an empty file.

- [ ] **Step 1 — Write the failing tests** against those fixtures, one test per fixture, asserting the full finalized record. Add explicit cases for: last `ai-title` wins over earlier ones; sidechain records raise `ended` but do not raise `prompts`; a truncated final line is silently skipped and the rest of the session still parses; CRLF lines parse identically to LF; an empty file finalizes to `(untitled)` with zero prompts.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement.**
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add transcript accumulator`.

### Task 11: Session discovery, incremental cache, `waid sync`

**Files:** create `src/sessions.ts`, `src/commands/sync.ts`; test `test/sessions.test.ts`; modify `src/cli.ts`.

**Interfaces produced:**
- `discoverTranscripts(cfg): {path, dir, id, mtimeMs, size}[]` — globs `$claudeDir/projects/*/*.jsonl`, tolerating a missing `claudeDir`.
- `syncSessions(cfg, opts?: {full?: boolean, now?: Date}): SessionCache & {stats: {scanned, parsed, reused, removed}}` where `SessionCache = {version: 1, syncedAt: string, sessions: CachedSession[]}` — reuses a cached record when both `mtimeMs` and `size` match; `full` forces a rebuild; drops records whose file no longer exists.
- `loadSessions(cfg): {syncedAt: string | null, sessions: CachedSession[]}` — reads the cache, returning an empty set when absent or version-mismatched.
- `cacheAgeMinutes(cfg, now: Date): number | null`.
- `sync.run(ctx) → stats`.

- [ ] **Step 1 — Write the failing tests** against a temp `claudeDir` built from the Task 10 fixtures. Cover: a first sync parses every transcript; a second sync with untouched files reports `reused` equal to the file count and re-reads nothing (assert by injecting a counting reader); touching a file's mtime forces a re-parse; `--full` re-parses everything; deleting a transcript drops it from the cache on the next sync; deleting the whole `cache/` directory is safe and recovers on the next sync; a missing `claudeDir` yields zero sessions and no error.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement** and register `sync`.
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add incremental session harvest and waid sync`.

### Task 12: Implicit sync in the CLI

**Files:** modify `src/cli.ts`; test `test/implicit-sync.test.ts`.

**Behaviour:** before dispatching a command that exports `needsSessions = true`, `run` syncs when the cache is missing or older than 5 minutes, then puts the result on `ctx.sessions`. `--no-sync` always skips. Sync failure is non-fatal: `ctx.sessions` falls back to whatever is cached and a note is attached to the rendered output.

- [ ] **Step 1 — Write the failing tests.** Cover: a `needsSessions` command with a stale cache triggers a sync; with a fresh cache it does not; `--no-sync` suppresses it in both cases; a command without `needsSessions` never syncs; a throwing sync leaves the command working with the stale cache and exit code 0.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement.**
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: sync the session cache implicitly for read commands`.

### Task 13: `waid today`

**Files:** create `src/commands/today.ts`; test `test/today.test.ts`; modify `src/cli.ts`. Exports `needsSessions = true`.

**Interfaces produced:** `today.run(ctx) → {date, sessions: Session[], items: {added: Item[], closed: Item[], noted: Item[]}, projects: {project, sessions, prompts}[]}`.

**Behaviour:** `--date YYYY-MM-DD` overrides today; the day window is local time via `dayBounds`. A session counts for a day if its span overlaps the window. Sessions with zero prompts are excluded from output (but stay in the cache). Item activity comes from `readEvents` filtered by `ts` within the window.

- [ ] **Step 1 — Write the failing tests.** Cover: sessions on the requested day appear grouped by project with prompt counts; a zero-prompt session is omitted; a session spanning midnight appears on both days; `--date` selects a past day; items added, closed, and noted that day are listed; a day with nothing renders an explicit empty message and exits 0.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement** and register.
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add waid today`.

### Task 14: `waid week`

**Files:** create `src/commands/week.ts`; test `test/week.test.ts`; modify `src/cli.ts`. Exports `needsSessions = true`.

**Interfaces produced:** `week.run(ctx) → {start, end, projects: {project, sessions, prompts, titles, itemsClosed}[], totals: {sessions, prompts, closed}}`.

**Behaviour:** Monday-start week via `weekBounds`; `--last` shifts back one week. Grouped by project, projects ordered by prompt count descending, each carrying its session titles so the rollup reads as a work log.

- [ ] **Step 1 — Write the failing tests.** Cover: the current week includes only sessions inside the Monday–Sunday window; `--last` selects the previous window; projects are ordered by prompts descending; session titles are listed per project; closed items in the window are counted per project; totals match the sum of the groups; an empty week renders an explicit empty message.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement** and register.
- [ ] **Step 4 — Run the full suite and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add waid week rollup`.

---

## Phase 3 — Detection surfaces loops I had forgotten

*Spec stage 3. Done when: `waid loops` shows real open loops I hadn't declared.*

### Task 15: Git wrappers

**Files:** create `src/git.ts`; extend `src/types.ts` with `GitClient`; test `test/git.test.ts`.

**Interfaces produced:** a `GitClient` interface — `headCommitDate(repo): Date | null`; `currentBranch(repo): string | null`; `aheadCount(repo): number | null` (null when there is no upstream); `dirtyFileCount(repo): number`; `isRepo(dir): boolean` — plus `realGitClient(): GitClient`. Declaring the interface rather than loose functions is what lets Tasks 16 and 18 inject fakes with compiler-checked shape. Every wrapper is bounded by a short timeout and returns null / zero rather than throwing when `git` fails or the directory is not a repo.

- [ ] **Step 1 — Write the failing tests** against real temp repos created in test setup (init, commit, branch, dirty file). Cover: each wrapper's happy path; `aheadCount` on a branch with no upstream returns null; `aheadCount` after committing against a local "remote" clone returns the right number; a non-repo directory returns falsy from every wrapper without throwing; a nonexistent path does not throw.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement.**
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add git subprocess wrappers`.

### Task 16: Repo discovery and the activity gate

**Files:** create `src/repos.ts`; test `test/repos.test.ts`.

**Interfaces produced:**
- `discoverRepos(cfg): string[]` — walks each `scanRoots` entry to `scanMaxDepth` looking for `.git`, skipping `node_modules`, `bin`, `obj`, and `.git` internals, and not descending into a repo once found.
- `activeRepos(cfg, sessions: CachedSession[], deps: {now: Date, git?: GitClient}): string[]` — keeps a repo when a harvested session's `project` path lies inside it within `activeWindowDays`, **or** its HEAD commit is within `activeWindowDays`.
- `repoForPath(path: string, repos: string[]): string | null` — longest-prefix match, used to attribute sessions and signals to repos.

- [ ] **Step 1 — Write the failing tests** against a temp tree of fake repos (`.git` directories) at varying depths, injecting a fake `GitClient` so no real commits are needed. Cover: discovery finds repos at depth 1 and at `scanMaxDepth`; a repo deeper than `scanMaxDepth` is not found; `node_modules` and nested `.git` internals are skipped; a nested repo inside a repo is not double-reported; the activity gate admits a repo with a recent session and one with a recent HEAD, and rejects a repo with neither; `repoForPath` prefers the longest matching repo prefix; a missing scan root is skipped without error.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement.**
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add repo discovery and the activity gate`.

### Task 17: GitHub client seam and response cache

**Files:** create `src/gh.ts`; extend `src/types.ts` with `GhClient` and `GhPr`; test `test/gh.test.ts`.

**Interfaces produced:** `GhClient = {reviewRequested(): GhPr[], authored(): GhPr[]}` and `realGhClient(): GhClient` shelling out to `gh search prs --review-requested=@me --state=open` and `--author=@me --state=open` with `--json` fields; `fetchGh(cfg, deps: {client: GhClient, now: Date}): {available: boolean, reason?: string, reviewRequested: GhPr[], authored: GhPr[], cachedAt: string}` — reads `cache/gh.json` when it is younger than `ghCacheTtlMinutes`, otherwise calls the client and rewrites the cache. Missing, unauthenticated, timed-out, or erroring `gh` yields `{available: false, reason}` and never throws.

`GhPr` declares only the fields waid renders — number, repository, title, author, review state, created date — and `gh`'s JSON is narrowed into it rather than trusted.

Both queries are account-wide, so detection costs exactly two calls regardless of repo count.

- [ ] **Step 1 — Write the failing tests** with a fake client. Cover: a fresh cache is used and the client is not called; an expired cache triggers exactly one call per query and rewrites the cache; a throwing client yields `available: false` with a reason and leaves any existing cache intact; a corrupt `cache/gh.json` is treated as a miss, not an error; `ghUser: null` in config short-circuits to unavailable.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement.**
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add gh client seam and response cache`.

### Task 18: Signal assembly, ranking and dismissal

**Files:** create `src/detect.ts`; replace the placeholder `Signal` in `src/types.ts` with the real declaration; test `test/detect.test.ts`.

**Interfaces produced:** `Signal = {key: string, kind: SignalKind, title: string, detail: string, project: string | null, age: string}` with `SignalKind = 'review' | 'pr' | 'ahead' | 'dirty'` (a union, not an enum); `detectSignals(cfg, deps: {sessions, state, now, git: GitClient, gh: GhClient}): {signals: Signal[], dismissedCount: number, notes: string[]}`.

**Signals and key formats (exactly as specified):**

| Kind | Key | Detail |
|---|---|---|
| `review` | `review:owner/repo#123` | title, author, age |
| `pr` | `pr:owner/repo#123` | title, review state, age |
| `ahead` | `ahead:<repo>:<branch>` | branch, commits ahead |
| `dirty` | `dirty:<repo>` | file count, branch |

Ranked in that order; ties broken by age descending. Keys in the folded `dismissed` set are removed and counted into `dismissedCount`. When GitHub is unavailable, `notes` carries one line explaining it and the git signals still return.

- [ ] **Step 1 — Write the failing tests** with injected `GitClient` and `GhClient` fakes. Cover: all four signal kinds are produced with exactly the documented key format; ranking order across a mixed set; dismissed keys are filtered and counted; dismissing `pr:o/r#123` leaves `pr:o/r#124` visible; an unavailable `gh` produces git signals plus a note; a repo with no upstream produces no `ahead` signal; a clean repo produces no `dirty` signal; a repo that is both dirty and ahead produces two distinct signals.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement.** Swapping the placeholder `Signal` for the real one must not require edits to `loops.ts` — if it does, Task 8's contract was wrong and this is where you find out.
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add signal detection, ranking and dismissal`.

### Task 19: `waid scan`

**Files:** create `src/commands/scan.ts`; test `test/scan.test.ts`; modify `src/cli.ts`. Exports `needsSessions = true`.

**Interfaces produced:** `scan.run(ctx) → {signals: Signal[], dismissedCount: number, notes: string[]}`. `-p` narrows to signals whose project matches the resolved project.

- [ ] **Step 1 — Write the failing tests.** Cover: `--json` returns ranked signals; `-p` narrows correctly; a clean machine renders an explicit empty message and exits 0; the unavailable-gh note is rendered as a single dim line; the footer names `waid promote` and `waid dismiss`.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement** and register. The `GitClient` and `GhClient` seams reach the command through optional fields on `Ctx`, defaulting to the real clients, so tests inject fakes without monkey-patching modules.
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add waid scan`.

### Task 20: `dismiss` and `promote`

**Files:** create `src/commands/dismiss.ts`, `src/commands/promote.ts`; test `test/promote.test.ts`; modify `src/cli.ts`.

**Interfaces produced:** `dismiss.run(ctx) → {key, dismissed: true}`; `promote.run(ctx) → {id, key, title, project}` — appends an `add` seeded from the signal (title from the PR title or a generated description, `project` from the repo path, `tags: ["promoted"]`), then appends a `dismiss` for the key so the signal does not appear twice.

- [ ] **Step 1 — Write the failing tests.** Cover: `dismiss` appends one `dismiss` event and the key stops appearing in `scan`; `promote` writes exactly two events in order (`add` then `dismiss`), the new item carries the `promoted` tag and the repo path as its project, and the signal disappears from `scan` while the item appears in `loops`; promoting an unknown key exits 1; dismissing an already-dismissed key is a harmless no-op that still exits 0.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement** and register both.
- [ ] **Step 4 — Run tests and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: add promote and dismiss`.

### Task 21: Wire detection into `loops` and `doctor`

**Files:** modify `src/commands/loops.ts`, `src/commands/doctor.ts`; test `test/loops-detected.test.ts`; modify `src/cli.ts` (`loops` gains `needsSessions = true`).

**Behaviour:** `loops` fills the `detected` array reserved in Task 8 and renders it as a separate `DETECTED (N shown, M dismissed)` section beneath declared items — never merged into them. `doctor` gains gh availability, cache freshness, and repo/activity counts.

- [ ] **Step 1 — Write the failing tests.** Cover: declared items and detected signals appear in separate sections with the spec's exact header text and counts; the human output matches the spec's sample layout including the trailing `waid promote <key> …` hint; detection failure degrades to declared items plus a note and still exits 0; `--json` carries both arrays; `doctor` reports gh availability and cache age.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Implement.**
- [ ] **Step 4 — Run the full suite and typecheck clean.**
- [ ] **Step 5 — Commit:** `feat: show detected signals in waid loops`.

---

## Phase 4 — Agents record items without being asked

*Spec stage 4. Done when: sessions record items on their own and I stop typing `waid add` by hand.*

### Task 22: Install and README

**Files:** create `README.md`; test `test/smoke.test.ts`.

**Behaviour:** `npm link` from `C:\dev\waid` exposes `waid`. The README documents install, every command, the config keys, the `WAID_HOME` layout (default `C:\data\waid`, itself a git repo), and the fact that there is no build step — sources run as written.

This task is where the TypeScript decision gets its real proof. `npm link` symlinks the package into the global `node_modules`, and Node refuses to strip types for files that genuinely live under `node_modules` — it works here only because symlink realpath resolution puts the entry back at `C:\dev\waid`. Verified working on v24.11.0 with a `.ts` `bin` entry, but confirm it on the real package rather than assuming.

- [ ] **Step 1 — Write the failing smoke test:** spawn `bin/waid.ts` as a real child process against a temp `WAID_HOME` and assert `--help` exits 0, `add` then `loops --json` round-trips an item, and stdout under `--json` parses as a single document with nothing else on it.
- [ ] **Step 2 — Run and confirm failure.**
- [ ] **Step 3 — Write the README** and fix whatever the smoke test exposes about the real entry point.
- [ ] **Step 4 — Run the full suite and typecheck**, then `npm link` and verify `waid loops` works from another directory — confirming no `ERR_UNSUPPORTED_NODE_MODULES_TYPE_STRIPPING`.
- [ ] **Step 5 — Commit:** `docs: add README and end-to-end smoke test`.

### Task 23: Global CLAUDE.md snippet

**Files:** create `agent/CLAUDE.snippet.md`.

**Content:** roughly five lines, matching the spec verbatim in intent — record bugs, deferred work, and blockers with `waid add … --json`; close with `waid done <id> --json`; always pass `--json`; **and explicitly do not record routine in-session steps.** That last line is what keeps the log a list of open loops rather than a transcript.

- [ ] **Step 1 — Write the snippet** and paste it into `~/.claude/CLAUDE.md` under its own heading.
- [ ] **Step 2 — Verify by hand:** start a session in another repo, report a bug conversationally, and confirm exactly one item is created with the right project.
- [ ] **Step 3 — Verify the negative case:** complete a routine multi-step task and confirm nothing was recorded.
- [ ] **Step 4 — Commit:** `docs: add CLAUDE.md tracking snippet`.

### Task 24: `/waid` skill

**Files:** create `agent/skills/waid/SKILL.md`.

**Content:** three flows, each with explicit approval before any write —
- **Wrap up** — review the session and propose items for unfinished work.
- **Triage** — walk detected signals, promoting or dismissing each.
- **Review** — read back the week and propose closures for items evidently finished.

- [ ] **Step 1 — Write the skill** following the frontmatter conventions of the existing personal skills.
- [ ] **Step 2 — Install** it to `~/.claude/skills/waid/SKILL.md`.
- [ ] **Step 3 — Verify each flow by hand** in a real session, confirming nothing is written without approval.
- [ ] **Step 4 — Commit:** `docs: add /waid skill`.

---

## Phase 5 — Later (explicitly not part of the initial build)

*Spec stage 5. Nice-to-have follow-up; do not start until Phases 1–4 have been in daily use.*

### Task 25: SessionStart hook

Add a `settings.json` hook running `waid loops -p <cwd> --json` and injecting the output as `additionalContext`. It must fail silently and finish fast — a broken or slow `waid` must never block a session from starting. Requires a hard timeout and swallowed errors, verified by pointing the hook at a deliberately broken `WAID_HOME`.

Measure startup here specifically: type stripping parses every imported `.ts` on each invocation. If the hook's latency is uncomfortable, `module.enableCompileCache()` in `bin/waid.ts` is the first thing to try, before considering a build step.

### Task 26: Nightly digest agent

A scheduled agent that runs `waid week --json` and writes a digest. Deferred until the weekly rollup has proven itself by hand.

---

## Open questions carried from the spec

1. ~~**Does `$WAID_HOME` become its own git repo?**~~ Settled: yes. `C:\data\waid` is the home and is already a git repo, so `ensureHome` treats a populated home as the normal case.
2. **Does `events.jsonl` ever need compaction?** Not at projected volume. Revisit only if fold time becomes noticeable, and then via a `waid compact` that archives closed items older than N months.
