---
name: waid
description: Wrap up a session, triage detected open loops, or review the week with the `waid` CLI. Use when the user asks to wrap up, triage their loops or signals, or review what they got done.
user-invocable: true
argument-hint: "wrap up | triage | review"
---

# waid

`waid` is a local open-loop tracker over an append-only event log. It holds **declared items** —
things the user or an agent recorded — and surfaces **detected signals** from git and GitHub that
nobody has recorded yet.

This skill covers the three flows that need judgment. Pick the one named in the argument; if none
was given, ask which of the three the user wants rather than guessing.

- **Wrap up** — review this session and propose items for anything left unfinished.
- **Triage** — walk the detected signals, promoting or dismissing each.
- **Review** — read back the week and propose closures for items that are evidently finished.

## The rule that governs all three flows

**Never run a command that writes without the user's explicit approval, in that turn, for that
command.**

| Read-only — run freely | Writes — needs approval |
| --- | --- |
| `loops` `scan` `list` `show` `today` `week` `transcript` `doctor` `sync` | `add` `done` `reopen` `move` `heading` `tag` `note` `promote` `dismiss` `undismiss` |

`promote` is a write: it appends an `add` and a `dismiss`. `undismiss` is one as well, and only
accepts a key that is dismissed now. `sync` only rebuilds the disposable cache, so it counts as a
read.

Every flow has the same shape: gather with read-only commands, present a **numbered** proposal,
stop, and run only what comes back approved. Silence is not approval, and neither is a previous
turn's approval — approval covers the list in front of the user and nothing else. If the user
approves a subset ("1 and 3"), run exactly that subset and say what you skipped. If they correct a
title, parent or tag, use their version verbatim.

## Wrap up

1. `waid loops --origin . --json` — see what is already tracked for work that came from this repo,
   so you never propose a duplicate of an open item.
2. Read back over the session for work that **outlives it**: a bug found and not fixed, work
   deliberately deferred, something blocked on another person, a follow-up promised out loud. Do
   **not** propose items for routine steps completed inside the session — that noise is exactly
   what the log is meant to stay free of.
3. Check the open items from step 1 in the other direction: anything this session actually
   finished is a proposed `waid done`.
4. Present both halves as one numbered list. For each entry give the exact command you would run
   and the one-line reason from the session that justifies it. Say plainly when there is nothing
   worth recording — an empty wrap-up is a good outcome, not a failure.
5. Wait. Then run the approved commands with `--json` and report the ids that came back.

## Triage

1. `waid scan --json`. Add `-p .` when the user only wants the repo they are in.
2. If `notes` is non-empty, mention it once — usually `gh` being unavailable, which means the
   GitHub signals are missing but the git ones are real — then carry on with what did come back.
3. Signals arrive ranked (`review`, `pr`, `ahead`, `dirty`, then oldest first). Walk them in that
   order and give each exactly one recommendation: **promote**, **dismiss**, or **leave**.
   - `review` blocks another person, so it is a promote unless the user says otherwise.
   - `pr` you authored is a promote when it is stale, a leave when it just opened.
   - `ahead` and `dirty` in a scratch or experiment repo are usually dismiss; in a project with
     tracked work they are worth promoting.
   - Leave means the signal stays visible and nothing is written — the right answer when you have
     no basis to judge.
4. Present the whole list at once, numbered, each row carrying the signal's key, its `detail`, and
   your recommendation. Do not walk them one at a time; the user should see the shape of the pile
   before deciding.
5. Wait. Then run `waid promote <key> --json` / `waid dismiss <key> --json` for the approved rows
   only, and report the ids `promote` returned.
6. `waid scan --json` again if the user wants to see what is left.

## Review

1. `waid week --json` — or `waid week --last --json` when the user means the week just gone.
2. `waid loops --json` for what is still open across every project.
3. Summarize the week **first**: projects busiest first, the session titles under each, and what
   was closed. This is the part the user asked for; the proposals come after.
4. Then cross-read the two. For each open item, look for evidence in the week that it is done — a
   session title naming it, work in its project, a closure alongside it. Run `waid show <id>
   --json` when an item's history would settle it.
5. Propose `waid done <id>` **only** where you can name the evidence, and name it in the proposal:
   `ab12 "fix flaky retry test" — session "Fix flaky retry test" in C:\dev\waid on Tuesday`.
   Where you are unsure, propose `waid note <id> "<what you saw>"` instead, or leave the item
   alone. A wrong closure is worse than a stale item, because the item stops being visible.
6. Wait. Then run the approved commands and report the results.

## Conventions

- **Always pass `--json`** and read the parsed output — never scrape the human rendering.
- **`-p` must resolve.** On `add` and `move` it names a **parent item**: a substring matching
  exactly one item's title. A substring matching none or several exits 1 and lists the candidates;
  pick from that list and rerun rather than guessing. On `add` it also still takes an absolute path,
  or `.` for the current directory, which records where the work came from and leaves the item at the
  top level — that is the form to use when the work belongs to a repo rather than under an existing
  item. `move` takes a parent only. On `scan` it still names a repo path.
- **`--parent <id>`** on `add` and `move` names the parent by id rather than by title, and cannot be
  ambiguous. Prefer it whenever the parent's id is already in hand; it replaces `-p`, not joins it.
- **`--origin <fragment>`** narrows `list` and `loops` to items recorded against a path, resolving
  the way project paths always have. It is how you ask "what is tracked for this repo".
- **Signal keys contain `\` and `#`** — `dirty:C:\dev\waid`, `review:owner/repo#12`. Pass the key
  exactly as it reads after JSON parsing (the `\\` in the raw JSON is one backslash), quoted, so
  the shell does not eat it.
- **Items nest, and a parent will not close while anything under it is open.** `waid done` takes
  several ids, so when a heading and its children are all finished, propose one `waid done <child>
  <child> <parent>` rather than a run of separate closures.
- **Exit codes:** 1 is a user error — read the message on stderr and fix the argument. 2 is a bug
  in `waid` — report it to the user and stop; do not retry.
- **Titles** are one line saying what needs doing, with no `TODO:` prefix and no date. Add
  `--tag bug` for a defect and `--waiting-on <who>` when the block is a person, which sets the
  item's status to `waiting`.
- **`waid ui` is the user's, not yours.** The app owns the terminal for as long as it runs, and
  with stdout redirected it exits 1 rather than falling back to text, so never launch it. Point
  the user at it when they would rather do a pass themselves than read a numbered list: its Repos
  tab promotes and dismisses signals with `p` and `d`, and its Loops tab closes, retitles and
  refiles items.
- If `waid` is not on `PATH`, every command works as `go run ./cmd/waid <command>` from
  `C:\dev\waid`.
