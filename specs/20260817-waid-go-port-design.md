# waid — Port to Go — Design

**Date:** 2026-08-17
**Status:** Approved design, pending implementation plan
**Followed by:** `20260817-waid-app-design.md`, which depends on this landing first.

## Purpose

waid is about to grow a full TUI. Written in Go, that TUI would import the domain directly; written
against the Node CLI it needs a subprocess boundary, mirrored types, and a JSON contract to defend.
That framing prompted the real question — whether the CLI itself should be Go — and three
measurements answered it.

**Startup.** Bare `node` costs 85ms before waid runs a line. `waid list --json` takes ~205ms warm.
Agents call `waid add` and `waid done` on a session cadence, so that latency is paid constantly.
A Go binary starts in single-digit milliseconds.

**Detection.** `waid loops --json` takes **5.9–7.9 seconds**. Almost none of it is thinking:
discovery finds 122 repos in 90ms, then `repos.ts:109` walks the active ones in a serial `for` loop
issuing ~4 git subprocesses each, at 87ms per subprocess on Windows. The wall clock is serialized
subprocess latency. A bounded goroutine pool collapses it to well under a second.

**Distribution.** `C:\Users\brent\go\bin` is already on PATH and is independent of any Node version
manager. `go install ./cmd/waid` makes `waid` resolve identically inside a repo pinned to Node 22
and one on Node 24. Today's install is `npm link`, which survives only because realpath resolution
happens to put the entry back outside `node_modules` before Node's type-stripping check runs — a
mechanism the README needs three paragraphs to explain.

The port is also unusually cheap right now. The codebase is three days old, 4,191 lines, and has
**zero runtime dependencies**: every external interaction is already a subprocess (`git`, `gh`) or
JSONL file I/O. There is no ecosystem to bring across.

## Non-goals

- **Not a redesign.** Commands, flags, output, and exit codes are reproduced, not improved. Every
  behaviour change is a separate decision made after the port is green.
- **Not an event log migration.** `events.jsonl` holds real data and is read and appended in place.
- **Not a rethink of detection.** The same four signal kinds, the same ranking, the same keys. Only
  the concurrency changes.
- **Not a Node/Go hybrid.** The Node tree survives only until differential testing passes, then it
  is deleted. Two implementations of the fold is the outcome this port exists to avoid.

## 1. The compatibility contract

The port is correct when these hold. Everything else is an implementation detail.

| Surface | Requirement |
| --- | --- |
| `events.jsonl` | Existing lines read identically; appended lines byte-identical to what Node writes. |
| `config.json` | Same keys, same defaults, same "never overwritten" first-run behaviour. |
| `--json` stdout | Identical documents. `CLAUDE.md` and the `/waid` skill are live consumers. |
| Human stdout | Identical text, including column widths and the `…` truncation. |
| stderr | Degradation notes, same wording. |
| Exit codes | `0` success, `1` user error, `2` internal with a stack. |
| Command surface | Every command and flag **except `-i`**, which is dropped (§6). |

`cache/` is explicitly **not** in the contract. It is derived, disposable, and gitignored, so the Go
build may store `sessions.json` and `gh.json` however it likes; a stale Node-written cache is
discarded and rebuilt rather than parsed.

## 2. Byte compatibility of the log

The fold treats **file order as authoritative** and `ts` as display metadata, because parallel agents
append out-of-order timestamps. That property is load-bearing and must survive.

Four encoding details differ between `JSON.stringify` and Go's `encoding/json` and will silently
corrupt compatibility if left at their defaults:

| Detail | Node | Go default | Required |
| --- | --- | --- | --- |
| HTML escaping | `<`, `>`, `&` emitted raw | escaped to `\u003c`, `\u003e`, `\u0026` | `Encoder.SetEscapeHTML(false)` |
| U+2028 / U+2029 | emitted raw | escaped | must not escape |
| Timestamp | `toISOString()` → `2026-08-17T15:32:04.642Z` | `RFC3339Nano` trims trailing zeros | `.UTC().Format("2006-01-02T15:04:05.000Z")` |
| Key order | insertion order | struct field order | struct fields declared in Node's emission order |

Note text is free-form and routinely carries `>` and `&`, so the HTML-escaping default is not a
theoretical concern — it would corrupt the first note written after cutover.

Item ids stay 4 characters of Crockford base32 over `0123456789abcdefghjkmnpqrstvwxyz` (the digits
and letters less `i`, `l`, `o`, `u`). Node draws from `crypto.randomInt`, which rejection-samples;
Go must do the same via `crypto/rand` rather than `%` on a random byte, or the alphabet's 32
characters will not be uniform over a 256-value byte.

