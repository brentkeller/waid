# Marks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the Loops tab mark several rows with `x` and make one move, close, waiting or tag write against all of them.

**Architecture:** A mark is an item id in a set on `loopsModel`. The shared `Tree[T]` learns to draw a mark through an optional `Marked` func that only Loops supplies. The four batchable writes resolve their targets through one helper (the marked set, else the cursor row) and land through one `commit` helper that appends an event per item, pushes a single merged undo entry and records the receipts — so the single-item write is the batch with one id.

**Tech Stack:** Go 1.27, bubbletea, lipgloss, `teatest` goldens (`-update` regenerates them).

**Spec:** `specs/20261005-waid-marks-design.md` — read it first; section numbers below (§N) refer to it.

## Global Constraints

- Everything lives in `internal/tui` (package `tui`). No change to `internal/command`, `internal/events` or the CLI.
- `Model` is copied by value through the update loop: never mutate a map or slice it holds — build a fresh one and assign it (see `hide` in `triage.go`, `pushUndo` in `undo.go`).
- A batch walks the marked items in the order `m.loops.items` holds them.
- Keys: `x` toggles a mark and steps down, `X` clears every mark, `d` is done. `esc` is not touched.
- The mark glyph is `●` in column 0. A tree with no `Marked` func must render byte for byte what it renders today.
- Batch footer hints, verbatim: `m move · d done · w waiting · t tag · x mark · X clear`.
- Code comments are informative prose in the surrounding style. No comment mentions this plan, a task number, or the change being made.
- Run tests with `go test ./internal/tui`. Regenerate goldens only with `go test ./internal/tui -run <Name> -update`, and read the diff before committing it.
- Commit messages follow the repo's `feat(tui): …` / `docs: …` style and end with the session's attribution trailer.

## Review Focus

- A marked item hidden by the query is still written by a batch — the header count is the only thing showing it. Pinned in Task 5.
- An append that fails leaves the marks standing and says why. Pinned in Task 5.
- `u` after a batch reverses all of it and does not put the marks back. Pinned in Task 5.
- A batch tag answer that normalizes to nothing (`", ,"`) writes nothing rather than a no-op event per item. Pinned in Task 7.
- With several subjects lifted, no subject and no descendant of one is offered as a destination. Pinned in Task 8.

## Fixtures the tests rely on

`working(t)` (in `writes_test.go`) is a 140-column Loops tab over a temp log. Its visible rows, top to bottom, with the cursor opening on row 0:

| Row | Id | Notes |
|-----|----|-------|
| 0 | `vq2n` | fold, "Localized notifications" |
| 1 | `sga9` | under `vq2n` |
| 2 | `nktt` | under `vq2n`, tags `["promoted"]` |
| 3 | — | the `(unassigned)` bucket |
| 4 | `9xz1` | top level |
| 5 | `p0rt` | top level |
| 6 | `4h2k` | top level, waiting on maria |

Log order (the order `m.loops.items` holds) is `vq2n, nktt, sga9, 4h2k, 9xz1, p0rt, shut` — `nktt` precedes `sga9`. `shut` is closed and hidden.

`filing(t)` (in `move_test.go`) is the same log with `4h2k` moved under `nktt`.

Helpers already present: `press`, `plain`, `rowFor`, `lineFor`, `listed`, `assertLog`, `written`, `loopsStamped`, `parentOf`, `destinations`, `focused`, `headingLine`, `looped`, `loopsFixture`, `loopsNested`.

---

### Task 1: The tree draws a mark

**Files:**
- Modify: `internal/tui/tree.go`
- Test: `internal/tui/tree_test.go`

**Interfaces:**
- Produces: `const markGlyph = "●"`; `Tree[T].Marked func(row Row[T]) bool`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/tree_test.go` (add `"github.com/charmbracelet/lipgloss"` to the imports if it is not there):

```go
// marking is a tree two levels deep whose fold and first leaf carry a mark.
func marking(marked func(Row[string]) bool) Tree[string] {
	return Tree[string]{
		Rows: []Row[string]{
			{Node: "root", Depth: 0, Key: "root", Title: "root", node: true},
			{Node: "leaf", Depth: 1, Key: "leaf", node: true},
			{Node: "other", Depth: 1, Key: "other", node: true},
		},
		ExpandedByDefault: true,
		Selectable:        func(Row[string]) bool { return true },
		Marked:            marked,
	}
}

// A marked row draws the glyph in the gutter at any depth, on a fold and on an item alike, and the
// row is no wider for it.
func TestMarkedRowsDrawTheGlyphInTheGutter(t *testing.T) {
	marked := marking(func(row Row[string]) bool { return row.Key != "other" }).Lines(40)
	bare := marking(nil).Lines(40)

	for i, want := range []bool{true, true, false} {
		if got := strings.HasPrefix(marked[i].Text, markGlyph); got != want {
			t.Errorf("line %d is %q, marked = %v, want %v", i, marked[i].Text, got, want)
		}
		if got, want := lipgloss.Width(marked[i].Text), lipgloss.Width(bare[i].Text); got != want {
			t.Errorf("line %d is %d columns marked and %d bare", i, got, want)
		}
	}
}

