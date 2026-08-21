# waid — Loops become a tree — Design

**Date:** 2026-08-21
**Status:** Approved design, pending implementation plan
**Depends on:** `20260817-waid-app-design.md`. This reshapes the Loops tab that spec built.

## Purpose

`waid` files an item under a project, and a project is an absolute path. That works while an item is
a thing that happened in a repo, and stops working the moment it is not. A one-off task belongs
nowhere. A product spanning several repos splits. Three worktrees of one repo become three projects,
because `repos.Discover` treats each `.git` entry as a root and `sessions.harvest` records the raw
`cwd`.

The path is the wrong key because it answers a different question. There are two features here and
they have been sharing a field:

1. **What have I been doing** — the Repos and Agents tabs. Path-keyed, correctly and permanently. A
   session ran in a directory; a dirty worktree is at a path. Nothing about this changes.
2. **What do I owe** — Loops. A task manager, whose grouping should be thematic and hand-made:
   `DevResults` → `Move bulk imports to a background worker` → the individual findings, PRs and
   reviews that make it up.

Promotion is the one-way door between them. Once a signal becomes a loop it no longer belongs to the
repo it came from; the repo becomes provenance.

This spec makes Loops a tree of items, and retires the path as a grouping key.

## Non-goals

- **Not a change to detection.** Repos and Agents keep grouping by path, and `today` and `week` keep
  rolling *sessions* up by path. Sessions are not items and were never in this hierarchy.
- **Not a new store.** The event log stays. See §2.
- **Not tag-driven views.** Tags already exist and the tree does not change them. Grouping by tag
  instead of by parent — priority lanes, work types — is a later piece of work.
- **Not a rewrite of the log.** Every line already written keeps folding correctly.
- **No project entity.** Areas and projects are not a new kind of thing; see §1.

## 1. Everything is an item

An area, a project and a task are the same shape: an item with a parent. `DevResults` is an item.
`Move bulk imports to a background worker` is an item whose parent is `DevResults`. The N+1 fix is an
item whose parent is that.

The alternative was a real project entity with its own id, lifecycle and commands. It buys empty
projects, cheap renames and project-level completion — and costs a second entity, a second set of
commands, and a second thing every feature has to be taught about. The outliner buys all of that for
free instead: `done`, `note`, `tag`, `show`, filter and undo already work on items, so they work at
every level of the tree the day the parent pointer lands.

Depth is unlimited. A project that grows sub-projects is a normal thing to want, and there is no
level at which refusing to nest would be principled rather than arbitrary.

## 2. The data model

The store does not change. It was worth re-asking, because a hierarchy is the kind of thing a
database is for, and the answer was still the log:

- `C:\data\waid` is a git repository. The log is version-controlled, diffable and mergeable today. A
  SQLite file there is an opaque blob.
- The log is deliberately hand-editable — `store.go` accepts CRLF precisely because it gets edited by
  hand.
- Single-line appends are lock-free, which is what makes parallel agents sharing the log safe.
- Undo, `today`, `week` and `doctor` read history rather than current state, so an events table would
  have to exist anyway.
- The dependency set is Bubble Tea and Lipgloss. SQLite means cgo, or a very large pure-Go tree.

At 38 lines the fold is not a bottleneck, and it would not be at 38,000. Nothing the tree needs is
SQL-shaped: a recursive CTE against twenty lines of Go over a map. Cycle prevention is app code
either way — foreign keys give parent-exists, not acyclicity.

Two fields change:

```go
type Item struct {
    Id        string
    Title     string
    Status    Status
    WaitingOn *string
    Parent    *string  `json:"parent"`  // id of the parent item; nil is top level
    Origin    *string  `json:"origin"`  // was Project: provenance, never a grouping key
    Session   *string
    Tags      []string
    Notes     []Note
    Created, Updated string
}
```

`Origin` holds the same value `Project` held — the absolute path the item came from. It is demoted,
not deleted: it stays queryable as `--origin`, which is better than burying the path in note text
where nothing can filter on it.

Events follow the shapes already in `events/types.go`:

- `AddEvent` gains `Parent`, and its `Project` field becomes `Origin` (JSON key `origin`).
- `ParentEvent` is added, mirroring `ProjectEvent` exactly and for the reason that shape's own doc
  comment gives: the parent is *always* written, so `null` can mean "move to top level" — which an
  `omitempty` `UpdateEvent` cannot express.
- `ProjectEvent` is deleted. Nothing will emit one again.

### 2.1 Reading what is already written