Project paths are stored as literal Windows strings (`C:\dev\waid`). They are opaque identifiers in
the log, never re-derived, so `filepath` must not normalise them on the way through.

## 3. Package layout

```
waid/
  go.mod                          module github.com/brentkeller/waid
  cmd/waid/main.go                argv → cli.Run → exit code
  internal/
    config/                       WAID_HOME resolution, config.json, first-run
    events/     fold.go           lines → State. Pure, never throws, reports Problems.
                append.go         the only writer
    sessions/   harvest.go        transcript parsing
                cache.go          sessions.json, incremental by mtime+size
    git/                          subprocess wrappers; no method fails loudly
    gh/                           gh client + response cache
    repos/                        discovery, the activity gate
    detect/                       signals, ranking, dismissal
    project/                      -p resolution (path, `.`, unique substring)
    render/                       shared column formatting and truncation
    command/                      one file per command
    cli/                          flag parsing, dispatch, --json vs render
  testdata/
    transcripts/                  ported unchanged from test/fixtures
    golden/                       differential fixtures (§5)
```

Two boundaries carry over from the Node design because they earned their keep:

**`events` never touches the filesystem.** `fold` takes raw lines and returns `State`, narrowing
every untrusted field and turning anything unusable into a `Problem` with its 1-based line number.
It is the core of the tool and stays testable with a string slice.

**Commands return data, not text.** The Node `CommandModule` pairs `run(ctx) → D` with
`render(D) → string` so `--json` and human output cannot diverge. In Go that is an interface over a
generic result; the constraint matters more than the encoding of it.

## 4. Concurrency

The payoff, and the only place the port changes behaviour rather than reproducing it.

Repo probing becomes a bounded worker pool — `errgroup` with `SetLimit`. The work is subprocess-bound
rather than CPU-bound, so the limit is a fixed 16 rather than `NumCPU`; the constraint is process
spawn overhead, not cores.

Two rules keep it honest:

- **Ordering is never inherited from completion order.** Results are collected, then sorted by the
  existing rank (kind, then oldest first). Detection output must be byte-identical run to run, or
  the differential harness in §5 becomes useless.
- **Failures stay per-repo.** `GitClient`'s contract is that no method fails loudly — an absent or
  broken `git` reads as null/zero/false. A worker that panics must not take the pool with it.

The two `gh` queries are account-wide and already cached; they run concurrently with the git pool
rather than before it.

## 5. Differential testing

The cutover safety net, and the reason this port is verifiable rather than hopeful. Both binaries
run against the same data and their output is diffed.

**Read commands** are compared directly:

```
for cmd in list show today week loops scan doctor:
    node bin/waid.ts $cmd --json  >  a.json
    ./waid            $cmd --json  >  b.json
    diff a.json b.json
```

**Write commands** are compared by their effect: run each against a fresh copy of a fixture home,
then diff the resulting `events.jsonl` byte for byte.

Three sources of non-determinism have to be pinned, and each already has a seam on the Node side
that the Go build must mirror:

| Source | How it is pinned |
| --- | --- |
| Clock | Injected. Node has `ctx.now`; Go takes a `now time.Time`. |
| Ids | Injected generator, seeded to a fixed sequence in tests. |
| `gh` | The existing fake client, driven from a recorded fixture rather than the network. |

`testdata/golden/` holds a checked-in fixture home — a synthetic `events.jsonl` exercising every
event type, a `problems` case for each `ProblemReason`, and the six transcript fixtures. The harness
also runs against the real `C:\data\waid` (copied, never in place), because synthetic data will not
reproduce a note containing `&` or a project path with a space.

Beyond the harness, the Go suite reproduces the Node suite's approach: real temp git repos for
`git`, the binary spawned as a child process for smoke tests, fakes only for `gh` and the clock.

## 6. The `-i` picker is dropped

`-i` is **not** ported. It is superseded by the app in the following spec, and porting it would mean
building a subsystem in order to delete it.

The picker was justified as serving a scripted path, but it never did: `-i` exits 1 when stdin or
stdout is not a TTY, so an agent cannot use it and never could — an agent calls
`waid promote <key> --json`. Its only user is a human at a terminal, which is exactly who the app's
Scan tab serves, with the same keys and better context.

Keeping both would also mean two triage surfaces with opposite semantics: the picker stages marks
and writes nothing until `Enter`, while the app writes immediately with an undo stack. That is a
worse outcome than either alone, because the correct keystroke depends on which one you are in.

Consequently `src/tui/` and `specs/20260814-waid-tui-design.md`'s implementation are retired at
cutover, and **the Go CLI takes no dependencies at all** — Bubble Tea and Lipgloss arrive with the
app, not here. The 2026-08-14 spec stays in `specs/` as the record of what shipped and why.

