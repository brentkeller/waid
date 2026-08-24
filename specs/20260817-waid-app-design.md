# waid — The app (`waid ui`) — Design

**Date:** 2026-08-17
**Status:** Approved design, pending implementation plan
**Depends on:** `20260817-waid-go-port-design.md`. Nothing here is buildable until the port lands.

## Purpose

`waid today`, `waid week`, and `waid loops` print a lot at once, and reading them means scrolling
back through a wall of text that cannot be filtered, folded, or acted on. The information is right;
the surface is wrong for it.

This adds a long-running terminal app over the same data: three tabs, collapsible grouping, a
transcript preview, and triage that writes as you go.

It also replaces the `-i` picker, which the port drops. Between cutover and this app's Scan tab
there is no interactive triage at all, which is why §9 builds Scan first.

Bubble Tea and Lipgloss enter the dependency set here. The port takes none.

## Non-goals

- **Not a replacement for the CLI.** Every command keeps working and keeps its output. Bare `waid`
  still prints loops, because the `CLAUDE.md` snippet and the `/waid` skill depend on it.
- **Not a mouse target.** Keyboard only, as the picker already established.
- **Not a session viewer.** The transcript pane shows enough to recognise a session; reading one
  properly is what `R` hands off to Claude for.
- **No new detection.** Scan shows exactly what `waid scan` detects. Sessions are history, reviewed
  in Review, not loops to triage.

## 1. Shape

Three tabs, switched with `1`/`2`/`3` or `tab`. Tabs rather than lazygit's fixed panes because the
three sections are modes of a workday, not a drill-down hierarchy — there is no sense in which Scan
is "inside" Loops. But *within* Scan and Review there is a genuine hierarchy (project → children →
detail), so each tab is a master/detail split. Winget-tui's chrome, lazygit's detail pane.

```
╭───────────╮
│ Loops  12 │ Scan  8 │ Review │                          ⟳ scanning  ████████░░░░  5/9 repos
┴───────────┴─────────┴────────┴──────────────────────────────────────────────────────────────
```

The right slot is the only place slow work is ever visible. Detection is sub-second once the port's
worker pool lands, but sub-second is still a freeze, so it stays off the update loop.

### 1.1 Loops

```
 ‹open›  waiting  done  all                                       12 items · 3 projects
──────────────────────────────────────────────────────────────────────────────────────────────
 C:\dev\dr\devresults\devresults
   nktt   open     Background workers speak the requester's language   [promoted]        2d
 ▸ sga9   open     Design template + args persistence for localiz…                      19m
   4h2k   waiting  Deploy blocked until the migration is approved      @maria            5d

 C:\dev\waid
   9xz1   open     TUI design spike                                                      1h
──────────────────────────────────────────────────────────────────────────────────────────────
 sga9 · devresults · open · created 19m ago

 Design template + args persistence for localized persisted strings
 (LastFailureMessage, PublishedAwardReportingPeriods.Errors)

 notes
   19m  needs to survive a round trip through the queue
──────────────────────────────────────────────────────────────────────────────────────────────
 x done · w waiting · e edit · n note · a add · / filter · s status · ? keys
```

The status row is a segmented toggle rather than a filter dialog: `s` cycles it, `1`–`4` jump
directly. Filtering is client-side over the loaded state, so it is instant and issues no work.

The detail pane is `show`'s data rendered live, and `p` collapses it when density matters more.

### 1.2 Scan

```
 ‹all›  review  pr  ahead  dirty                      8 signals · 1 dismissed · ⏱ 2m ago
──────────────────────────────────────────────────────────────────────────────────────────────
 ▾ DevResults/DevResults                                                          2 signals
     review  #6886   Add SharedDashboards role to non-owners     @copilot         15w
   ▸ pr      #5862   Activity Compendium prototype               compendium-spike  1y

 ▾ C:\dev\waid                                                                    1 signal
     ahead   tui     1 commit ahead                              tui               4m

 ▸ C:\dev\bkc-my                                                                  3 signals
──────────────────────────────────────────────────────────────────────────────────────────────
 ✓ dismissed pr:DevResults/devresults-react#18                              u undo
 p promote · d dismiss · o open in browser · r refresh · / filter · ? keys
```

Rows leave the list the moment you act on them, and the receipt line is the undo affordance.

### 1.3 Review