`applyToItem` reads fields by key presence, so compatibility is two lines: prefer `record["origin"]`,
fall back to `record["project"]`. Same in the `add` branch. No dual-write, no rewrite, no version
gate.

## 3. The fold stays line-oriented

`Fold` stores `Parent` verbatim and validates nothing. It is pure and per-line, cannot see the whole
graph without becoming something else, and is the wrong place to discover a cycle.

A new `internal/tree` package builds the forest from folded items and owns every question that needs
the whole set:

- An item whose parent id does not exist is treated as top level and reported.
- An item whose ancestor chain loops is treated as top level and reported.

Both become new `ProblemReason` values surfaced by `doctor`. The rule is that a corrupt log still
renders — the tree degrades to a flat list rather than failing to draw.

`State` gains an index built once per fold: `map[string]int` plus each item's children, behind
`State.Find`, `State.Children` and `State.Ancestors`. `Find` is a linear scan today, and the tree
work hammers it — cycle checks, close guards, ancestor walks, filter-keeps-ancestors. The index is
derived rather than stored, so `State` encodes to the same JSON it does now.

`internal/project` splits rather than dies. The path helpers — `CompareNames`, `NormalizePath`,
`KnownProjects` — stay, serving `today` and `week`. The item half, `GroupByProject` and `Resolve`,
moves to `internal/tree` and becomes title-fragment resolution instead of path matching.

## 4. What the tree renders

```
▾ DevResults                                    5 open
  ▾ Bulk imports → background worker            2 open
      c3   fix N+1 in importer
      c4   add retry backoff
  ▾ Search rewrite                              1 open
      c7   reindex script
  ▾ (unassigned)                                2 open
      c9   renew SSL cert
      d1   reply to Dana re: quotas

▾ (unassigned)                                  1 open
    e2   book flights
```

**`(unassigned)` is a rendering artifact, never an item.** It appears at a level only when that level
*mixes* parents and leaves, and gathers the leaves. A level whose children are all leaves renders
them directly — `Search rewrite` above has no bucket inside it. The rule is uniform: the top level
gets a bucket on the same terms as any other, which is why `book flights` sits in one alongside
`DevResults`. Synthetic rows are not selectable; there is nothing to act on.

**Leaf-ness is computed on visible children, not stored ones.** A parent whose children are all done,
under the default open filter, has no visible children and therefore renders as a leaf — into the
bucket with the other leaves. Computing it on stored children instead would leave empty headings
scattered through the tree, which is the same wall of text this replaces.

**Sibling ordering** is parents first by title, then leaves by most-recently-updated, then
`(unassigned)` last. Headings are landmarks and should stay put while task rows churn underneath
them.

**Filtering keeps ancestors.** When a query or a tag matches a deep item, its ancestor chain stays
visible as context. A filtered tree that drops its headings has thrown away the thing that made it
readable.

A match keeps its **whole subtree**; a node that does not match is kept only as context for a
descendant that did. So filtering on a project's name shows that project's contents rather than an
empty heading, and filtering on a leaf's name shows the headings above it and nothing beside it.

## 5. Closing a parent

A parent cannot be closed while anything beneath it is open. `waid done` on such an item is an
`errs.UserError` naming the open descendants.

Cascade was the alternative and was rejected: a recursive close is a large, quiet write, and the
point of a tree is that a closed project provably has no loose ends. The cost is abandonment —
walking the children of a project being dropped rather than finished — which is real, and is why
`done` takes several ids (§6).

The guard is one function, called by both `waid done` and the app's `x`. Two surfaces that disagreed
about when a parent may close would be worse than either rule on its own.

## 6. CLI surface

`-p` keeps its spelling and its matching rules — case-insensitive substring, no match is
`errs.Userf`, several is `errs.Ambiguous` — but resolves against **item titles** rather than paths.
It reads as "parent" now.

It also still accepts an absolute path — and `.` for the current directory — filing it as `origin`
with the item left at top level. This is exactly what promotion does, and it means the
`waid add "<title>" -p <project path>` form in the global `WAID.md` snippet keeps working unchanged.
Agents that learned the path form are not broken by this spec.

So the resolver returns a discriminated result rather than a string — a parent id, or an origin path,
never both. `add` accepts either. `move` accepts a parent only: reparenting to a path is meaningless
now that a path is not a place in the tree, and an absolute path passed to `move` is a
`errs.UserError` rather than a silent no-op.

