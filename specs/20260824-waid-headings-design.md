# waid — Headings — Design

**Date:** 2026-08-24
**Status:** Approved design, pending implementation plan
**Refines:** `20260821-waid-hierarchy-design.md` — §2 (the data model) and §4 (what the tree
renders). Everything that spec says about the tree holds; this changes one rule inside it.

## Purpose

`(unassigned)` gathers the leaves of a level that mixes parents and leaves, and leaf-ness is computed
on visible children. That rule is right for a task, and wrong for the row a task is filed under. A
project cleared of its work does not become a task: it becomes a shelf with nothing on it. Today it
is swept into `(unassigned)` beside the loose items, which is the one place it must not be —
`(unassigned)` is the inbox, and a project sitting in the inbox reads as work nobody has filed yet.

The tree cannot infer the difference. `Localized notifications` with no open children and `Read the
port design once more` are the same shape, and only the person who wrote them knows which is a shelf.
So the difference is declared: an item can be marked a **heading**, and a heading heads a level
whether or not anything is filed under it.

New items keep landing loose, so `(unassigned)` stays the inbox it is being used as.

## Non-goals

- **Not a project entity.** §1 of the hierarchy spec still holds: an area, a project and a task are
  the same shape. `Heading` is a flag on an item, not a second kind of thing with its own id,
  lifecycle or commands. A heading is closed, noted, retitled, moved and undone like any other item.
- **Not a root-level construct.** A heading is allowed at any depth. Depth 0 keeps meaning what it
  means today.
- **Not a change to what lands where.** `add` and `promote` file exactly where they file now.
- **Not a rewrite of the log.** No line already written changes, and none needs to.

## 1. The data model

One field on the item:

```go
type Item struct {
    // …
    // Heading marks an item as a landmark over work rather than work of its own: it heads a level
    // whether or not anything is filed under it, and is never gathered into (unassigned).
    Heading bool `json:"heading"`
}
```

Two event shapes carry it:

- `AddEvent` gains `Heading bool json:"heading,omitempty"`. It is omitted when false, so every line
  already in the log re-encodes byte for byte.
- `HeadingEvent` is new, and always writes the field:

```go
type HeadingEvent struct {
    Ts      string `json:"ts"`
    Ev      string `json:"ev"`
    Id      string `json:"id"`
    Heading bool   `json:"heading"`
}
```

It is a shape of its own for the reason `ParentEvent` is one: `UpdateEvent` omits an absent field, so
an `omitempty` boolean could never say *false*, and a retitle that dropped the key would silently
unmark a heading. A move already had to solve this; the toggle has the same shape of problem.

**The key is `heading`, not `project`.** `project` is taken: the fold reads it as the pre-rename
spelling of `origin`, and `--project` is a retired CLI flag that fails loudly. `project` also already
means a repo path in Repos, Agents, `today` and `week`. The tree's own vocabulary is *heading*
(`OnHeading`, "headings are landmarks"), and that is the word used everywhere here.

### 1.1 Reading what is already written

The fold reads the key by presence, as it does for every other field: the `add` branch narrows
`record["heading"]` through a `boolOf` helper, and `applyToItem` sets `item.Heading` when an update
carries the key. A value that is not a boolean is ignored rather than reported — the same treatment
`parent`, `origin` and `tags` get. Only a failure that loses an event outright becomes a `Problem`.

No line in the log carries `heading` today, so everything already written folds to `Heading: false`.
No migration, no dual-write, no version gate.

## 2. The tree rule

`internal/tree` grows one predicate, and two rules switch to it:

```go
// heads reports whether a node heads a level: it holds children, or it was declared a heading.
func heads(node Node) bool { return len(node.Children) > 0 || node.Item.Heading }
```

- **`Bucket`** splits a level with it instead of on `len(Children) == 0`. A heading is never gathered
  into `(unassigned)`, empty or not. Nothing else about bucketing changes: an ordinary item with no
  children still lands in the inbox exactly as it does now, and a level of nothing but leaves still
  renders them directly.
- **`sortSiblings`** orders heads-first by title, then the rest most-recently-updated first. An empty
  heading therefore sorts with the parents by title rather than drifting through the leaves by age,
  which is what "headings are landmarks and should stay put" already asks for.

Closing is untouched. `GuardClose` refuses while anything under an item is still open, so a heading
holding work cannot be closed and an empty one closes like any other item — being a shelf is not a
reason to keep it. A closed heading then leaves the open view through the status gate, toggle or no
toggle: `heading` and `status` are separate fields and neither reads the other.

## 3. Empty headings are hidden

The hierarchy spec's §4 objected that computing leaf-ness on stored children "would leave empty
headings scattered through the tree, which is the same wall of text this replaces." That objection
stands, and is what this section answers: a heading with no **visible** children is dropped from the
view unless the reader has asked for it.