```
 ‹today›  week  last week                     17 sessions · 94 prompts · 5 items closed
──────────────────────────────────────────────────────────────────────────────────────────────
 ▾ C:\dev\waid                      6 sessions · 41 prompts  │ Add terminal I/O and the picker
     14:22  Wire -i into the CLI          12 prompts         │ 15:04–15:42 · 9 prompts · tui
   ▸ 15:04  Add terminal I/O and the…      9 prompts         │ 017516d6-7c60-4081-961e-f2120…
     15:58  Show PR title and branch       4 prompts         │
                                                             │ ▸ you
 ▸ C:\dev\dr\devresults\devresults  3 sessions · 28 prompts  │   Let's add the terminal I/O
                                                             │   layer now — raw mode, alt
 ▸ C:\dev\bkc-my                    8 sessions · 25 prompts  │   screen, SIGWINCH.
                                                             │
                                                             │ ▸ claude
                                                             │   I'll start with term.ts, the
                                                             │   only file touching stdin…
──────────────────────────────────────────────────────────────────────────────────────────────
 space preview · R resume in claude · o open repo · y copy id · ? keys
```

Projects are collapsed by default with a session count, which is the whole reason this reads better
than `waid today` — a busy day is three lines until you ask for more.

## 2. One tree, two tabs

Scan and Review render the same component: a collapsible project → children list with counts on the
fold, a cursor that skips headings, and identical expand/preview keys. Only the child row and the
action set differ.

This is the single most important structural decision in the design. Two hand-written trees would
drift within a month, and the drift would be invisible — each would look correct in isolation.

```go
type Tree[T any] struct {
    Groups   []Group[T]
    Cursor   int
    Expanded map[string]bool
    Render   func(T, bool) string   // row, focused
}
```

## 3. Writes happen immediately

Actions write as you press them. The row leaves the list, a receipt appears in the footer, and `u`
undoes it. There is no confirm step — in an app you live in, "press Enter to commit" is a modal
interruption repeated all day.

The event log makes this safe in a way it would not be over a mutable store. Undo is a bounded stack
of **inverse commands**, not a state snapshot:

| Action | Inverse |
| --- | --- |
| `dismiss <key>` | `undismiss <key>` |
| `done <id>` | `reopen <id>` |
| `promote <key>` | `done <newid>` + `undismiss <key>` |
| `waiting <id> <who>` | restore the prior status and `waitingOn` |

Undo therefore appends rather than erases, leaving the reversal visible in the log. For an
append-only design that is the correct behaviour, not a limitation.

Promote is the one action carrying text — an item title read for weeks afterwards. Under immediate
writes it fires with the signal's own title, and the footer offers `e` to rename alongside `u`, so
the common case costs one key and the correction stays one key away.

### 3.1 Receipts survive the alt screen

The picker printed one receipt line per write *after* restoring the terminal, so a triage pass left
its trace in real scrollback. An app on the alt screen would lose that entirely: quit, and every
decision you made vanishes with the screen.

So quitting prints a summary to the restored terminal — the same receipt lines, in the order they
were written:

```
promoted 7k3m  Activity Compendium prototype
dismissed ahead:C:\dev\waid:tui
closed   sga9  Design template + args persistence…
```

Nothing is printed when nothing was written, so reading the week stays silent.

## 4. Keys

Global, on every tab:

| Key | Effect |
| --- | --- |
| `1` `2` `3` / `tab` | Switch tab |
| `j` `k` `↑` `↓` | Move the cursor, skipping headings |
| `g` / `G` | First / last row |
| `enter` | Expand or collapse the fold under the cursor |
| `h` `l` `←` `→` | Collapse / expand the fold under the cursor |
| `/` | Filter; `esc` clears |
| `a` | Add an item, from any tab |
| `r` | Refresh the current tab |
| `u` | Undo the last write |
| `?` | Key table |
| `q` / `ctrl-c` | Quit |

Per tab:

| Tab | Keys |
| --- | --- |
| Loops | `x` done · `w` waiting · `e` edit title · `n` note · `s` cycle status filter · `p` toggle detail |
| Scan | `p` promote · `d` dismiss · `o` open in browser |
| Review | `space` preview · `R` resume in claude · `o` open repo · `y` copy session id |

A key that does not apply to the row under the cursor is inert and says why in the footer, matching
the rule the picker already set.

## 5. Resume, and the responsive split

**`R` suspends the app rather than embedding Claude.** Leave the alt screen, run
`claude --resume <id>` with inherited stdio, re-enter when it exits. Bubble Tea's `ExecProcess`
does exactly this. Hosting an interactive Claude inside the update loop would mean proxying its
stdio and its own terminal handling; suspending is a few lines and behaves natively — including
`ctrl-c` going where the user expects.

**The preview pane is width-dependent.** Below 120 columns a side-by-side split leaves the
transcript a ~30-column gutter, which is unreadable for prose. At or above 120 it splits; below, the
preview is a full-width overlay toggled with `space`. One measurement in the layout package, not a
setting.

## 6. Architecture

