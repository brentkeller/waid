# waid — Interactive picker (`-i`) — Design

**Date:** 2026-08-14
**Status:** Approved design, pending implementation plan

## Purpose

`waid loops` and `waid scan` print a list of open loops and then tell you to retype a key:

```
  waid promote <key> to track · waid dismiss <key> to hide
```

The keys carry `\` and `#` (`dirty:C:\dev\waid`, `review:waid/waid#12`), so acting on a signal
means copying an awkward string into a second command, once per signal. Triage is the one thing
waid asks you to do regularly, and it is the one thing that costs the most typing.

This adds an opt-in interactive mode to those two commands: `waid loops -i` and `waid scan -i`
render the same list with a cursor on it, let you mark rows, and apply the marks in one keystroke.

## Non-goals

- **Not a full TUI app.** No tabs, no view switcher, no long-running process. `-i` is a picker
  that opens, takes a decision, applies it, and exits. `today`, `week`, `list`, and `show` stay
  print-and-exit.
- **Not a new command.** No `waid ui`, no `waid triage`. The flag lives on the commands that
  already produce the list.
- **No runtime dependencies.** No `ink`, no `blessed`, no `chalk`. Node's `setRawMode` and raw
  escape codes only, matching the project's stdlib-only rule and its no-build-step constraint.
- **Not a mouse target.** Keyboard only.

## 1. Shape

`-i` (long form `--interactive`) is opt-in. It is never enabled automatically, not even on a TTY,
because the non-interactive output is what agents and scripts consume.

Interaction is **mark-then-apply**: keys stage marks against rows, nothing is written until you
press `Enter`, which applies every mark and exits. `q` or `Esc` discards the marks and writes
nothing.

The declared items and the detected signals live in one list. In `waid loops -i` the cursor moves
through both sections, and the keys that apply depend on the row under it.

## 2. Architecture

The load-bearing constraint is that **the picker never touches the event log and never writes to
the terminal directly**. Everything except two small files is pure and testable without a TTY.

```
loops.run(ctx) ──► LoopsResult ──► loops.rows(data, ctx) ──► PickRow[]
                                                                 │
                                              ┌──────────────────┴──────────────────┐
                                              │  picker loop (src/tui/run.ts)       │
                                              │    term.readKey ──► keys.decode     │
                                              │    reduce(state, key) ──► PickState │
                                              │    paint(state) ──► term.write      │
                                              └──────────────────┬──────────────────┘
                                                                 │  Plan
                                                        applyPlan(cfg, plan)
                                                            appendEvent ×N
```

### 2.1 `rows()` on `CommandModule`

`CommandModule` gains one optional member beside `run` and `render`:

```ts
export type CommandModule<D> = {
  run: (ctx: Ctx) => Promise<D>;
  render(data: D, ctx: Ctx): string;
  /** Selectable rows for `-i`. Absent means the command does not support the flag. */
  rows?(data: D, ctx: Ctx): PickRow[];
  needsSessions?: boolean;
};
```

`loops` and `scan` implement it. Every other command omits it, and `-i` on them is a user error.

The mapping lives on the module, next to the `render` it mirrors, so the printed layout and the
interactive layout cannot drift apart. `rows()` builds its text with the same helpers `render`
uses — `itemLine` from `list.ts`, `detectedSection` from `scan.ts` — so `-i` looks like the
command with a cursor on it rather than a second design.

### 2.2 Row model

```ts
export type PickRow =
  | { kind: 'heading'; text: string }
  | { kind: 'signal'; id: string; text: string; signal: Signal }
  | { kind: 'item'; id: string; text: string; item: Item };
```

Headings are rows so the interactive layout reproduces the printed one exactly — `OPEN LOOPS`,
the project headings, the blank separators, `DETECTED`. The cursor skips them. `id` is unique
within a screen and is what a mark is keyed by (a signal's key, an item's id).

### 2.3 Marks and the plan

```ts
export type Mark =
  | { action: 'promote'; title: string }      // seeded from signal.title
  | { action: 'dismiss' }
  | { action: 'done' }
  | { action: 'waiting'; waitingOn: string };

export type Plan = { row: PickRow; mark: Mark }[];   // in row order
```

Two of the four marks carry an editable text field. That is what makes the inline editor one
feature rather than two: `e` edits *the mark's text field*, whichever it is.