// A tree that marks nothing renders what a tree with no notion of marks renders.
func TestATreeMarkingNothingRendersUnchanged(t *testing.T) {
	none := marking(func(Row[string]) bool { return false }).Lines(40)
	bare := marking(nil).Lines(40)

	for i := range bare {
		if none[i].Text != bare[i].Text {
			t.Errorf("line %d is %q, want %q", i, none[i].Text, bare[i].Text)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/tui -run 'TestMarkedRows|TestATreeMarkingNothing'`
Expected: build failure — `unknown field Marked`, `undefined: markGlyph`.

- [ ] **Step 3: Implement**

In `internal/tui/tree.go`, add to the first `const` block:

```go
	// markGlyph is drawn in the first column of a row that carries a mark. indent always leaves that
	// column blank, so the glyph meets neither the cursor mark nor the fold marker.
	markGlyph = "●"
```

Add to `Tree[T]`, after `Selectable`:

```go
	// Marked reports whether a row carries a mark. A tree supplying none draws none.
	Marked func(row Row[T]) bool
```

Add a method beside `indent`:

```go
// gutter is a row's prefix with the mark drawn in its first column when the row carries one.
func (t Tree[T]) gutter(row Row[T], prefix string) string {
	if t.Marked == nil || !t.Marked(row) {
		return prefix
	}
	return markGlyph + strings.TrimPrefix(prefix, " ")
}
```

In `heading`, change `prefix := indent(row.Depth) + marker + " "` to:

```go
	prefix := t.gutter(row, indent(row.Depth)+marker+" ")
```

In `itemLine`, make the same change to its `prefix :=` line.

- [ ] **Step 4: Run the package**

Run: `go test ./internal/tui`
Expected: PASS, no golden changed.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tree.go internal/tui/tree_test.go
git commit -m "feat(tui): the tree draws a mark in a row's gutter"
```

---

### Task 2: `d` is done

**Files:**
- Modify: `internal/tui/loops.go` (the `case "x":` in `loopsKey`), `internal/tui/writes.go:26`, `internal/tui/app.go` (`tabFooters`, `tabBindings`), `internal/tree/close.go:11` (comment only)
- Test: `internal/tui/writes_test.go`, goldens under `internal/tui/testdata/`

- [ ] **Step 1: Move the tests to the new key**

In `internal/tui/writes_test.go`, change every `"x"` passed to `press` or listed in a `[]string{"x", "w", "e", "n"}` loop to `"d"` — lines 58, 83, 110, 233, 253, 276 and 604 — and fix the comments and failure messages beside them that name `x`. Then add:

```go
// x is no longer the close: a press of it writes nothing.
func TestXNoLongerCloses(t *testing.T) {
	m, path := working(t)

	press(t, m, "G", "x")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("x wrote %v, want nothing", log)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui -run 'TestDone|TestUndoOfADone|TestXNoLonger|TestLoopsWrites'`
Expected: FAIL — `d` writes nothing yet and `x` still closes.

- [ ] **Step 3: Implement**

`internal/tui/loops.go`, in `loopsKey`: change `case "x":` to `case "d":`.

`internal/tui/writes.go`: change the hint `"x closes an item"` to `"d closes an item"`.

`internal/tui/app.go`:
- `tabFooters[tabLoops]`: replace the leading `x done` with `d done`.
- `tabBindings[tabLoops]`: replace `{[]string{"x"}, "x", "done"}` with `{[]string{"d"}, "d", "done"}`.

`internal/tree/close.go`: in the `GuardClose` comment, change ``the app's `x` `` to ``the app's `d` ``.

- [ ] **Step 4: Regenerate the footer goldens and read the diff**

Run: `go test ./internal/tui -run Golden -update && git diff --stat internal/tui/testdata`
Expected: only the `TestGoldenLoops*` files change, each by its footer line (`x done` → `d done`). The `TestGoldenMove*`, `TestGoldenScan*`, `TestGoldenChrome*` and `TestGoldenReview*` files do not.

- [ ] **Step 5: Run the package**

Run: `go test ./internal/tui`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui internal/tree/close.go
git commit -m "feat(tui): d closes an item, leaving x free"
```

---

### Task 3: Marks — `x`, `X`, the header count and the footer

**Files:**
- Create: `internal/tui/marks.go`, `internal/tui/marks_test.go`
- Modify: `internal/tui/loops.go` (`loopsModel`, `loopsLoaded`, `loopsKey`, `loopsTree`, `loopsCounts`), `internal/tui/app.go` (`tabBindings`, `footer`)

**Interfaces:**
- Consumes: `Tree[T].Marked`, `markGlyph` (Task 1).
- Produces:
  - `loopsModel.marked map[string]bool`
  - `func (m Model) toggleMark(t Tree[events.Item]) (Model, tea.Cmd, bool)`
  - `func (m Model) markedItems() []events.Item` — the marked items in log order
  - `func (m Model) targets(t Tree[events.Item], action string) (Model, []events.Item, bool)` — the marked items, else the cursor row
  - `func (m Model) topmost(items []events.Item) []events.Item`
  - `func idsOf(items []events.Item) []string`
  - `const markKeys`

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/marks_test.go`:

```go
package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"github.com/brentkeller/waid/internal/events"
)

// markedIds are the ids carrying a mark, sorted so an assertion does not depend on map order.
func markedIds(m Model) []string {
	ids := []string{}
	for id := range m.loops.marked {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// x marks the row under the cursor and steps down, so a run of rows is marked by repeating the key.
func TestXMarksTheRowAndStepsDown(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "x")

	if got, want := markedIds(m), []string{"nktt", "sga9"}; !slices.Equal(got, want) {
		t.Errorf("marked %v, want %v", got, want)
	}
	if log := written(t, path); len(log) != 0 {
		t.Errorf("marking wrote %v, want nothing", log)
	}

	view := plain(m.View())
	for _, id := range []string{"sga9", "nktt"} {
		if row := rowFor(t, view, id); !strings.HasPrefix(row, markGlyph) {
			t.Errorf("the row for %s is %q, want the mark in its gutter", id, row)
		}
	}
	if row := rowFor(t, view, "9xz1"); strings.HasPrefix(row, markGlyph) {
		t.Errorf("an unmarked row drew the mark: %q", row)
	}
}

// A second press on a marked row takes the mark off, and steps down all the same.
func TestXTakesTheMarkOffAMarkedRow(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "x", "up", "x")

	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v, want nothing", got)
	}
	if item, _ := m.loopsTree(m.viewWidth()).SelectedItem(); item.Id != "nktt" {
		t.Errorf("the cursor is on %q, want it stepped down to nktt", item.Id)
	}
}

// The last row has nowhere to step to, so the cursor stays on it.
func TestXStaysOnTheLastRow(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "G", "x")

	if got, want := markedIds(m), []string{"4h2k"}; !slices.Equal(got, want) {
		t.Errorf("marked %v, want %v", got, want)
	}
	if item, _ := m.loopsTree(m.viewWidth()).SelectedItem(); item.Id != "4h2k" {
		t.Errorf("the cursor is on %q, want it left on the last row", item.Id)
	}
}

// A mark covers the item it is on: marking a fold marks nothing filed under it.
func TestXOnAFoldMarksTheFoldAlone(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "x")

	if got, want := markedIds(m), []string{"vq2n"}; !slices.Equal(got, want) {
		t.Errorf("marked %v, want %v", got, want)
	}
	if line := lineFor(t, plain(m.View()), "Localized notifications"); !strings.HasPrefix(line, markGlyph) {
		t.Errorf("the fold is drawn %q, want the mark in its gutter", line)
	}
}

// The bucket is not an item, so it takes no mark and the footer says why.
func TestXIsInertOnTheBucket(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "down", "down", "x")

	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v, want nothing", got)
	}
	if !strings.Contains(m.hint, "project") {
		t.Errorf("hint = %q, want it to name the row", m.hint)
	}
}

// X clears every mark at once.
func TestShiftXClearsTheMarks(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "x", "x", "X")

	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v after X, want nothing", got)
	}
}

// Marks are keyed by id, so they hold while the list is filtered, cleared, re-gated and re-read —
// and esc, which clears the query, leaves them alone.
func TestMarksHoldAcrossFiltersAndReloads(t *testing.T) {
	m, _ := working(t)
	m, _ = press(t, m, "down", "x")

	m, _ = press(t, m, "/", "s", "p", "i", "k", "e", "enter")
	if view := plain(m.View()); !strings.Contains(view, "1 marked · 1 item") {
		t.Errorf("the header does not count the mark the query hides:\n%s", view)
	}

	m, _ = press(t, m, "esc", "s", "h", "left")
	next, _ := m.Update(m.loops.load())
	m = next.(Model)

	if got, want := markedIds(m), []string{"sga9"}; !slices.Equal(got, want) {
		t.Errorf("marked %v, want %v", got, want)
	}
}

// A read that no longer holds a marked item drops its mark, so the count stays honest.
func TestAReloadDropsMarksForItemsTheLogNoLongerHolds(t *testing.T) {
	m, _ := working(t)
	m, _ = press(t, m, "down", "x")

	msg := loopsFixture()
	msg.items = slices.DeleteFunc(msg.items, func(item events.Item) bool { return item.Id == "sga9" })
	next, _ := m.Update(msg)

	if got := markedIds(next.(Model)); len(got) != 0 {
		t.Errorf("marked %v, want the mark dropped with the item", got)
	}
}

// While anything is marked the header counts it and the footer offers the keys that take the set.
func TestMarksChangeTheHeaderAndTheFooter(t *testing.T) {
	m, _ := working(t)

	bare := plain(m.View())
	if strings.Contains(bare, "marked") || strings.Contains(bare, "X clear") {
		t.Errorf("an unmarked list mentions marks:\n%s", bare)
	}

	m, _ = press(t, m, "down", "x", "x")
	view := plain(m.View())
	if !strings.Contains(view, "2 marked · 6 items") {
		t.Errorf("the header does not count the marks:\n%s", view)
	}
	if !strings.Contains(view, markKeys) {
		t.Errorf("the footer does not offer the batch keys:\n%s", view)
	}
}

// x and X are Loops keys: pressed elsewhere they are inert and the footer names the tab.
func TestMarkKeysBelongToLoops(t *testing.T) {
	m, _ := working(t)

	for _, pressed := range []string{"x", "X"} {
		next, _ := press(t, m, "2", pressed)
		if !strings.Contains(next.hint, "Loops key") {
			t.Errorf("%s on Repos left the hint %q, want it to name Loops", pressed, next.hint)
		}
	}
}

// A marked list at the two widths the goldens pin: the gutter glyph on a fold and on a leaf, and the
// count beside the item count.
func TestGoldenLoopsMarkedAt80Columns(t *testing.T) { goldenMarked(t, 80) }

func TestGoldenLoopsMarkedAt140Columns(t *testing.T) { goldenMarked(t, 140) }

func goldenMarked(t *testing.T, width int) {
	t.Helper()

	m, _ := press(t, looped(t, width, loopsFixture()), "x", "x")
	teatest.RequireEqualOutput(t, []byte(plain(m.View())))
}

// The keys that address one item keep addressing the row under the cursor while rows are marked.
func TestSingleItemKeysIgnoreTheMarks(t *testing.T) {
	m, _ := working(t)

	// Marking sga9 steps the cursor onto nktt.
	m, _ = press(t, m, "down", "x", "e")

	if m.prompt.kind != promptRename || m.prompt.subject != "nktt" {
		t.Errorf("e opened prompt %d on %q, want the title of the cursor row nktt", m.prompt.kind, m.prompt.subject)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui -run 'Mark|TestX|TestShiftX'`
Expected: build failure — `m.loops.marked undefined`, `undefined: markKeys`.

- [ ] **Step 3: Create `internal/tui/marks.go`**

```go
package tui

import (
	"maps"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brentkeller/waid/internal/events"
)

// A mark says which before a write says what: rows are marked one at a time, and a move, a close, a
// waiting or a tag made while any are marked is made against all of them. Marks are held by item id
// rather than by row, so they survive everything that re-sorts or redraws the list — a reload, a
// fold, a query, the status row.

// markKeys are the footer's hints while anything is marked: the four writes that take the set, and
// the two keys that change it.
const markKeys = "m move · d done · w waiting · t tag · x mark · X clear"

// toggleMark sets or clears the mark on the row under the cursor and steps down a row, so a run of
// rows is marked by repeating the key. A row holding no item takes no mark, and the footer says so.
//
// The set is replaced rather than written through: Model is copied by value through the update loop,
// and two copies sharing one map would see each other's marks.
func (m Model) toggleMark(t Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, item, ok := m.loopTarget(t, "x marks an item")
	if !ok {
		return m, nil, true
	}

	marked := make(map[string]bool, len(m.loops.marked)+1)
	maps.Copy(marked, m.loops.marked)
	if marked[item.Id] {
		delete(marked, item.Id)
	} else {
		marked[item.Id] = true
	}
	m.loops.marked = marked

	t.Down()
	m.loops.cursor = t.Cursor
	return m, nil, true
}

// markedItems are the loaded items carrying a mark, in the order the log holds them — which is the
// order a write against the set appends in, whatever order the marks were made in.
func (m Model) markedItems() []events.Item {
	if len(m.loops.marked) == 0 {
		return nil
	}

	var items []events.Item
	for _, item := range m.loops.items {
		if m.loops.marked[item.Id] {
			items = append(items, item)
		}
	}
	return items
}

// keptMarks are the marks whose items a fresh read still holds.
func (m Model) keptMarks(items []events.Item) map[string]bool {
	if len(m.loops.marked) == 0 {
		return nil
	}

	kept := map[string]bool{}
	for _, item := range items {
		if m.loops.marked[item.Id] {
			kept[item.Id] = true
		}
	}
	return kept
}

// targets are the items a write addresses: every marked item, or the row under the cursor when
// nothing is marked. A marked item the query or the status row is hiding is a target all the same —
// the header's count is what keeps it in view.
func (m Model) targets(t Tree[events.Item], action string) (Model, []events.Item, bool) {
	if marked := m.markedItems(); len(marked) > 0 {
		return m, marked, true
	}

	m, item, ok := m.loopTarget(t, action)
	if !ok {
		return m, nil, false
	}
	return m, []events.Item{item}, true
}

// topmost drops every item that sits beneath another item of the set. A move carries a subtree with
// its root, so an item written a parent of its own as well would be pulled out from under the
// ancestor that was already taking it along.
func (m Model) topmost(items []events.Item) []events.Item {
	chosen := make(map[string]bool, len(items))
	for _, item := range items {
		chosen[item.Id] = true
	}

	state := events.State{Items: m.loops.items}
	var kept []events.Item
	for _, item := range items {
		carried := false
		for _, ancestor := range state.Ancestors(item.Id) {
			if chosen[ancestor.Id] {
				carried = true
				break
			}
		}
		if !carried {
			kept = append(kept, item)
		}
	}
	return kept
}

// idsOf are the ids of a list of items, in its order.
func idsOf(items []events.Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Id)
	}
	return ids
}
```

- [ ] **Step 4: Wire it into Loops**

`internal/tui/loops.go`:

Add to `loopsModel`, after `expanded`:

```go
	// marked is the ids of the rows carrying a mark, which a move, a close, a waiting or a tag is made
	// against while it holds any. It is keyed by id so a reload or a filter cannot move a mark.
	marked map[string]bool
```

In `loopsLoaded`, before the line that sets `m.loops.items`:

```go
	m.loops.marked = m.keptMarks(msg.items)
```

In `loopsKey`, add beside the other write keys:

```go
	case "x":
		return m.toggleMark(tree)
	case "X":
		m.loops.marked = nil
		return m, nil, true
```

In `loopsTree`, add to the `Tree[events.Item]` literal:

```go
		Marked: func(row Row[events.Item]) bool { return row.node && m.loops.marked[row.Node.Id] },
```

Replace the body of `loopsCounts`:

```go
func (m Model) loopsCounts() string {
	items := plural(len(m.loopsVisible()), "item")
	if marked := len(m.loops.marked); marked > 0 {
		return fmt.Sprintf("%d marked · %s", marked, items)
	}
	return items
}
```

and extend its comment with: `While anything is marked the count of marks leads it, since a mark can be on a row the list is not showing.`

`internal/tui/app.go`:

In `tabBindings[tabLoops]`, insert after the `d` row:

```go
		{[]string{"x"}, "x", "mark the row, for a write against several"},
		{[]string{"X"}, "X", "clear the marks"},
```

In `footer`, after the `moving.active()` check:

```go
	} else if m.tab == tabLoops && len(m.loops.marked) > 0 {
		hints = markKeys
	}
```

(restructure the existing `if` into `if … { … } else if … { … }`), and extend the comment above it with: `Marked rows change what four of the tab's keys act on, so the hints name those keys while any row is marked.`

- [ ] **Step 5: Generate the new goldens, then run the package**

Run: `go test ./internal/tui -run TestGoldenLoopsMarked -update`, open both new files under `internal/tui/testdata/` and confirm `●` leads the `Localized notifications` and `sga9` lines and the header reads `2 marked · 6 items`.

Run: `go test ./internal/tui`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): x marks a row and X clears the marks"
```

---

### Task 4: Undo entries and receipts carry several items

**Files:**
- Modify: `internal/tui/undo.go`, `internal/tui/receipt.go`
- Test: `internal/tui/undo_test.go`, `internal/tui/receipt_test.go`

**Interfaces:**
- Produces:
  - `undoEntry.items []events.Item` (replaces `item *events.Item`)
  - `func mergeUndo(entries []undoEntry) undoEntry`
  - `receipt.summary bool`, skipped by `replay`

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/undo_test.go`:

```go
// A batch leaves one entry, so one press of u reverses every write in it and leaves one receipt.
func TestUndoOfABatchReversesEveryWriteInIt(t *testing.T) {
	m, path := working(t)

	first, _ := m.loadedItem("sga9")
	second, _ := m.loadedItem("nktt")
	m = m.pushUndo(mergeUndo([]undoEntry{undoHeading(first), undoHeading(second)}))

	m, _ = press(t, m, "u")

	assertLog(t, path, []string{headingLine("sga9", false), headingLine("nktt", false)})
	if view := plain(m.View()); !strings.Contains(view, "restored 2 items") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
	if len(m.undos) != 0 {
		t.Errorf("the undo stack holds %d entries after the undo, want it empty", len(m.undos))
	}
}

// A batch of one is the entry it was built from, receipt and all.
func TestABatchOfOneUndoIsThatUndo(t *testing.T) {
	m, _ := working(t)
	item, _ := m.loadedItem("sga9")

	if got, want := mergeUndo([]undoEntry{undoDone(item)}).receipt.line(), undoDone(item).receipt.line(); got != want {
		t.Errorf("the merged receipt is %q, want %q", got, want)
	}
}
```

Append to `internal/tui/receipt_test.go`:

```go
// A summary is what the footer says about a batch. The replay already holds a line for every item
// the batch wrote, so it leaves the summary out.
func TestTheReplayLeavesOutASummary(t *testing.T) {
	receipts := []receipt{
		{verb: "closed", subject: "sga9", detail: "one"},
		{verb: "closed", subject: "nktt", detail: "two"},
		{verb: "closed", subject: "2 items", summary: true},
	}

	if got, want := replay(receipts), "closed   sga9  one\nclosed   nktt  two"; got != want {
		t.Errorf("replay =\n%s\nwant\n%s", got, want)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui -run 'Batch|TestTheReplayLeavesOut'`
Expected: build failure — `undefined: mergeUndo`, `unknown field summary`.

- [ ] **Step 3: Implement**

`internal/tui/receipt.go` — add to `receipt`:

```go
	// summary marks the receipt a write against several items leaves for the footer. Each item's own
	// receipt is recorded ahead of it, so the replay prints those and skips this.
	summary bool
```

and in `replay`, skip them:

```go
	for _, r := range receipts {
		if r.summary {
			continue
		}
```

`internal/tui/undo.go`:

Replace the `item *events.Item` field and its comment with:

```go
	// items are the loaded copies the list is left showing once the inverse has been written, put back
	// beside the log so the rows move in the frame the undo was pressed in — the state before the write
	// for one that changed an item, and the closed item for an add, which nothing removes. A write on
	// an item no tab has loaded leaves none, and a write against several items leaves one for each.
	items []events.Item
```

Change `restoring` to `e.items = []events.Item{item}`.

In `undoAdd`, `undoDone`, `undoWaiting`, `undoHeading`, `undoTags` and `undoRefile`, change `item: &closed` / `item: &item` to `items: []events.Item{closed}` / `items: []events.Item{item}`.

Add after `undoRefile`:

```go
// mergeUndo is the one entry a write against several items leaves, so a single undo reverses the
// whole of it: every inverse event in the order the writes were made, every item put back, and a
// receipt that counts them. A batch of one is the entry it was built from.
func mergeUndo(entries []undoEntry) undoEntry {
	if len(entries) == 1 {
		return entries[0]
	}

	merged := undoEntry{receipt: receipt{verb: "restored", subject: plural(len(entries), "item")}}
	for _, entry := range entries {
		merged.events = append(merged.events, entry.events...)
		merged.items = append(merged.items, entry.items...)
	}
	return merged
}
```

In `undo`, replace the `if entry.item != nil { … }` block with:

```go
	for _, item := range entry.items {
		restored := item
		restored.Updated = ts
		m = m.applyItem(restored)
	}
```

- [ ] **Step 4: Run the package**

Run: `go test ./internal/tui`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "refactor(tui): an undo entry and a receipt can stand for several items"
```

---

### Task 5: `commit`, and `d` against the marked set

**Files:**
- Modify: `internal/tui/writes.go`
- Test: `internal/tui/marks_test.go`

**Interfaces:**
- Consumes: `targets`, `idsOf` (Task 3); `mergeUndo`, `receipt.summary` (Task 4).
- Produces:

```go
type itemWrite struct {
	event   events.WaidEvent
	after   events.Item
	receipt receipt
	inverse undoEntry
}

func (m Model) commit(action string, writes []itemWrite, summary receipt) (Model, tea.Cmd)
```

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/marks_test.go`:

```go
// d closes every marked item in the order the log holds them, leaves a line per item for the replay
// and a count for the footer, and clears the marks it answered.
func TestDoneClosesEveryMarkedItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "x", "d")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"close","id":"nktt"}`,
		`{"ts":"` + ts + `","ev":"close","id":"sga9"}`,
	})

	view := plain(m.View())
	if listed(view, "sga9") || listed(view, "nktt") {
		t.Errorf("a closed item is still in the list:\n%s", view)
	}
	if !strings.Contains(view, "closed 2 items") {
		t.Errorf("the footer carries no summary of the batch:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v after the batch, want the marks cleared", got)
	}

	replayed := replay(m.receipts)
	if !strings.Contains(replayed, "nktt") || !strings.Contains(replayed, "sga9") || strings.Contains(replayed, "2 items") {
		t.Errorf("the replay is\n%s\nwant a line per item and no summary", replayed)
	}
}