```
waid add "<title>" [-p <parent|path>] [--waiting-on <who>] [--tag <t>]
waid move <id> [-p <parent> | --top]           reparent; rejects self and descendants
waid done <id>...                               several ids; refuses while descendants are open
waid loops [-p <fragment>] [--origin <frag>]    a subtree, or everything from one path
waid list [--tag t] [--origin <fragment>]       --origin replaces --project
```

`done` taking several ids exists only because §5 chose refusal over cascade; without it, "close the
children first" is four invocations. It is a change to `targetItem`, not to the write. Every id is
validated before anything is written, so a bad id in the list cannot leave a half-finished close
behind.

`--origin` is on `loops` as well as `list`, because "what is tracked for this repo" survives the
reframe as a question — it is simply an origin question now rather than a project one.
`agent/skills/waid/SKILL.md` opens its triage flow with `waid loops -p .`, and this is what that
becomes.

`today` and `week` are untouched.

## 7. The app: one flat row list

`Tree[T]`'s cursor is `position{group, item}` — two levels, structurally. Rather than a second
component for Loops, the tree flattens:

```go
type Row[T any] struct {
    Node     T
    Depth    int
    HasKids  bool
    Expanded bool
}
```

The cursor becomes an index into the visible rows. Repos and Agents become the degenerate case —
depth 0 headings, depth 1 children — which is what `tree.go`'s own comment argues for: two
hand-written trees would drift within a month and each would look correct in isolation.

`Selectable func(Row[T]) bool` replaces the hard-coded rule that the cursor steps over an expanded
heading. Repos and Agents pass `Depth > 0`, which is today's behaviour exactly. Loops selects
everything, because its headings are items that get noted, tagged and closed.

`Expanded map[string]bool` generalises untouched — keyed by item id for Loops, by path for the other
two.

Indent is capped past a few levels, so a deep branch cannot squeeze the title column to nothing on a
narrow terminal.

### 7.1 Moving is navigation, not typing

`P`, `promptProject` and `promptParent` are all deleted. The tree is already on screen, and making
someone type a title they can see is worse than letting them point at it.

`m` lifts the item under the cursor — and its descendants — out of the tree. What remains is the tree
*as it will be after the move*, navigated with the ordinary keys. `enter` drops, `esc` cancels.

```
 moving  c3  fix N+1 in importer  + 2 children        esc cancels · enter drops
────────────────────────────────────────────────────────────────────────────────
     ── top level ──
   ▾ DevResults                                     4 open
     ▸ Bulk imports → background worker             1 open
 › ▸ Search rewrite                                 1 open
   ▸ waid                                           3 open
```

Lifting the subtree out is what makes this safe rather than merely convenient: **the app never has to
reject a move, because the invalid destinations are not on screen to choose.** A node cannot be
dropped inside itself when its own descendants have left the tree with it. The `errs.UserError` in §6
remains for the CLI, where a fragment can still name a descendant.

The rest follows from that:

- **Every real row is a target, leaves included.** Dropping onto a leaf makes it a parent. That is not
  how tiers are normally created (§7.2) but it is a coherent thing to do and there is no reason to
  forbid it.
- **`(unassigned)` headings are not targets**, as everywhere else. Dropping "into unassigned" is
  dropping onto that level's parent, which is already a row.
- **A synthetic `── top level ──` row** is pinned above the tree, in move mode only. The roots are
  items, so without it there is no row meaning "no parent" — and a dedicated key would be one more
  thing to know for a destination the eye can already find.
- **Folds start collapsed** except along the path to the item's current parent. Any tree worth
  building then fits one screen as roots-plus-one-branch, which is what keeps navigation cheap enough
  that no search is needed. `h`/`l` expand along the way; `enter` cannot toggle here because it
  commits.
- **The cursor opens on the item's current parent**, so an immediate `enter` is a no-op rather than a
  surprise, and a nudge to a sibling project is one keystroke.
- **Undo** pushes an inverse `ParentEvent` carrying the prior parent, as `undoRefile` does now.

Only the *picker* is replaced. `refile.go`'s write is what move mode commits with, so it survives the
deletion of the prompt around it — which is also what lets the app stay usable while the tree is
being built, rather than losing the ability to refile for the length of the branch.

Typed filtering inside move mode was considered and deferred. `/` already implements the
ancestor-preserving filter (§4), so it can be folded in later if a tree ever outgrows a screen; doing
it now would be solving a problem collapsed-by-default appears to remove.

### 7.2 Creating a tier

`a` is global and already files under `m.cursorProject()` — "the project the row the cursor is on
already says". That pattern generalises directly:

- **`a` adds a sibling**: a child of the cursor row's parent. This is today's behaviour exactly, and
  is unchanged when pressed from Repos or Agents.