### 2.4 Modules

| File | Job | Purity |
| --- | --- | --- |
| `src/tui/keys.ts` | bytes → `Key` union | pure |
| `src/tui/pick.ts` | `PickState`, `reduce(state, key)` | pure |
| `src/tui/paint.ts` | `PickState` → `string[]` | pure |
| `src/tui/apply.ts` | `Plan` → `appendEvent` calls | writes the log |
| `src/tui/term.ts` | raw mode, alt screen, size, `SIGWINCH`, restore | I/O only |
| `src/tui/run.ts` | the loop wiring the five together | glue |

`PickState` is `{ rows, cursor, marks, editing, viewport, hint }`. `reduce` takes no config, no
clock, and no I/O, so the entire interaction is exercised by `node --test` without a pty.

`term.ts` is the only file that touches `process.stdin` / `process.stdout`. It is kept small
deliberately: it is the part tests cannot reach, so it must be reviewable by eye.

## 3. Keys

| Key | On a signal row | On an item row |
| --- | --- | --- |
| `p` | mark promote | inert |
| `d` | mark dismiss | inert |
| `x` | inert | mark done |
| `w` | inert | mark waiting, then open the editor for *who* |
| `e` | edit the mark's text field | edit the mark's text field |
| `u` | unmark | unmark |

| Key | Effect |
| --- | --- |
| `j` / `k` / `↑` / `↓` | move the cursor, skipping headings; clamps at the ends |
| `g` / `G` | first / last selectable row |
| `?` | toggle the full legend |
| `Enter` | apply every mark and exit |
| `q` / `Esc` | cancel, write nothing, exit 0 |
| `Ctrl-C` | cancel, write nothing, exit 130 |

Pressing the same action key twice unmarks the row. Pressing a different action key switches the
mark. A key that does not apply to the row under the cursor is inert and puts a transient hint in
the footer (`p only applies to detected signals`) rather than doing nothing silently.

While the editor is open, keys go to the line editor: `Enter` commits the field and `Esc` reverts
it. Neither reaches the plan, so it is impossible to apply a batch by trying to finish a title.

## 4. Rendering

```
DETECTED  (4 shown, 1 dismissed)

  P  review:waid/waid#12       awaiting your review · 2d
> P  pr:waid/waid#9            open, draft · 5d
  D  ahead:C:\dev\waid:cli-ui  3 unpushed · 1h
     dirty:C:\dev\waid         6 files · 20m

  2 to promote, 1 to dismiss
  p promote · d dismiss · u unmark · enter apply · q cancel · ? keys
```

- A two-column gutter carries the cursor (`>`) and the mark letter, so marked rows are legible
  without colour. Colour, if used at all, is additive only — the screen must read correctly with
  every escape stripped.
- The list scrolls. With several active repos the rows outrun the terminal, so `PickState` holds
  a viewport that follows the cursor. `SIGWINCH` recomputes it and repaints.
- The footer shows the mark counts and a short legend; `?` expands it to the full key table.
- Degradation notes that `loops` and `scan` already produce (an unavailable `gh`, a failed
  detection pass) render beneath the list, as they do today.

## 5. Applying

`applyPlan(cfg, plan)` is the only writer. It walks the plan in row order and appends events
through the existing `appendEvent`:

| Mark | Events |
| --- | --- |
| `promote` | `add` (status `open`, tag `promoted`, the mark's title, the signal's project) then `dismiss` |
| `dismiss` | `dismiss` |
| `done` | `close` |
| `waiting` | `update` with status `waiting` and `waitingOn` |

To guarantee a promoted item is identical however it was created, `promote.ts` splits: the body
becomes `promoteSignal(cfg, state, signal, title)`, and the CLI command keeps its "re-detect to
resolve the key string" step before calling it. The picker calls `promoteSignal` directly with the
`Signal` captured when the screen opened.

**The picker does not re-detect on apply.** A signal that disappeared while you were deciding —
a PR merged, a worktree committed — still promotes. You made the decision against what you were
shown, and re-detecting would turn a race into a spurious failure. This is a deliberate difference
from `waid promote <key>`, which must re-detect because a bare key string carries no title or
project.