// One u reverses the whole batch, and does not put the marks back.
func TestUndoOfABatchDoneReopensEveryItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "x", "d", "u")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"close","id":"nktt"}`,
		`{"ts":"` + ts + `","ev":"close","id":"sga9"}`,
		`{"ts":"` + ts + `","ev":"reopen","id":"nktt"}`,
		`{"ts":"` + ts + `","ev":"reopen","id":"sga9"}`,
	})

	view := plain(m.View())
	if !listed(view, "sga9") || !listed(view, "nktt") {
		t.Errorf("a reopened item did not come back:\n%s", view)
	}
	if !strings.Contains(view, "restored 2 items") {
		t.Errorf("the footer carries no receipt for the undo:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("the undo put back the marks %v", got)
	}
}

// The close guard judges the set as one: a heading marked with everything open beneath it closes.
func TestDoneClosesAHeadingMarkedWithItsDescendants(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "x", "x", "x", "d")

	if log := written(t, path); len(log) != 3 {
		t.Errorf("the batch wrote %v, want the heading and both items closed", log)
	}
}

// A heading marked with a descendant left out is refused whole: nothing is written, and the marks
// stand so the set can be put right.
func TestDoneRefusesAMarkedHeadingWithADescendantLeftOut(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "x", "x", "d")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("a refused batch wrote %v", log)
	}
	if !strings.Contains(m.hint, "cannot close vq2n") {
		t.Errorf("hint = %q, want the guard's refusal", m.hint)
	}
	if got := markedIds(m); len(got) != 2 {
		t.Errorf("marked %v after a refusal, want both marks standing", got)
	}
}

// An item already closed is skipped, and a set holding nothing else is a hint and no write.
func TestDoneSkipsMarkedItemsAlreadyClosed(t *testing.T) {
	m, path := working(t)

	m.loops.marked = map[string]bool{"shut": true, "p0rt": true}
	mixed, _ := press(t, m, "d")
	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"close","id":"p0rt"}`})
	if view := plain(mixed.View()); !strings.Contains(view, "closed p0rt") {
		t.Errorf("a batch that closed one item did not receipt it as one:\n%s", view)
	}

	m.loops.marked = map[string]bool{"shut": true}
	closed, _ := press(t, m, "d")
	if log := written(t, path); len(log) != 1 {
		t.Errorf("a set of closed items wrote %v", log[1:])
	}
	if !strings.Contains(closed.hint, "already closed") {
		t.Errorf("hint = %q, want it to say the item is closed", closed.hint)
	}
}

