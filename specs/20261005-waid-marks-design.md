# waid — Marks — Design

**Date:** 2026-10-05
**Status:** Draft, pending review
**Refines:** `20260821-waid-hierarchy-design.md` — §7.1 (the move picker) — and
`20260817-waid-app-design.md` — §3 (writes, receipts and undo) and §4 (the key table). Everything
those specs say about a write against one item holds; this adds a way to make one against several.

## Purpose

Triage is the same write made many times: this goes under that project, and so does this, and this.
Today each one is a round trip — arrow to the item, `m`, pick the destination, and the cursor is
left wherever the write put it, so the next item has to be found again. Filing ten loose items under
one heading is ten trips to the same place.

The fix is to say *which* first and *what* once. Rows are **marked**, and a write made while any are
marked is made against all of them.

## Non-goals

- **Not on Repos or Agents.** The tree component learns to draw a mark, and only Loops hands it any.
- **Not a CLI change.** `waid move` keeps taking one id. `waid done <id>...` already takes several.
- **Not a range select.** A mark is one row. A contiguous run is marked a row at a time, which the
  step-down in §1 makes one key per row.
- **Not a batch for every key.** Four writes take the marked set (§2); the rest stay on the cursor.
- **Not a new event.** A batch is the events the same writes already append, one per item.

## 1. Marks

A mark is an item id held in the Loops model: `marked map[string]bool` on `loopsModel`. Keyed by id
rather than by row, it survives everything that re-sorts or redraws the list — a reload, a fold, a
query, the status row, the headings toggle.

| Key | Effect |
|-----|--------|
| `x` | Toggle the mark on the row under the cursor, then step the cursor down a row. |
| `X` | Clear every mark. |

- **`x` steps down** so a run of rows is marked by holding one key. It steps whether the press set
  the mark or took it off, and stays put on the last row.
- **`x` on the `(unassigned)` bucket is inert** with a reason in the footer, as every other write
  is there: the bucket is not an item.
- **A mark covers the item it is on and nothing else.** Marking a fold does not mark what is filed
  under it. A move still carries the subtree, because that is what moving an item means (§3).
- **`esc` is left alone.** It clears the query, and filter → mark → clear the filter → mark more is
  the flow marks are for. Clearing them is `X`'s job.
- **A reload drops marks for ids the log no longer holds.** Nothing removes an item today, so this
  only keeps the count honest if that changes.
- **A mark can be off screen.** An item marked and then hidden by the query or the status row stays
  marked, and a batch still takes it. The header count (§1.1) is what keeps that in view.

`x` was `done`. Done moves to `d` (§5).

### 1.1 Drawing a mark

`Tree[T]` gains one optional field:

```go
// Marked reports whether a row carries a mark. A tree supplying none draws none.
Marked func(row Row[T]) bool
```

A marked row draws `●` in column 0. `indent` always leaves that column blank — it is the
`headingIndent` before a depth-0 marker — so the glyph collides with neither the cursor mark nor
the fold marker, sits in one vertical gutter at every depth, and costs the row no width. An unmarked
tree renders byte for byte what it renders today, so no golden moves until something is marked.

The header row carries the count while there is one, ahead of the item count on the right edge:

```
 ‹open›  waiting  done  all  │  headings                3 marked · 41 items
```

The footer's hints swap to the batch keys while anything is marked, the way the move picker swaps
them: `m move · d done · w waiting · t tag · x mark · X clear`.

## 2. Writes against the marked set

While at least one item is marked, four keys act on every marked item rather than on the cursor row:

| Key | Batch |
|-----|-------|
| `m` | Move them all to one destination (§3). |
| `d` | Close them all. |
| `w` | One prompt, one name, every item waiting on it. |
| `t` | One prompt, tags **added** to every item. |

With nothing marked each of the four acts on the cursor row exactly as it does today.

`e`, `n`, `y`, `H`, `a` and `A` act on the cursor row whether or not anything is marked: a title, a
note and an id belong to one item, and an add is placed by where the cursor is.

**Order.** A batch walks the marked items in the order the log holds them, so the events it appends
do not depend on the order the marks were made in.

**Checked before written.** Everything a batch could refuse is checked before the first event is written.
An append that fails part way stops the batch there: the events that landed are applied and pushed
as the undo entry, the hint names the failure, and the marks are left standing so the rest can be
retried.

**Marks clear when a batch lands.** The set has been answered, and a second write against it is
more often a slip than an intention.

### 2.1 `d` — done

- Items already closed are skipped, as the single write refuses them; a batch holding nothing else
  is a hint and no write.
- The rest go through `tree.GuardCloses` as one set — the guard `waid done <id>...` already calls —
  so a heading marked alongside everything open beneath it closes with it, and one that leaves a
  descendant out refuses the whole batch with the guard's own message.

### 2.2 `w` — waiting

The prompt reads `waiting on (3 items)`. Its answer writes the same `UpdateEvent` per item that the
single write does. An empty answer abandons it, as today.

### 2.3 `t` — tags

A single `t` replaces the set, which is safe because the prompt opens on the set being replaced.
Several items carry several sets, and no one line can stand for them, so the batch prompt opens
empty, reads `add tags (3 items)`, and **adds** what is typed: each item is written a `TagsEvent`
carrying its own tags plus the new ones, normalized as usual.

- An empty answer abandons the prompt. It does not clear every item's tags, which is what an empty
  answer means to the single prompt.
- Taking a tag off several items at once is not offered. It is rarer than adding one in triage, and
  the single `t` still does it an item at a time.

## 3. Moving the marked set

`moveModel.subject string` becomes `subjects []string`, and `active()` is whether it holds any.