- **`A` adds a child** of the cursor row. This is how a new tier is created: cursor on `DevResults`,
  `A`, type a title, and the project exists — then `m` items into it.

Creating a project is therefore a first-class action rather than something achieved sideways by
moving a task onto another task.

## 8. Migration

None. Old lines fold correctly, `project` maps to `Origin`, and every existing item has no parent and
so lands in top-level `(unassigned)`.

A `waid regroup --from-origin` helper was considered and dropped: at the current volume the tree is
better built by hand, because the point of the tree is that it reflects how the work is thought about
rather than where the files happen to live.

## 9. Testing

- **Fold** — both `project` and `origin` keys; `parent` set, and cleared to null.
- **Tree** — cycles, unknown parent, sibling ordering, the mixes-parents-and-leaves rule for
  `(unassigned)`, filter-keeps-ancestors.
- **Close guard** — open descendants, all-closed descendants, leaf.
- **Move** — self-parent, descendant-parent, ambiguous fragment. All three are CLI-only concerns; the
  corresponding app test asserts the opposite, that a lifted subtree is absent from the destinations
  offered.
- **Move mode** — the lift, the collapsed-by-default fold state, the cursor opening on the current
  parent, `── top level ──` as a destination, `esc` leaving no event behind, and undo restoring the
  prior parent.
- **App** — `teatest` goldens for a three-deep tree, a filtered tree, a deep tree at narrow width, and
  a move in progress.
- `testdata/golden` is regenerated. The Node differential comparison is already retired, so there is
  no cross-build constraint on output shape.

## 10. Phasing

1. `Parent` and `Origin` in the fold, with the compatibility read. Nothing consumes them yet.
2. `internal/tree`: the forest, the `(unassigned)` rule, ordering, ancestor-preserving filter, and the
   `State` index. `doctor` reports cycles and orphans.
3. CLI: `-p` resolution, `move`, the close guard, multi-id `done`, `--origin`, tree rendering for
   `list` and `loops`, promotion, and the docs that describe all of it — `usage.go`, `README.md`,
   `agent/CLAUDE.snippet.md` and `agent/skills/waid/SKILL.md`.
4. `Tree[T]` flattening, with Repos and Agents ported first — their behaviour is unchanged, so their
   existing goldens are the proof the refactor is sound. A golden that moves means the refactor is
   wrong.
5. Loops on the flat tree: selectable headings, `h`/`l`, `a`/`A`, move mode, reparent undo.

Steps 1–3 are shippable without touching the app; step 4 is a refactor with no user-visible change.
The app stays usable throughout: the parent write lands in step 1 (§7.1), so refiling never stops
working while the tree is being built.

## 11. Decisions settled during design

- **Areas and projects are items, not entities.** One shape, and every existing command works at every
  level for free.
- **Unlimited depth**, not a three-level cap. There is no principled place to stop.
- **`(unassigned)` gathers leaves only where a level mixes them with parents**, and is never an item.
- **Closing a parent is refused, not cascaded**, while anything under it is open.
- **Promotion lands at top-level `(unassigned)`** with the repo recorded as `origin`. Filing is a
  separate sweep, so promotion stays one keystroke.
- **The path becomes `origin`, a queryable field** — not a note. A note cannot be filtered on.
- **The event log stays.** Git-diffable, hand-editable, lock-free; the hierarchy needs nothing a
  database provides.
- **`-p` still accepts an absolute path**, so the existing `WAID.md` agent instruction keeps working.
- **Moving is navigation, not typing.** `m` lifts the subtree and the tree is browsed to its
  destination. `P` and the parent prompt are deleted rather than kept alongside it.
- **Lifting the subtree replaces cycle rejection in the app.** An invalid destination is one that is
  not drawn, so the TUI never reports a move it refuses to make.
- **Move mode collapses folds by default**, which is what makes navigation cheap enough that typed
  filtering could be deferred rather than built.
- **`a` adds a sibling and `A` adds a child.** Creating a tier is a first-class action, not a side
  effect of moving one task onto another.
- **A filter match keeps its whole subtree**; a non-matching node is kept only as context for a
  descendant that did.
- **`--origin` is on `loops` as well as `list`.** "What is tracked for this repo" survives the
  reframe as a question, and the `/waid` skill opens its triage flow with it.
- **The close guard is one function**, called by `waid done` and the app's `x` alike.
- **Multi-id `done` validates every id before writing anything**, so a bad id cannot leave a
  half-finished close behind.
- **Tags are out of scope**, and unchanged by any of this.