// A mark the query is hiding is still part of the set, so the batch closes it.
func TestDoneClosesAMarkedItemTheFilterHides(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "/", "s", "p", "i", "k", "e", "enter", "d")

	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"close","id":"sga9"}`})
}

// A write that cannot land says so, records nothing, and leaves the marks standing for a retry.
func TestAFailedBatchKeepsItsMarks(t *testing.T) {
	m, _ := working(t)
	m, _ = press(t, m, "down", "x", "x")

	// A directory is not a file a line can be appended to.
	m.opts.Cfg.EventsPath = t.TempDir()
	m, _ = press(t, m, "d")

	if !strings.Contains(m.hint, "done failed") {
		t.Errorf("hint = %q, want it to say the write failed", m.hint)
	}
	if len(m.receipts) != 0 {
		t.Errorf("a failed batch left %d receipts", len(m.receipts))
	}
	if got := markedIds(m); len(got) != 2 {
		t.Errorf("marked %v after a failed batch, want both marks standing", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui -run 'TestDone|TestUndoOfABatchDone|TestAFailedBatch'`
Expected: FAIL — `d` closes only the cursor row.

- [ ] **Step 3: Implement**

In `internal/tui/writes.go`, replace the file's opening comment paragraph's phrase "the four Loops binds to the row under the cursor" with "the ones Loops binds to the row under the cursor or to the rows carrying a mark".