```
internal/tui/
  app.go          root model: tabs, focus, global keys, undo stack
  loops.go        items, status filter, detail pane
  scan.go         signals, triage
  review.go       sessions, preview, resume
  tree.go         the shared collapsible tree (§2)
  theme.go        one lipgloss palette
  layout.go       the width rule (§5)
  receipt.go      the write log replayed to stdout on quit (§3.1)
```

Every read is a `tea.Cmd`: it runs off the update loop and returns a `tea.Msg`. `Update` never waits,
the model keeps serving the last good data, and the spinner is a field rather than a thread.

```
Update(msg) ──► (model, tea.Cmd) ──► goroutine: detect.Scan(cfg, now)
     ▲                                            │
     └──────────────── scanLoadedMsg{signals} ────┘
```

Because the CLI is Go by this point, those commands call `internal/detect`, `internal/events`, and
`internal/sessions` **directly** — typed structs, no subprocess, no JSON, no mirrored types. That is
what the port bought, and it is why this spec is short.

The domain packages stay unaware the app exists. Nothing in `internal/tui` is imported by anything
outside it, and no domain package gains a terminal concern.

## 7. Errors and degradation

The app never renders an error as a crash. Detection routinely degrades — an unauthenticated `gh`
is the common case, and the CLI already reports it as a note rather than a failure.

| Condition | Treatment |
| --- | --- |
| Degradation notes | Dim strip below the list, wording unchanged from the CLI |
| A failed refresh | Last good data stays; the tab bar shows the failure and the age of what is shown |
| Not a TTY | Exit 1, `waid ui requires an interactive terminal`; the app is never a fallback for a pipe |
| Panic mid-render | Deferred restore leaves the alt screen and shows the cursor before the stack prints |

That last row is the failure mode that actually hurts a hand-rolled terminal app, and it is the one
the picker's spec already singled out for belt and braces. Bubble Tea handles it, but the smoke test
proving a stranded terminal cannot happen is still worth keeping.

## 8. Testing

`Update` is pure in exactly the way `reduce` is, so the interaction is exercised without a terminal.

| Target | Covers |
| --- | --- |
| `app` | Tab switching, focus, global keys, the undo stack's inverses |
| `tree` | Cursor clamping, heading skipping, expand/collapse, counts on the fold |
| `loops` / `scan` / `review` | Per-tab keys, inert-key hints, filter behaviour |
| `layout` | The 120-column split rule at boundary widths |
| golden views | `teatest` snapshots at 80 and 140 columns |
| smoke | The binary spawned with piped stdio exits 1 |

Undo gets its own assertions against a temp home, since it is the one place an interaction produces
log writes: each inverse is asserted as exact `events.jsonl` lines, including that undoing a promote
closes the item *and* undismisses the key.

## 9. Phasing

Each phase ends green on `go test ./...` and is usable on its own.

1. **Chrome and tree.** `app.go`, `tree.go`, `theme.go`, `layout.go`, tab switching, and the key
   table. Driven by tests; nothing loads real data yet.
2. **Scan.** Signals, async refresh with the spinner, triage, the undo stack, and the exit receipts.
3. **Review.** Sessions, the collapsed-by-project tree, the preview pane, then `R`.
4. **Loops.** Items, the status toggle, the detail pane, `a`, `e`, `n`.

Scan is first because the port drops `-i` and leaves no interactive triage behind it; every phase
before Scan is time spent copying signal keys by hand. That inverts the safer ordering — Review has
no writes and would prove the tree without being able to corrupt anything — so phase 1 carries the
weight instead, testing the tree exhaustively against fixtures before phase 2 points it at the log.

## 10. Decisions settled during design

| Question | Decision |
| --- | --- |
| Panes like lazygit, or tabs like winget-tui? | Tabs at the top, master/detail within each. The sections are modes, not a hierarchy. |
| Do Scan and Review share a component? | Yes, one tree. Two would drift invisibly. |
| Batch writes, or write immediately? | Immediately, with an undo stack of inverse commands. |
| How is the picker's scrollback trace kept? | Quitting replays the session's writes to the restored terminal. |
| Does promote prompt for a title? | No. It fires with the signal's title; `e` renames. |
| Do agent sessions appear in Scan? | No. Scan is exactly `waid scan`; sessions are history, shown in Review. |
| Embed Claude for resume, or suspend? | Suspend via `ExecProcess`. |
| Side pane or overlay for transcripts? | Both, chosen by width at 120 columns. |
| Subprocess or direct calls? | Direct. The port removed the boundary this design originally needed. |
| Does `-i` survive? | No. Dropped in the port; this app supersedes it, and Scan is built first to close the gap. |
| Does bare `waid` open the app? | No. It keeps printing loops; the app is `waid ui`. |