- `tree.DropEmptyHeadings` runs after `Prune` and `Filter` and before `Bucket`, in the three places
  that shape a forest for reading: `internal/tui`'s `shapedForest`, `internal/command/list.go`'s
  `forest`, and `runLoops`, which builds its own.
- The move picker never applies the pass. Every heading is a destination whether or not it holds
  anything — filing the first item under a fresh project is the reason to have one.

Which views ask for them is its own question, and the answer is: whichever view the reader says.
Status answers *what state is this item in*. Revealing the shelves answers *am I filing right now* —
a different axis, which composes with every status rather than taking a slot beside them. Every name
for it as a status came out wrong (`active`, `undone`, `incomplete`) because it is not one.

So the reveal is a **toggle**, orthogonal to the status row:

- In the app, `S` flips it. It holds across status changes, refreshes, filters and tab switches for
  the length of the session, and is not written to disk — there is no settings file, and this does
  not justify one.
- On the command line it is `--headings`, taken by both `list` and `loops`.
- The move picker ignores it and always shows them, as above.

Off — the default — every status drops empty headings; on, none of them do. `--all` is **not** a
second way to switch it on: `--all` is the status axis saying "include done", and one flag reaching
across two axes is how the row ended up needing a name for a thing that has none. `waid list --all
--headings` is the whole picture, and says so.

Three consequences worth naming.

A heading whose children are all done disappears with the toggle off, because it is empty *in that
view* — the same visible-children rule the tree already runs on.

The rule is emptiness alone. A heading carrying its own `waiting` status and nothing underneath is
still hidden, because a heading is a landmark over work rather than work.

**The toggle can add an `(unassigned)` bucket, not just a row.** The pass runs before `Bucket`, so a
level whose only head was an empty heading is a level of nothing but leaves while the toggle is off,
and those leaves render directly. Flipping it on restores the heading, which makes the level mixed
again, which is what puts its leaves in a bucket. Both renderings are correct for what they are
showing, and the order is what keeps them so — bucketing before the drop would leave `(unassigned)`
holding leaves that have nothing to be unassigned from.

## 4. Rendering and counts

An empty heading draws as a heading line — its title and `0 open` — with a blank marker rather than
`▸`, since there is nothing folded under it to open. That is the marker the move picker's
`── top level ──` row already uses for a row that is a destination without being a branch.

```
▾ Localized notifications                       2 open
  ▸ Background workers speak the requester's…    1 open
  ▾ (unassigned)                                1 open
      sga9  open  Design template + args persi…

  Port rewrite                                  0 open

▾ (unassigned)                                  2 open
    9xz1  open  TUI design spike
    p0rt  open  Read the port design once more
```

`Port rewrite` is a root beside `Localized notifications` — its title sits in the same column, and
the column where a marker would be is blank. It is what the toggle adds; with the toggle off the
tree is the same one without that row.

`waid list` and `waid loops` render the same shape through `TreeLines`.

**Counts skip what is hidden.** The Loops badge leaves out a heading with no open descendants
whatever the toggle says — the badge is what is owed, and an empty shelf is owed nothing. The
header's item count is read off the rendered tree as it is now, so it rises by the shelves while the
toggle is on. A heading with work under it counts as it does today.

`waid show` and every `--json` item carry `heading`, and the detail pane names it beside the status.

## 5. CLI surface

```
waid heading <id>              Mark an item a heading
waid heading <id> --off        Unmark it
waid add "<title>" --heading   Create one
waid loops [--headings]        Show empty headings alongside the open loops
waid list [--headings]         The same reveal, under list's own filters
```

`HeadingResult` echoes `{id, title, heading, ts}` for `--json`. The text rendering follows the shape
`move` uses — `marked <id>  <title>  a heading`, and `unmarked <id>  <title>` for `--off`. The
registry, the usage table and the README table each gain their line.

The event is written whatever the item already held, which is how `done` treats an item that is
already closed: the log is a history of what was asked for, the fold is idempotent, and a command
that sometimes wrote and sometimes did not would make `u` guess.

What `--headings` reaches differs between the two readers, because their payloads are built
differently and neither should change shape here. `loops` flattens its rendered forest into `Items`,
so the flag reaches its `--json` as well as its text. `list` selects `Items` from the log with its
own filters and holds the forest beside them, so the flag shapes only what it renders — an empty
heading that passes `--status` is in `list --json` either way.

## 6. The app

`P` toggles the heading flag on the row under the cursor. It is a write like `x` or `w`: it leaves a
receipt, it is undone by `u` — the inverse is a `HeadingEvent` carrying the prior value — and it is
inert with a reason in the footer on a row holding no item, which is the `(unassigned)` bucket.