Add after that comment block:

```go
// itemWrite is one item's share of a write: the event appended for it, the item as the write leaves
// it, the receipt it is recorded by, and the entry that reverses it.
type itemWrite struct {
	event   events.WaidEvent
	after   events.Item
	receipt receipt
	inverse undoEntry
}

// commit lands a write against one item or several. Each item's event is appended in turn, its
// loaded copy replaced and its receipt recorded, and the inverses are pushed as a single entry so one
// undo reverses the whole of it. A write against several also records summary, which is what the
// footer shows in place of the last item's own receipt.
//
// An append that fails stops the write there. What landed is kept and can be undone, the hint names
// the failure, and the marks are left standing so the rest can be tried again.
func (m Model) commit(action string, writes []itemWrite, summary receipt) (Model, tea.Cmd) {
	var inverses []undoEntry
	failed := ""
	for _, write := range writes {
		ts, err := events.Append(m.opts.Cfg.EventsPath, write.event, m.now())
		if err != nil {
			failed = writeFailed(action, err)
			break
		}

		landed := write.after
		landed.Updated = ts
		m = m.applyItem(landed).record(write.receipt)
		inverses = append(inverses, write.inverse)
	}

	if len(inverses) > 0 {
		m = m.pushUndo(mergeUndo(inverses))
	}
	if failed != "" {
		m.hint = failed
		return m, nil
	}

	if len(writes) > 1 {
		summary.summary = true
		m = m.record(summary)
	}
	m.loops.marked = nil
	return m, nil
}
```

Replace `doneSelected` and its comment:

```go
// doneSelected closes the marked items, or the item under the cursor when nothing is marked — which
// is the only way out of the list (§1.1).
func (m Model) doneSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, items, ok := m.targets(tree, "d closes an item")
	if !ok {
		return m, nil, true
	}

	// The done and all segments show items that are already closed, and a second close would be a log
	// line and a receipt for nothing (§4). A set holding some is closed around them.
	var open []events.Item
	for _, item := range items {
		if item.Status != events.StatusDone {
			open = append(open, item)
		}
	}
	if len(open) == 0 {
		m.hint = "every marked item is already closed"
		if len(items) == 1 {
			m.hint = items[0].Id + " is already closed"
		}
		return m, nil, true
	}

	// A heading closes only once the work beneath it is finished (§5). The guard is the one `waid
	// done` calls, so the two surfaces cannot disagree about when that is, and it judges the set as
	// one: a heading marked alongside everything open beneath it closes with it.
	if err := itemtree.GuardCloses(events.State{Items: m.loops.items}, idsOf(open)); err != nil {
		m.hint = err.Error()
		return m, nil, true
	}

	writes := make([]itemWrite, 0, len(open))
	for _, item := range open {
		closed := item
		closed.Status = events.StatusDone
		writes = append(writes, itemWrite{
			event:   events.CloseEvent{Ev: "close", Id: item.Id},
			after:   closed,
			receipt: receipt{verb: "closed", subject: item.Id, detail: item.Title},
			inverse: undoDone(item),
		})
	}

	m, cmd := m.commit("done", writes, receipt{verb: "closed", subject: plural(len(writes), "item")})
	return m, cmd, true
}
```

- [ ] **Step 4: Run the package**

Run: `go test ./internal/tui`
Expected: PASS, including every pre-existing single-item done test.

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): d closes every marked item in one write"
```

---

### Task 6: `w` against the marked set

**Files:**
- Modify: `internal/tui/prompt.go`, `internal/tui/writes.go`
- Test: `internal/tui/marks_test.go`

**Interfaces:**
- Consumes: `targets`, `idsOf`, `commit`, `itemWrite`.
- Produces:
  - `prompt.subjects []string`
  - `func promptSubject(items []events.Item) string` — the id of one item, or `(N items)`
  - `func (m Model) waitOn(ids []string, who string) (Model, tea.Cmd)` (was `id string`)

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/marks_test.go`:

```go
// w asks once who the marked items are waiting on and writes the answer to each.
func TestWaitingWritesEveryMarkedItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "x", "w")
	if view := plain(m.View()); !strings.Contains(view, "waiting on (2 items)") {
		t.Fatalf("the prompt does not say how many items it is asking about:\n%s", view)
	}

	m, _ = press(t, m, "maria", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"nktt","status":"waiting","waitingOn":"maria"}`,
		`{"ts":"` + ts + `","ev":"update","id":"sga9","status":"waiting","waitingOn":"maria"}`,
	})

	view := plain(m.View())
	for _, id := range []string{"sga9", "nktt"} {
		if row := rowFor(t, view, id); !strings.Contains(row, "@maria") {
			t.Errorf("the row for %s is %q, want it waiting on maria", id, row)
		}
	}
	if !strings.Contains(view, "waiting 2 items  maria") {
		t.Errorf("the footer carries no summary of the batch:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v after the batch, want the marks cleared", got)
	}
}

// An abandoned prompt writes nothing and leaves the marks standing.
func TestAnAbandonedBatchWaitingKeepsItsMarks(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "x", "w", "esc")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("an abandoned prompt wrote %v", log)
	}
	if got := markedIds(m); len(got) != 2 {
		t.Errorf("marked %v, want both marks standing", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui -run 'Waiting'`
Expected: FAIL — the prompt names one id and one line is written.

- [ ] **Step 3: Implement**

`internal/tui/prompt.go` — add to `prompt`, after `subject`:

```go
	// subjects are the items a waiting or a tags answer is written to: one for a prompt opened on the
	// row under the cursor, and every marked item otherwise. subject is what the footer prints for
	// them, which is an id for one and a count for several.
	subjects []string
```

In `commitPrompt`, change the waiting case to:

```go
	case promptWaiting:
		return m.waitOn(answered.subjects, value)
```

`internal/tui/writes.go` — add:

```go
// promptSubject is what a prompt's label names: the id of the one item it is asking about, or how
// many it is asking about at once.
func promptSubject(items []events.Item) string {
	if len(items) == 1 {
		return items[0].Id
	}
	return "(" + plural(len(items), "item") + ")"
}
```

Replace `waitingSelected` and `waitOn` with their comments:

```go
// waitingSelected asks who the marked items — or the item under the cursor — are waiting on. The
// name is what the status is for, since an item waiting on nobody is just open, so it is collected
// before anything is written.
func (m Model) waitingSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, items, ok := m.targets(tree, "w marks an item waiting")
	if !ok {
		return m, nil, true
	}

	m.prompt = m.open(prompt{
		kind: promptWaiting, label: "waiting on", subject: promptSubject(items), subjects: idsOf(items),
	})
	return m, nil, true
}

// waitOn writes the status and the name together to each item, since an update carrying one without
// the other would leave the item describing half a state.
func (m Model) waitOn(ids []string, who string) (Model, tea.Cmd) {
	var writes []itemWrite
	for _, id := range ids {
		item, loaded := m.loadedItem(id)
		if !loaded {
			continue
		}

		waiting := item
		waiting.Status, waiting.WaitingOn = events.StatusWaiting, &who
		writes = append(writes, itemWrite{
			event:   events.UpdateEvent{Ev: "update", Id: id, Status: events.StatusWaiting, WaitingOn: &who},
			after:   waiting,
			receipt: receipt{verb: "waiting", subject: id, detail: who},
			inverse: undoWaiting(item),
		})
	}
	if len(writes) == 0 {
		m.hint = noItems
		return m, nil
	}

	return m.commit("waiting", writes, receipt{verb: "waiting", subject: plural(len(writes), "item"), detail: who})
}
```

- [ ] **Step 4: Run the package**

Run: `go test ./internal/tui`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): w sets every marked item waiting on one name"
```

---

### Task 7: `t` adds tags to the marked set

**Files:**
- Modify: `internal/tui/prompt.go`, `internal/tui/writes.go`
- Test: `internal/tui/marks_test.go`

**Interfaces:**
- Consumes: `targets`, `idsOf`, `promptSubject`, `prompt.subjects`, `commit`, `itemWrite`.
- Produces: `func (m Model) setTags(ids []string, answer string) (Model, tea.Cmd)` (was `id string`).

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/marks_test.go`:

```go
// With several items marked, t opens empty and adds what is typed to the tags each item already
// carries, since no one line could stand for several different sets.
func TestTagAddsToEveryMarkedItem(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "x", "t")
	if got := m.prompt.text(); got != "" {
		t.Errorf("the batch prompt opened on %q, want it empty", got)
	}
	if view := plain(m.View()); !strings.Contains(view, "add tags (2 items)") {
		t.Fatalf("the prompt does not say it adds to several items:\n%s", view)
	}

	m, _ = press(t, m, "bug", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"nktt","tags":["promoted","bug"]}`,
		`{"ts":"` + ts + `","ev":"update","id":"sga9","tags":["bug"]}`,
	})

	view := plain(m.View())
	if !strings.Contains(view, "tagged 2 items  bug") {
		t.Errorf("the footer carries no summary of the batch:\n%s", view)
	}
	if got := markedIds(m); len(got) != 0 {
		t.Errorf("marked %v after the batch, want the marks cleared", got)
	}

	m, _ = press(t, m, "u")
	view = plain(m.View())
	if row := rowFor(t, view, "nktt"); !strings.Contains(row, "[promoted]") {
		t.Errorf("the undone row is %q, want only the tag it carried before", row)
	}
	if row := rowFor(t, view, "sga9"); strings.Contains(row, "bug") {
		t.Errorf("the undone row is %q, want the added tag gone", row)
	}
}

// A blank answer to the batch prompt is an abandoned edit — it must not clear every item's tags the
// way a blank answer clears one item's — and so is an answer holding only separators.
func TestABlankBatchTagAnswerWritesNothing(t *testing.T) {
	for _, answer := range [][]string{{"enter"}, {", ,", "enter"}} {
		m, path := working(t)

		m, _ = press(t, m, "down", "x", "x", "t")
		m, _ = press(t, m, answer...)

		if log := written(t, path); len(log) != 0 {
			t.Errorf("the answer %v wrote %v", answer, log)
		}
		if got := markedIds(m); len(got) != 2 {
			t.Errorf("the answer %v left the marks %v, want both standing", answer, got)
		}
	}
}

// One marked item is edited the way the cursor row is: the prompt opens on its set and replaces it.
func TestTagOnOneMarkedItemReplacesItsSet(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "down", "x", "g", "t")
	if got, want := m.prompt.text(), "promoted"; got != want {
		t.Fatalf("the prompt opened on %q, want the marked item's tags %q", got, want)
	}

	press(t, m, "ctrl+u", "bug", "enter")
	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"update","id":"nktt","tags":["bug"]}`})
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui -run 'Tag'`
Expected: FAIL on the three new tests; the tests in `tags_test.go` still pass.

- [ ] **Step 3: Implement**

`internal/tui/prompt.go`, in `commitPrompt`: replace the empty-answer guard and the tags case.

```go
	// An empty answer is an abandoned edit rather than a write of nothing — except for the tags of one
	// item, where it is a write of nothing in the literal sense: an item with no tags is a state to
	// want, and esc is what abandons an edit. A tags answer for several items adds to each rather than
	// replacing, so a blank one adds nothing and is abandoned like the rest.
	value := strings.TrimSpace(answered.text())
	clears := answered.kind == promptTags && len(answered.subjects) == 1
	if value == "" && !clears {
		return m, nil
	}
```

```go
	case promptTags:
		return m.setTags(answered.subjects, value)
```

`internal/tui/writes.go` — add `"slices"` to the imports, and replace `tagSelected` and `setTags` with their comments:

```go
// tagSelected edits the tags on the item under the cursor, or adds tags to the marked items.
//
// For one item the input opens on the set it already carries, so the whole set is in front of
// whoever is editing it — which is what makes a replacement safe here and not on the command line,
// where `waid tag` names a change instead (§7). Several items carry several sets and no one line can
// stand for them, so that input opens empty and what is typed is added to each.
func (m Model) tagSelected(tree Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, items, ok := m.targets(tree, "t tags an item")
	if !ok {
		return m, nil, true
	}

	asked := prompt{kind: promptTags, label: "add tags", subject: promptSubject(items), subjects: idsOf(items)}
	if len(items) == 1 {
		// The set is seeded with a space after each comma, since it is being read as well as typed. The
		// spaces are trimmed back off on the way to the log.
		asked.label, asked.prior = "tags", strings.Join(items[0].Tags, ", ")
	}

	m.prompt = m.open(asked)
	return m, nil, true
}

// setTags writes the typed tags: one item has its set replaced by them, and several each have them
// added to the set they carry. Either way the event carries the item's whole set rather than what
// changed, so the fold reads one line and `waid tag` writes the same shape (§1).
//
// The write is made whatever the item already held, for the reason the heading toggle is: the log is
// a history of what was asked for, and a key that sometimes wrote and sometimes did not would leave
// `u` guessing which press it was reversing. An answer that adds nothing to several items is the
// exception, since it asked for nothing.
func (m Model) setTags(ids []string, answer string) (Model, tea.Cmd) {
	typed := events.NormalizeTags(strings.Split(answer, ","))
	adding := len(ids) > 1
	if adding && len(typed) == 0 {
		return m, nil
	}

	var writes []itemWrite
	for _, id := range ids {
		item, loaded := m.loadedItem(id)
		if !loaded {
			continue
		}

		tags := typed
		if adding {
			tags = events.NormalizeTags(slices.Concat(item.Tags, typed))
		}

		tagged := item
		tagged.Tags = tags
		writes = append(writes, itemWrite{
			event:   events.TagsEvent{Ev: "update", Id: id, Tags: tags},
			after:   tagged,
			receipt: tagsReceipt(tagged),
			inverse: undoTags(item),
		})
	}
	if len(writes) == 0 {
		m.hint = noItems
		return m, nil
	}

	summary := receipt{verb: "tagged", subject: plural(len(writes), "item"), detail: strings.Join(typed, ", ")}
	return m.commit("tags", writes, summary)
}
```