- **What is lifted.** Every marked item and everything under each. A marked item sitting under
  another marked item is dropped from the subjects first: its ancestor already carries it, and
  writing its own parent as well would pull it out from under the ancestor and flatten the subtree.
- **The picker** is the tree with all of that lifted out, which keeps §7.1's guarantee: no invalid
  destination is on screen to choose.
- **The header** names the item for one subject, as today, and counts for several:
  `moving  3 items  + 4 children`.
- **Where it opens.** On the parent the subjects share, if they share one — so an immediate `enter`
  is still a no-op — and on the top-level row otherwise.
- **The drop** writes a `ParentEvent` per subject. A subject already under the destination is
  skipped rather than written, as `refile` skips it today; a drop that moves nothing writes nothing
  and leaves the marks standing.
- **`esc`** closes the picker and leaves the marks standing.

`m` with nothing marked opens the picker on the cursor row: one subject, and the same code path.

## 4. Undo and receipts

**One undo entry per batch.** `undoEntry.events` is already a list; `item *events.Item` becomes
`items []events.Item`, each restored in turn. `u` after a batch reverses every write in it and
leaves one receipt. It does not put the marks back.

**Receipts.** Each item's write is recorded as its own receipt, in the wording the single write
uses, so the quit replay still lists every id the session touched. The footer shows the last
receipt, which for a batch would be one item of several, so a batch records a summary after them
that the footer shows and the replay leaves out:

```
filed 3 items  Localized notifications
closed 3 items
waiting 3 items  alex
tagged 3 items  bug
```

`receipt` gains a `summary bool` that `replay` skips. The undo of a batch records its own summary in
the same shape (`restored 3 items`).

## 5. `d` is done

`x` becomes the mark, and done moves to `d`. Loops has no `d` today; Repos' `d` (dismiss) and
Agents' `d` (date) are tab keys and are untouched.

- `tabBindings[tabLoops]`: `x` → "mark the row, for a write against several", `X` → "clear the
  marks", `d` → "done". The effects of `m`, `d`, `w` and `t` say they take the marked set.
- `tabFooters[tabLoops]`: `d done · w waiting · … · x mark`.
- The inert hints that name the key (`x closes an item`) follow it.
- `README.md`'s key list, and the goldens that print the footer.

## 6. Code

| File | Change |
|------|--------|
| `tui/tree.go` | `Marked`, and the gutter glyph in `heading` and `itemLine`. |
| `tui/loops.go` | `marked` on the model; `x`, `X`, `d`; the header count; pruning on load. |
| `tui/marks.go` (new) | Toggle and clear; the targets of a write (the marked set, else the cursor row); dropping subjects that sit under another subject. |
| `tui/move.go` | `subjects`; the lifted set, the header and the opening row for several. |
| `tui/refile.go` | `refile` takes the subjects and writes one batch. |
| `tui/writes.go` | `doneSelected`, `waitOn` and `setTags` take a list of ids; the batch tag write adds. |
| `tui/prompt.go` | A prompt carries `subjects []string`; the batch labels. |
| `tui/undo.go` | `items`; the inverses built over a list. |
| `tui/receipt.go` | `summary`, skipped by `replay`. |
| `tui/app.go` | Bindings and footers. |

The single-item path is the batch path with one id, not a second implementation beside it. The
exceptions are the two places a batch means something different, which branch on the count: the tag
prompt (replace against add) and the footer summary (none for one item).

## 7. Testing

- **Tree:** a marked row draws the glyph in column 0 at every depth, on an item row and on a fold; a
  tree with no `Marked` renders unchanged.
- **Marks:** `x` sets, clears and steps down; it stays on the last row; it is inert on the bucket;
  `X` clears; marks hold across a reload, a fold, a query typed and cleared, `s` and `h`; the header
  counts them, including one the filter hides.
- **Move:** several subjects are lifted with their subtrees; a subject under another subject is not
  written and keeps its place beneath it; the picker opens on a shared parent and on the top level
  otherwise; a subject already at the destination is skipped; `esc` keeps the marks; a drop clears
  them.
- **Done:** a batch closes every marked item; closed ones are skipped; a heading with its open
  descendants all marked closes; one with a descendant left out refuses and writes nothing.
- **Waiting and tags:** one prompt writes every item; the tag batch adds to each item's own set and
  leaves the rest of it alone; an empty answer writes nothing.
- **Undo and receipts:** one `u` reverses a whole batch of each kind; the replay holds a line per
  item and no summary; the footer shows the summary.
- **Unmarked:** `m`, `d`, `w` and `t` with nothing marked behave as before, and `e`, `n`, `y`, `H`,
  `a`, `A` act on the cursor row with marks set.
- **Keys:** `d` closes and `x` no longer does; the key table and footers are regenerated; goldens
  added for a marked list and for the picker header over several subjects.

## 8. Decisions settled during design

- **Marks, not a repeat key.** A `.` that refiles the cursor row to the last destination needs no
  selection state, but it is still a trip per item and helps only the move.
- **`x` marks, `d` is done.** `x` reads as a tick in a box, and `d` is the letter the word starts
  with.
- **All four writes, not the move alone.** A mark that only `m` honours leaves `d` acting on the
  cursor while rows are visibly marked, which is a slip waiting to be made.
- **`X` clears, not `esc`.** `esc` clearing the query is what lets marks be gathered across filters.
- **A mark is one item.** A mark that reached down a fold would make `d` on a marked heading close
  everything under it — the cascade the close guard exists to refuse.
- **Batch tags add.** Replacing several different sets with one typed line destroys what was not on
  screen.
- **Marks clear after a batch.** Leaving them set invites a second write nobody meant.