`P` rather than `H`: `h` collapses a fold, so a shift-slip while walking the tree with `h` and `l`
would write an event. `P` is unbound on every tab, and shifted pairs are already how the tab spells a
variant of a key — `a` adds a sibling, `A` adds a child.

Both keys are Loops keys: they join `tabBindings[tabLoops]`, so pressing either on Repos or Agents is
inert and the footer says which tab owns it, as every other tab key already does. The Loops key table
and footer hints gain both.

Filing into a fresh shelf means the toggle is on. `A` files under the row the cursor is on, and a
hidden row has no cursor — `S`, then `A`, is how a project gets its first item. Move mode needs no
such thing, since it shows every heading regardless (§3).

### 6.1 The headings chip

`S` flips §3's toggle — the shifted sibling of `s`, which cycles the statuses, so the two filter keys
sit together. Not `H`, for the reason `P` is not `H`.

The chip is drawn on the header row, after the status segments and set apart from them by `│` in
`Theme.TabRule` — the glyph and the style the tab bar already uses to divide one zone from another,
so the row reads as two things rather than as four segments and a fifth that behaves oddly. The chip
itself is spelled the way a segment is, since it is the same kind of switch: `headings` in
`FilterInactive` when off, `‹headings›` in `FilterActive` when on.

```
 ‹open›  waiting  done  all  │  headings                          7 items
 ‹open›  waiting  done  all  │  ‹headings›                        9 items
```

`segmentRow` returns its printed width already, and `headerLine` hangs the counts off the right
edge, so the chip is appended between them and the widths keep adding up. At 80 columns the row and
the counts both fit, which is the width the goldens pin.

## 7. Migration

None. Existing items fold to `Heading: false`, which is exactly what they are: nothing in the log was
ever declared a shelf. A project that should be one is marked with a keystroke.

## 8. Testing

- **Fold:** an `add` carrying `heading` folds to a marked item; an update toggles it both ways; a
  retitle leaves it alone; a non-boolean value is ignored; a line written before this spec folds
  unchanged and re-encodes byte for byte.
- **`internal/tree`:** an empty heading stays out of the bucket and sorts with the parents;
  `DropEmptyHeadings` drops it and leaves a heading with visible children alone; an ordinary
  childless item still buckets; a level whose only head was a dropped heading renders its leaves
  directly, and gets its bucket back when the heading is kept; `GuardClose` still refuses a heading
  holding open work and allows an empty one.
- **Commands:** `waid heading` marks, unmarks and rejects an unknown id; `add --heading` writes the
  key; `--headings` reveals what the default view hides on both `list` and `loops`, and `--all`
  without it does not; marking an item that is already marked writes the event anyway; `--json`
  carries the field, and reaches `loops`' item set but not `list`'s.
- **App:** `P` writes, receipts and undoes; an empty heading is absent until `S` and present after
  it; the toggle composes — it holds while `s` cycles the statuses and while a query is typed and
  cleared; the chip draws both states with the divider between it and the segments; the move picker
  offers the heading either way; the badge holds still while the header count rises; goldens
  regenerated for both chip states.
- **Harness:** the smoke test drives `waid heading` through the installed binary.

## 9. Decisions settled during design

- **A field, not a tag.** A reserved `#heading` tag needs no schema change, but it is a magic string
  in a namespace the user owns, it draws in the row's meta column, `--tag heading` would list
  headings as though the word were a label, and toggling it is a read-modify-write of the whole
  `tags` array — unsafe with the parallel agents this log is written by.
- **Declared, not derived.** "Has ever had children" needs no new state and answers a different
  question: it cannot express a project created empty, and it cannot be undone.
- **Any depth, not root-only.** Root-only would need move mode to refuse a destination and a mark to
  reparent the item, for no gain — the flag means "never bucketed, survives being empty", and that is
  as true two levels down as at the top.
- **Hidden while empty, not always shown.** A permanent shelf of cleared-out projects is the wall of
  text the tree replaced.
- **A toggle, not a status.** A fifth segment was drafted and dropped. Every name for it described a
  status it was not — `active` and `undone` both read as "not done", which is what `open` already
  says — and the naming trouble was the design saying that revealing the shelves is a second axis. As
  a toggle it composes with all four statuses, mirrors `--headings` one-to-one, and needs no name
  beyond the thing it reveals.
- **`--all` does not imply `--headings`.** One flag reaching across both axes is what made the
  segment hard to name. `--all` includes done; `--headings` includes empty headings; asking for
  everything is asking for both.
- **Toggle both ways.** A one-way mark is a mis-tap away from an edit of the log by hand.