- [ ] **Step 4: Run the package**

Run: `go test ./internal/tui`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): t adds tags to every marked item"
```

---

### Task 8: `m` moves the marked set

**Files:**
- Modify: `internal/tui/move.go`, `internal/tui/refile.go`
- Test: `internal/tui/move_test.go`

**Interfaces:**
- Consumes: `targets`, `topmost`, `idsOf`, `commit`, `itemWrite`.
- Produces:
  - `moveModel.subjects []string` (replaces `subject string`)
  - `func (m Model) lifted() []string`
  - `func sharedParent(items []events.Item) *string`
  - `func (m Model) refile(ids []string, parent *string) (Model, tea.Cmd)` (was `id string`)

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/move_test.go`:

```go
// m with rows marked lifts all of them into one picker and files each under the row it is dropped on.
func TestMoveFilesEveryMarkedItem(t *testing.T) {
	m, path := working(t)

	// Four rows down is 9xz1, and the row under it is p0rt: both sit at the top level, so the picker
	// opens on the row that means it and one step down is the fold.
	m, _ = press(t, m, "down", "down", "down", "down", "x", "x", "m")
	if view := plain(m.View()); !strings.Contains(view, "moving  2 items") {
		t.Fatalf("the header does not count what is being moved:\n%s", view)
	}
	if got := focused(t, m); got != moveTopKey {
		t.Fatalf("the picker opened on %q, want the parent the items share", got)
	}

	m, _ = press(t, m, "down", "enter")

	ts := loopsStamped()
	assertLog(t, path, []string{
		`{"ts":"` + ts + `","ev":"update","id":"9xz1","parent":"vq2n"}`,
		`{"ts":"` + ts + `","ev":"update","id":"p0rt","parent":"vq2n"}`,
	})
	if view := plain(m.View()); !strings.Contains(view, "filed 2 items  "+branchTitle) {
		t.Errorf("the footer carries no summary of the move:\n%s", view)
	}
	if len(m.loops.marked) != 0 {
		t.Errorf("the marks are %v after the move, want them cleared", m.loops.marked)
	}

	m, _ = press(t, m, "u")
	for _, id := range []string{"9xz1", "p0rt"} {
		if got := parentOf(t, m, id); got != nil {
			t.Errorf("%s sits under %q after the undo, want it back at the top level", id, *got)
		}
	}
}

// With several subjects lifted, none of them and nothing under any of them is on offer.
func TestMoveOffersNoMarkedItemNorItsDescendants(t *testing.T) {
	m, _ := filing(t)

	m.loops.marked = map[string]bool{"nktt": true, "p0rt": true}
	m, _ = press(t, m, "m")

	offered := destinations(m)
	for _, gone := range []string{"nktt", "4h2k", "p0rt"} {
		if slices.Contains(offered, gone) {
			t.Errorf("the picker offers %q as a destination: %v", gone, offered)
		}
	}
	if view := plain(m.View()); !strings.Contains(view, "moving  2 items  + 1 child") {
		t.Errorf("the header does not count what comes with the marked items:\n%s", view)
	}
}

// A marked item under another marked item travels with it and keeps its place beneath it.
func TestMoveLeavesAMarkedDescendantUnderItsMarkedAncestor(t *testing.T) {
	m, path := filing(t)

	m.loops.marked = map[string]bool{"nktt": true, "4h2k": true}
	m, _ = press(t, m, "m", "g", "enter")

	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"update","id":"nktt","parent":null}`})
	if got := parentOf(t, m, "4h2k"); got == nil || *got != "nktt" {
		t.Errorf("the descendant sits under %v, want it still under nktt", got)
	}
}

// A marked item already under the destination is skipped rather than written a move that moves
// nothing, and the one write that is left is receipted as the single move it is.
func TestMoveSkipsAMarkedItemAlreadyAtTheDestination(t *testing.T) {
	m, path := working(t)

	m.loops.marked = map[string]bool{"sga9": true, "p0rt": true}
	m, _ = press(t, m, "m")
	if got := focused(t, m); got != moveTopKey {
		t.Fatalf("the picker opened on %q, want the top level for items sharing no parent", got)
	}

	m, _ = press(t, m, "down", "enter")

	assertLog(t, path, []string{`{"ts":"` + loopsStamped() + `","ev":"update","id":"p0rt","parent":"vq2n"}`})
	if view := plain(m.View()); !strings.Contains(view, "filed p0rt  "+branchTitle) {
		t.Errorf("the footer does not receipt the one move made:\n%s", view)
	}
}

// Items sharing a parent open the picker on it, so an immediate enter moves nothing — and a drop
// that moves nothing leaves the marks standing.
func TestMoveOntoTheSharedParentWritesNothing(t *testing.T) {
	m, path := working(t)

	m, _ = press(t, m, "down", "x", "x", "m")
	if got := focused(t, m); got != "vq2n" {
		t.Fatalf("the picker opened on %q, want the parent the items share", got)
	}

	m, _ = press(t, m, "enter")

	if log := written(t, path); len(log) != 0 {
		t.Errorf("a drop on the current parent wrote %v", log)
	}
	if m.loops.moving.active() {
		t.Error("the picker is still open after the drop")
	}
	if len(m.loops.marked) != 2 {
		t.Errorf("the marks are %v, want both standing", m.loops.marked)
	}
}

// esc abandons the move and leaves the marks as they were.
func TestEscOutOfTheMoveKeepsTheMarks(t *testing.T) {
	m, _ := working(t)

	m, _ = press(t, m, "down", "x", "x", "m", "esc")

	if m.loops.moving.active() {
		t.Error("the picker is still open after esc")
	}
	if len(m.loops.marked) != 2 {
		t.Errorf("the marks are %v, want both standing", m.loops.marked)
	}
}

