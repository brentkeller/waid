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

`State` gains an index built once per fold: `map[string]int` plus each item's children. `State.Find`
is a linear scan, and the tree work hammers it — cycle checks, close guards, ancestor walks,
filter-keeps-ancestors.

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

## 5. Closing a parent

A parent cannot be closed while anything beneath it is open. `waid done` on such an item is an
`errs.UserError` naming the open descendants.

Cascade was the alternative and was rejected: a recursive close is a large, quiet write, and the
point of a tree is that a closed project provably has no loose ends. The cost is abandonment —
walking the children of a project being dropped rather than finished — which is real, and is why
`done` takes several ids (§6).

## 6. CLI surface

`-p` keeps its spelling and its matching rules — case-insensitive substring, no match is
`errs.Userf`, several is `errs.Ambiguous` — but resolves against **item titles** rather than paths.
It reads as "parent" now.

It also still accepts an absolute path, and files it as `origin` with the item left at top level.
This is exactly what promotion does, and it means the `waid add "<title>" -p <project path>` form in
the global `WAID.md` snippet keeps working unchanged. Agents that learned the path form are not
broken by this spec.

So the resolver returns a discriminated result rather than a string — a parent id, or an origin path,
never both. `add` accepts either. `move` accepts a parent only: reparenting to a path is meaningless
now that a path is not a place in the tree, and an absolute path passed to `move` is a
`errs.UserError` rather than a silent no-op.

```
waid add "<title>" [-p <parent|path>] [--waiting-on <who>] [--tag <t>]
waid move <id> [-p <parent> | --top]        reparent; rejects self and descendants
waid done <id>...                            several ids; refuses while descendants are open
waid loops [-p <fragment>]                   that node and everything under it
waid list [--tag t] [--origin <fragment>]    --origin replaces --project
```

`done` taking several ids exists only because §5 chose refusal over cascade; without it, "close the
children first" is four invocations. It is a change to `targetItem`, not to the write.

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

On Loops, `P` keeps its key and changes what it searches: titles, not paths. `promptProject` becomes
`promptParent`, and `refile.go` keeps its shape — `projectCandidates()` returns nodes,
`chosenProject` becomes `chosenParent` and gains the cycle check. `h` and `l` collapse and expand;
`enter` stays toggle. Undo pushes an inverse `ParentEvent` carrying the prior parent, as `undoRefile`
does now.

Indent is capped past a few levels, so a deep branch cannot squeeze the title column to nothing on a
narrow terminal.

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
- **Move** — self-parent, descendant-parent, ambiguous fragment.
- **App** — `teatest` goldens for a three-deep tree, a filtered tree, and a deep tree at narrow
  width.
- `testdata/golden` is regenerated. The Node differential comparison is already retired, so there is
  no cross-build constraint on output shape.

## 10. Phasing

1. `Parent` and `Origin` in the fold, with the compatibility read. Nothing consumes them yet.
2. `internal/tree`: the forest, the `(unassigned)` rule, ordering, ancestor-preserving filter, and the
   `State` index. `doctor` reports cycles and orphans.
3. CLI: `-p` resolution, `move`, the close guard, multi-id `done`, `--origin`, recursive `loops`.
4. `Tree[T]` flattening, with Repos and Agents ported first — their behaviour is unchanged, so their
   existing goldens are the proof the refactor is sound.
5. Loops on the flat tree: selectable headings, `h`/`l`, `P` over nodes, reparent undo.

Steps 1–3 are shippable without touching the app; step 4 is a refactor with no user-visible change.

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
- **Tags are out of scope**, and unchanged by any of this.