**This opens a gap.** Between cutover and the app's Scan tab, triage returns to copying keys like
`review:DevResults/DevResults#6886` into a second command — the friction the picker existed to
remove. The gap is bounded by building Scan first in the following spec rather than by keeping dead
code alive, but it is real and should be expected rather than discovered.

## 7. Two commands the port adds

Both are additive — neither changes existing output, so the contract in §1 is unaffected.

**`waid undismiss <key>`** closes a real gap rather than serving a future spec. `undismiss` is
already a valid event that `foldEvents` handles, but nothing emits it, so dismissing a signal is
currently irreversible from the CLI: the only recovery is hand-editing the log. The app's undo model
needs the inverse too, but the command is worth adding on its own terms.

**`waid transcript <id> --json`** returns a session's turns. waid can already tell you a session
existed, what it was called, and how many prompts it took, but offers no way to see what was in it.
This is a CLI-surface gap, not an app prerequisite — the app calls `internal/sessions` directly and
would work without it. It lands here because the parser is in scope for this phase anyway, and it is
the cheapest moment to expose it. If phase 5 runs long, this is the item to cut.

## 8. Phasing

Each phase ends green on `go test ./...` and `go vet ./...`, and every phase from 2 onward extends
the differential harness rather than deferring it.

1. **Foundation.** `go.mod`, `config`, `events` (fold and append), the id generator, and the
   harness skeleton. The encoding rules in §2 are proven here, against real log lines, before
   anything writes.
2. **Declared items.** `add`, `done`, `reopen`, `note`, `list`, `show`, and `project` resolution.
   The first phase where both binaries can be diffed on writes.
3. **Sessions.** `harvest` and the cache, with the six transcript fixtures ported first so the
   parser is pinned before it is written. Then `today` and `week`.
4. **Detection.** `git`, `gh`, `repos`, `detect`, then `scan` and `loops` — sequentially first, so
   the diff proves equivalence, and only then the §4 worker pool, so the speedup is measured against
   a known-correct baseline.
5. **Remainder.** `sync`, `doctor`, `dismiss`, `promote`, `undismiss`, `transcript`, usage text.
6. **Cutover.** `go install ./cmd/waid`, delete `bin/`, `src/`, `test/`, `package.json`,
   `tsconfig.json`. Rewrite the README's install, no-build-step, and interactive-triage sections.
   Verify the `CLAUDE.md` snippet and the `/waid` skill against the Go binary.

The ordering is deliberate: the slowest, most valuable work (§4) lands only once a correctness
baseline exists to compare it against. Nothing here touches a terminal, so the port carries none of
the raw-mode risk the picker's spec had to phase around.

## 9. Risks

| Risk | Mitigation |
| --- | --- |
| Silent log corruption after cutover | §2's four encoding rules, proven in phase 1 against real lines before any write path exists. |
| Transcript parser regressions | Fixtures ported before the parser; they pin sidechain, meta, CRLF, and truncation handling exactly. |
| A live consumer breaks | `CLAUDE.md` and the `/waid` skill are verified in phase 7, against the same commands they actually issue. |
| Concurrency reorders detection | Results sorted after collection, never by completion; the harness would catch it, which is why §4 lands after a baseline. |
| Port stalls half-done | The Node tree stays runnable and installed until phase 7. There is no window without a working `waid`. |

## 10. Decisions settled during design

| Question | Decision |
| --- | --- |
| Port the CLI, or keep Node and bridge? | Port. A bridge means mirrored types, a JSON contract, and 200ms per read, all of which the port deletes. |
| Go or Rust? | Go. Bubble Tea's Elm architecture is what the picker already implements by hand; Ratatui buys nothing here and iterates slower. |
| Port first, or build the app first? | Port first. Building the app against JSON means ~400 lines of client, mirrors, and contract tests that the port would delete. |
| Rewrite the event log format? | No. It holds real data and its append-only design is sound. |
| Keep `cache/` compatible? | No. It is derived and disposable; a stale cache is rebuilt. |
| Keep `-i`? | No. It requires a TTY, so it never served scripts; the app's Scan tab supersedes it, and two triage surfaces with opposite semantics is worse than one. |
| Does the Go CLI take dependencies? | None. Bubble Tea arrives with the app, not the port. |
| Where do `undismiss` and `transcript` land? | In the port. `undismiss` closes a real gap; `transcript` is cheap while the parser is in scope. |
| How is the port verified? | Differential diffing of both binaries against the same data, including the real log. |
| Does the port change behaviour? | Only detection's concurrency, and only after the sequential version is proven equivalent. |