// The picker's header over several subjects, at the narrow width where it has least room.
func TestGoldenMoveMarkedAt80Columns(t *testing.T) {
	m := looped(t, 80, loopsNested())
	m.loops.marked = map[string]bool{"nktt": true, "p0rt": true}

	m, _ = press(t, m, "m")
	teatest.RequireEqualOutput(t, []byte(plain(m.View())))
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/tui -run 'Move'`
Expected: FAIL — the picker lifts the cursor row only.

- [ ] **Step 3: Implement `refile.go`**

Replace `refile` and its comment:

```go
// refile writes the move in the frame the key was pressed in, for one item or for several dropped
// together. A destination that is the parent an item already sits under is not a write for that item
// — the log would hold a move that moved nothing (§6) — so it is skipped, and a drop that moves
// nothing at all writes nothing.
func (m Model) refile(ids []string, parent *string) (Model, tea.Cmd) {
	label := m.parentLabel(parent)

	var writes []itemWrite
	for _, id := range ids {
		item, loaded := m.loadedItem(id)
		if !loaded || sameRef(parent, item.Parent) {
			continue
		}

		moved := item
		moved.Parent = parent
		writes = append(writes, itemWrite{
			event:   events.ParentEvent{Ev: "update", Id: id, Parent: parent},
			after:   moved,
			receipt: receipt{verb: verbFiled, subject: id, detail: label},
			inverse: undoRefile(item, m.parentLabel(item.Parent)),
		})
	}
	if len(writes) == 0 {
		return m, nil
	}

	return m.commit("parent", writes, receipt{verb: verbFiled, subject: plural(len(writes), "item"), detail: label})
}
```

Add after `sameRef`:

```go
// sharedParent is the parent every item of a list sits under, or nil when they sit under different
// ones — which is also what the top level is, and the same row either way in the picker.
func sharedParent(items []events.Item) *string {
	parent := items[0].Parent
	for _, item := range items[1:] {
		if !sameRef(item.Parent, parent) {
			return nil
		}
	}
	return parent
}
```

- [ ] **Step 4: Implement `move.go`**

In the file's opening comment, change "`m` lifts the item — and its descendants — out of the tree" to "`m` lifts the item — or every marked item — and its descendants out of the tree".

Replace `moveModel` and `active`:

```go
// moveModel is the picker's state: what is being moved, and the cursor and folds over what is left.
// subjects is empty when the mode is closed, which is the zero value. It never holds an item that
// sits beneath another of its own, since the one above already carries it.
type moveModel struct {
	subjects []string
	cursor   int
	expanded map[string]bool

	// top is the first line of the picker on screen once it holds more than its rows.
	top int
}

// active reports whether the picker is open, which is when it holds the keyboard.
func (mm moveModel) active() bool { return len(mm.subjects) > 0 }
```

Replace `startMove`:

```go
// startMove opens the picker on the marked items, or on the item under the cursor when nothing is
// marked. A row that holds no item is inert, as it is for every other write, and the footer says
// which row the key was pressed on (§4).
func (m Model) startMove(t Tree[events.Item]) (Model, tea.Cmd, bool) {
	m, items, ok := m.targets(t, "m moves an item under another")
	if !ok {
		return m, nil, true
	}

	// The subjects are set first, since the folds and the cursor are read off the tree the mode draws,
	// and that tree is the one the subjects have already been lifted out of.
	subjects := m.topmost(items)
	if len(subjects) == 0 {
		// A log whose parents loop leaves every marked item beneath another, and nothing topmost.
		subjects = items
	}
	parent := sharedParent(subjects)
	m.loops.moving = moveModel{subjects: idsOf(subjects)}
	m.loops.moving.expanded = m.moveFolds(parent)
	m.loops.moving.cursor = m.moveCursor(parent)
	return m, nil, true
}
```

In the comments of `moveFolds` and `moveCursor`, read "the item's current parent" as "the parent the subjects share"; add to `moveCursor`'s: `Subjects sitting under different parents open on the top-level row, as an item at the top level does.`

Replace `moveCandidates` and add `lifted`:

```go
// lifted is everything that leaves the tree while the picker is open: the subjects and whatever
// hangs beneath each.
func (m Model) lifted() []string {
	var lifted []string
	for _, id := range m.loops.moving.subjects {
		lifted = append(append(lifted, id), m.descendants(id)...)
	}
	return lifted
}

// moveCandidates are the items left once the subjects and everything under them have been lifted out.
func (m Model) moveCandidates() []events.Item {
	lifted := m.lifted()

	kept := make([]events.Item, 0, len(m.loops.items))
	for _, item := range m.loops.items {
		if !slices.Contains(lifted, item.Id) {
			kept = append(kept, item)
		}
	}
	return kept
}
```

In `commitMove`, replace the last three lines:

```go
	subjects := m.loops.moving.subjects
	m.loops.moving = moveModel{}
	return m.refile(subjects, parent)
```

and change its comment's opening to "commitMove drops what is being moved on the row the cursor is on."

Replace `moveHeader`:

```go
// moveHeader names what is being moved, how much comes with it, and the two keys that end the mode:
// one item by its id and title, several by their count. The title gives way first, since the id and
// the keys are the parts a narrow terminal cannot infer.
func (m Model) moveHeader(width int) string {
	subjects := m.loops.moving.subjects

	head := " moving"
	lead, title := "  "+plural(len(subjects), "item"), ""
	if len(subjects) == 1 {
		item, loaded := m.loadedItem(subjects[0])
		if !loaded {
			return ""
		}
		lead, title = "  "+item.Id+"  ", item.Title
	}

	carried := ""
	if n := len(m.lifted()) - len(subjects); n > 0 {
		carried = "  + " + kin(n)
	}

	fixed := lipgloss.Width(head+lead+carried) + lipgloss.Width(moveEnders) + 2
	rest := lead + truncate(title, max(width-fixed, 0)) + carried

	row := m.theme.Heading.Render(head) + m.theme.Row.Render(rest)
	return m.headerLine(row, lipgloss.Width(head+rest), moveEnders, width)
}
```

Then fix any remaining reference to `moving.subject` the compiler reports (`go build ./internal/tui`).

- [ ] **Step 5: Generate the new golden, check the old ones did not move, run the package**

Run: `go test ./internal/tui -run TestGoldenMoveMarked -update`, and confirm the new file's header line reads ` moving  2 items  + 1 child` with `esc cancels · enter drops` against the right edge.

Run: `go test ./internal/tui && git status --short internal/tui/testdata`
Expected: PASS; only `TestGoldenMoveMarkedAt80Columns.golden` is new, and `TestGoldenMoveAt80Columns.golden` / `…140…` are unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): m moves every marked item to one destination"
```

---

### Task 9: Document the keys

**Files:**
- Modify: `README.md` (the tab table near line 161 and the paragraph on `m` under it), `internal/tui/app.go` (`tabBindings[tabLoops]` effects)
- Test: `internal/tui/app_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/app_test.go`:

```go
// The key table names the mark keys and says which writes take the marked set.
func TestTheKeyTableDocumentsMarks(t *testing.T) {
	m, _ := press(t, chrome(t, 140), "?")

	view := plain(m.View())
	for _, want := range []string{"mark the row", "clear the marks", "or every marked item"} {
		if !strings.Contains(view, want) {
			t.Errorf("the key table does not say %q:\n%s", want, view)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui -run TestTheKeyTableDocumentsMarks`
Expected: FAIL on `or every marked item`.

- [ ] **Step 3: Implement**

`internal/tui/app.go`, `tabBindings[tabLoops]` — set these effects:

```go
		{[]string{"d"}, "d", "done — the row, or every marked item"},
		{[]string{"x"}, "x", "mark the row, for a write against several"},
		{[]string{"X"}, "X", "clear the marks"},
		{[]string{"w"}, "w", "waiting — the row, or every marked item"},
```

```go
		{[]string{"t"}, "t", "edit the tags, or add tags to every marked item — commit an empty answer to take one item's off"},
		{[]string{"m"}, "m", "move the row, or every marked item, under another — enter drops, esc cancels"},
```

`README.md` — in the Loops row of the tab table, replace `` `x` done, `` with `` `x` mark, `X` clear the marks, `d` done, `` and after the paragraph that begins `` `m` moves an item by navigation `` add:

```markdown
`x` marks a row and steps down, so a run of rows is marked by repeating it; `X` clears the marks.
While any row is marked, `m`, `d`, `w` and `t` act on every marked item instead of the row under the
cursor: `m` drops them all on one destination, `d` closes them, `w` asks once who they wait on, and
`t` adds the tags typed to each. Marks follow the item rather than the row, so they hold while the
list is filtered — filter, mark, clear the filter, mark more — and the header counts them. One `u`
reverses the whole write.
```

- [ ] **Step 4: Run everything**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add README.md internal/tui
git commit -m "docs: the README and the key table cover marks"
```