After restoring the terminal, applying prints the same receipts the individual commands print —
`promoted 7K3M  6 uncommitted files in waid`, `dismissed pr:waid/waid#9` — one line per event, so
an interactive session leaves the same trace in the scrollback as doing it by hand.

## 6. Guards and failure

Three conditions are user errors (exit 1) rather than silent fallbacks, because a scripted
`waid scan -i` that quietly printed and exited 0 would look like it had worked:

| Condition | Message |
| --- | --- |
| `-i` with `--json` | `-i cannot be combined with --json` |
| stdin or stdout is not a TTY | `-i requires an interactive terminal` |
| `-i` on a command with no `rows()` | `list does not support -i` |

One condition is not an error: **nothing selectable**. `waid scan -i` with zero signals prints
exactly what `waid scan` prints and exits 0 without ever entering raw mode.

**Terminal restore** is the failure mode that actually hurts a hand-rolled TUI, so it gets belt
and braces: a `finally` around the loop plus a `process.on('exit')` handler that re-emits
show-cursor and leave-alt-screen. A throw mid-render must never strand the user in raw mode with
a hidden cursor.

## 7. Testing

| File | Covers |
| --- | --- |
| `test/tui-reduce.test.ts` | cursor clamping and heading-skipping, toggle vs. switch, `u`, edit mode capturing keys, viewport following the cursor |
| `test/tui-rows.test.ts` | `loops.rows()` / `scan.rows()` over fixture results — heading placement, unique ids, signals carrying their key |
| `test/tui-paint.test.ts` | gutter and footer for a marked/unmarked/edited state; correctness with escapes stripped |
| `test/tui-apply.test.ts` | `applyPlan` against a temp home via `test/helpers.ts`, asserting exact `events.jsonl` lines, including that promote uses the *edited* title |
| `test/tui-keys.test.ts` | the decoder alone: `\x1b[A` → `up`, `\x03` → `ctrl-c`, printable → `char` |
| `test/cli.test.ts` (extend) | the three guards |
| `test/smoke.test.ts` (extend) | `waid scan -i` with piped stdio exits 1 in a real child process |

That leaves raw-mode toggling and the alt-screen escapes as the only untested code — a handful of
lines confined to `src/tui/term.ts`, which is why they live in a file of their own.

## 8. Files touched

**New:** `src/tui/{keys,pick,paint,apply,term,run}.ts` and the five new test files above.
`test/cli.test.ts` and `test/smoke.test.ts` are extended rather than replaced.

**Modified:**

| File | Change |
| --- | --- |
| `src/types.ts` | `PickRow`, `Mark`, `Plan`, `PickState`; `rows?` on `CommandModule` |
| `src/args.ts` | `i` → `interactive` alias; `interactive` in `BOOLEAN_FLAGS` |
| `src/cli.ts` | the `-i` branch, the three guards, usage text |
| `src/commands/loops.ts` | `rows` |
| `src/commands/scan.ts` | `rows` |
| `src/commands/promote.ts` | extract `promoteSignal` |
| `README.md` | document `-i` |

## 9. Phasing

Each phase ends green on `npm test` and `npm run typecheck`.

1. **Rows.** `PickRow` types and `rows()` on `loops` / `scan`, with tests. Pure data; nothing
   interactive exists yet.
2. **State machine.** `reduce` and `paint`, driven entirely by tests. Still nothing on screen —
   the whole interaction is proven before an escape code is written.
3. **Writes.** `promoteSignal` extraction, `applyPlan`, event-log assertions against a temp home.
4. **Terminal.** `keys.ts`, `term.ts`, `run.ts`, the `-i` wiring and guards, the smoke test, the
   README. The first phase in which a key can be pressed.

The ordering is the point: the code that can strand a terminal in raw mode lands last, on top of
three phases that are already covered.

## 10. Decisions settled during design

| Question | Decision |
| --- | --- |
| New command or flag? | Flag on the commands that already build the list. |
| Act immediately or batch? | Mark then apply. Nothing is written until `Enter`. |
| Are declared items actionable in `loops -i`? | Yes — one list, kind-aware keys. |
| Can a promoted title be edited? | Yes, on demand, via `e` on a marked row. |
| Auto-enable on a TTY? | No. Opt-in only. |
| Fall back or fail without a TTY? | Fail, exit 1. |
| Re-detect on apply? | No. Apply against what was shown. |
