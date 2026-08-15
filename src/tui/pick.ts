import type { Key, Mark, PickRow, PickState, Plan } from '../types.ts';

/** The keys that stage a mark, and the action each one stages. */
const ACTIONS: Record<string, Mark['action']> = {
  p: 'promote',
  d: 'dismiss',
  x: 'done',
  w: 'waiting',
};

/** Actions that only make sense against a detected signal. */
const SIGNAL_ACTIONS = new Set<Mark['action']>(['promote', 'dismiss']);

const HINTS = {
  signalOnly: (pressed: string) => `${pressed} only applies to detected signals`,
  itemOnly: (pressed: string) => `${pressed} only applies to declared items`,
  nothingToEdit: 'e edits a promote title or who an item is waiting on',
};

/** A fresh screen, with the cursor parked on the first row a mark can be made against. */
export function createPickState(rows: PickRow[], height: number): PickState {
  return follow({
    rows,
    cursor: seek(rows, -1, 1) ?? 0,
    marks: {},
    editing: null,
    viewport: { top: 0, height },
    hint: null,
    legend: false,
  });
}

/** A new height from `SIGWINCH`, with the viewport pulled back around the cursor. */
export function resize(state: PickState, height: number): PickState {
  return follow({ ...state, viewport: { ...state.viewport, height } });
}

/** Every mark the user staged, in row order — the batch `applyPlan` walks. */
export function plan(state: PickState): Plan {
  const entries: Plan = [];
  for (const row of state.rows) {
    if (row.kind === 'heading') continue;
    const mark = state.marks[row.id];
    if (mark !== undefined) entries.push({ row, mark });
  }
  return entries;
}

/**
 * The whole interaction. Takes no config, no clock and no I/O, so every key the picker responds to
 * is exercised without a terminal.
 */
export function reduce(state: PickState, key: Key): PickState {
  // While the editor is open every key belongs to it, so it is impossible to apply a batch by
  // trying to finish a title.
  if (state.editing !== null) return edit(state, key);

  const cleared = state.hint === null ? state : { ...state, hint: null };

  if (key.name === 'up') return move(cleared, -1);
  if (key.name === 'down') return move(cleared, 1);
  if (key.name !== 'char') return cleared;

  switch (key.value) {
    case 'j':
      return move(cleared, 1);
    case 'k':
      return move(cleared, -1);
    case 'g':
      return jump(cleared, 1);
    case 'G':
      return jump(cleared, -1);
    case '?':
      return { ...cleared, legend: !cleared.legend };
    case 'u':
      return unmark(cleared);
    case 'e':
      return openEditor(cleared);
    default: {
      const action = ACTIONS[key.value];
      return action === undefined ? cleared : mark(cleared, key.value, action);
    }
  }
}

/** Stages, switches or — when the same key is pressed twice — clears the mark under the cursor. */
function mark(state: PickState, pressed: string, action: Mark['action']): PickState {
  const row = current(state);
  if (row === null) return state;

  if (row.kind === 'signal' && !SIGNAL_ACTIONS.has(action)) {
    return { ...state, hint: HINTS.itemOnly(pressed) };
  }
  if (row.kind === 'item' && SIGNAL_ACTIONS.has(action)) {
    return { ...state, hint: HINTS.signalOnly(pressed) };
  }
  if (state.marks[row.id]?.action === action) return unmark(state);

  const staged = seed(row, action);
  const marked = { ...state, marks: { ...state.marks, [row.id]: staged } };

  // `waiting` is the one action that is useless without its field, so it opens the editor itself.
  return staged.action === 'waiting' ? editing(marked, row.id, staged.waitingOn) : marked;
}

/** The mark a freshly pressed action key stages, seeded from whatever the row already knows. */
function seed(row: Exclude<PickRow, { kind: 'heading' }>, action: Mark['action']): Mark {
  switch (action) {
    case 'promote':
      return { action, title: row.kind === 'signal' ? row.signal.title : row.text };
    case 'waiting':
      return { action, waitingOn: (row.kind === 'item' ? row.item.waitingOn : null) ?? '' };
    case 'dismiss':
      return { action };
    case 'done':
      return { action };
  }
}

function unmark(state: PickState): PickState {
  const row = current(state);
  if (row === null || state.marks[row.id] === undefined) return state;

  const marks = { ...state.marks };
  delete marks[row.id];
  return { ...state, marks };
}

/** `e` edits *the mark's* text field, whichever of the two it is. */
function openEditor(state: PickState): PickState {
  const row = current(state);
  if (row === null) return state;

  const field = textField(state.marks[row.id]);
  if (field === null) return { ...state, hint: HINTS.nothingToEdit };
  return editing(state, row.id, field);
}

function editing(state: PickState, rowId: string, value: string): PickState {
  return { ...state, editing: { rowId, value, original: value } };
}

/** The line editor: `Enter` commits the field, `Esc` restores it, and nothing reaches the plan. */
function edit(state: PickState, key: Key): PickState {
  const open = state.editing;
  if (open === null) return state;

  switch (key.name) {
    case 'enter':
      return { ...commit(state, open.rowId, open.value), editing: null };
    case 'esc':
      return { ...commit(state, open.rowId, open.original), editing: null };
    case 'backspace':
      return { ...state, editing: { ...open, value: open.value.slice(0, -1) } };
    case 'char':
      return { ...state, editing: { ...open, value: open.value + key.value } };
    default:
      return state;
  }
}

function commit(state: PickState, rowId: string, value: string): PickState {
  const mark = state.marks[rowId];
  if (mark === undefined) return state;

  const updated: Mark | null =
    mark.action === 'promote'
      ? { action: 'promote', title: value }
      : mark.action === 'waiting'
        ? { action: 'waiting', waitingOn: value }
        : null;
  if (updated === null) return state;

  return { ...state, marks: { ...state.marks, [rowId]: updated } };
}

/** The editable text a mark carries, or null when it carries none. */
function textField(mark: Mark | undefined): string | null {
  if (mark === undefined) return null;
  if (mark.action === 'promote') return mark.title;
  if (mark.action === 'waiting') return mark.waitingOn;
  return null;
}

function current(state: PickState): Exclude<PickRow, { kind: 'heading' }> | null {
  const row = state.rows[state.cursor];
  return row === undefined || row.kind === 'heading' ? null : row;
}

function move(state: PickState, step: 1 | -1): PickState {
  const next = seek(state.rows, state.cursor, step);
  return next === null ? state : follow({ ...state, cursor: next });
}

function jump(state: PickState, step: 1 | -1): PickState {
  const from = step === 1 ? -1 : state.rows.length;
  const next = seek(state.rows, from, step);
  return next === null ? state : follow({ ...state, cursor: next });
}

/** The nearest selectable row past `from` in `step`'s direction, or null when the list ends. */
function seek(rows: readonly PickRow[], from: number, step: 1 | -1): number | null {
  for (let i = from + step; i >= 0 && i < rows.length; i += step) {
    if (rows[i]?.kind !== 'heading') return i;
  }
  return null;
}

/** Slides the viewport the least it can to keep the cursor on screen. */
function follow(state: PickState): PickState {
  const { top, height } = state.viewport;
  const maxTop = Math.max(0, state.rows.length - height);

  let next = Math.min(top, maxTop);
  if (state.cursor < next) next = state.cursor;
  else if (state.cursor >= next + height) next = state.cursor - height + 1;
  next = Math.max(0, Math.min(next, maxTop));

  return next === top ? state : { ...state, viewport: { ...state.viewport, top: next } };
}
