import { test } from 'node:test';
import assert from 'node:assert/strict';

import { createPickState, plan, reduce, resize } from '../src/tui/pick.ts';
import type { Item, Key, PickRow, PickState, Signal } from '../src/types.ts';

function signal(key: string, overrides: Partial<Signal> = {}): Signal {
  return {
    key,
    kind: 'dirty',
    title: `Signal ${key}`,
    subject: `subject for ${key}`,
    detail: `detail for ${key}`,
    branch: 'main',
    project: 'C:\\dev\\waid',
    age: '2d',
    ...overrides,
  };
}

function item(id: string, overrides: Partial<Item> = {}): Item {
  return {
    id,
    title: `Item ${id}`,
    status: 'open',
    waitingOn: null,
    project: 'C:\\dev\\waid',
    session: null,
    tags: [],
    notes: [],
    created: '2026-08-12T09:00:00.000Z',
    updated: '2026-08-13T09:00:00.000Z',
    ...overrides,
  };
}

const ALPHA = item('k3f9');
const BETA = item('m7qz');
const REVIEW = signal('review:acme/web#123', { kind: 'review', title: 'review acme/web#123' });
const DIRTY = signal('dirty:C:\\dev\\waid');

/**
 * A `loops` screen: headings at both ends, two item rows and two signal rows, with headings in
 * between. Selectable rows are 3, 4, 8 and 9.
 */
function screen(): PickRow[] {
  return [
    { kind: 'heading', text: 'OPEN LOOPS' },
    { kind: 'heading', text: '' },
    { kind: 'heading', text: '  C:\\dev\\waid' },
    { kind: 'item', id: ALPHA.id, text: `  ${ALPHA.id}  ${ALPHA.title}`, item: ALPHA },
    { kind: 'item', id: BETA.id, text: `  ${BETA.id}  ${BETA.title}`, item: BETA },
    { kind: 'heading', text: '' },
    { kind: 'heading', text: 'DETECTED  (2 shown, 0 dismissed)' },
    { kind: 'heading', text: '' },
    { kind: 'signal', id: REVIEW.key, text: `  ${REVIEW.key}  ${REVIEW.detail}`, signal: REVIEW },
    { kind: 'signal', id: DIRTY.key, text: `  ${DIRTY.key}  ${DIRTY.detail}`, signal: DIRTY },
    { kind: 'heading', text: '' },
    { kind: 'heading', text: '  waid promote <key> to track' },
  ];
}

const ITEM_ROW = 3;
const SECOND_ITEM_ROW = 4;
const SIGNAL_ROW = 8;
const LAST_ROW = 9;

function state(height = 40): PickState {
  return createPickState(screen(), height);
}

const key = (name: 'up' | 'down' | 'enter' | 'esc' | 'backspace' | 'ctrl-c'): Key => ({ name });
const char = (value: string): Key => ({ name: 'char', value });

/** Applies keys in order, the way the picker loop does. */
function press(start: PickState, ...keys: Key[]): PickState {
  return keys.reduce((current, next) => reduce(current, next), start);
}

/** `Plan` carries whole rows, so ids come back out through the union. */
function rowId(row: PickRow): string | null {
  return row.kind === 'heading' ? null : row.id;
}

/** Types a string one character at a time. */
function type(start: PickState, text: string): PickState {
  return press(start, ...[...text].map(char));
}

test('the cursor starts on the first selectable row', () => {
  const initial = state();
  assert.equal(initial.cursor, ITEM_ROW);
  assert.deepEqual(initial.marks, {});
  assert.equal(initial.editing, null);
  assert.equal(initial.hint, null);
  assert.equal(initial.legend, false);
});

test('a screen with nothing selectable parks the cursor and ignores movement', () => {
  const empty = createPickState([{ kind: 'heading', text: 'no open loops' }], 10);
  assert.equal(empty.cursor, 0);
  assert.equal(press(empty, char('j'), char('G')).cursor, 0);
  assert.deepEqual(plan(empty), []);
});

test('j and k skip headings and clamp at both ends', () => {
  const start = state();

  assert.equal(press(start, char('j')).cursor, SECOND_ITEM_ROW);
  assert.equal(press(start, char('j'), char('j')).cursor, SIGNAL_ROW);
  assert.equal(press(start, char('j'), char('j'), char('j')).cursor, LAST_ROW);
  assert.equal(press(start, char('j'), char('j'), char('j'), char('j')).cursor, LAST_ROW);

  assert.equal(press(start, char('k')).cursor, ITEM_ROW);
  assert.equal(press(start, char('j'), char('j'), char('k')).cursor, SECOND_ITEM_ROW);
});

test('the arrow keys move the cursor like j and k', () => {
  const start = state();
  assert.equal(press(start, key('down')).cursor, SECOND_ITEM_ROW);
  assert.equal(press(start, key('down'), key('down'), key('up')).cursor, SECOND_ITEM_ROW);
  assert.equal(press(start, key('up')).cursor, ITEM_ROW);
});

test('g and G jump to the first and last selectable rows', () => {
  const start = state();
  assert.equal(press(start, char('G')).cursor, LAST_ROW);
  assert.equal(press(start, char('G'), char('g')).cursor, ITEM_ROW);
});

test('p and d mark signal rows', () => {
  const marked = press(state(), char('G'), char('p'));
  assert.deepEqual(marked.marks, { [DIRTY.key]: { action: 'promote', title: DIRTY.title } });
  assert.equal(marked.hint, null);

  const dismissed = press(state(), char('G'), char('d'));
  assert.deepEqual(dismissed.marks, { [DIRTY.key]: { action: 'dismiss' } });
});

test('x marks an item done and w marks it waiting', () => {
  const done = press(state(), char('x'));
  assert.deepEqual(done.marks, { [ALPHA.id]: { action: 'done' } });

  const waiting = press(state(), char('w'));
  assert.deepEqual(waiting.marks, { [ALPHA.id]: { action: 'waiting', waitingOn: '' } });
});

test('p and d are inert on item rows and hint why', () => {
  const inert = press(state(), char('p'));
  assert.deepEqual(inert.marks, {});
  assert.equal(inert.editing, null);
  assert.match(inert.hint ?? '', /^p only applies to detected signals/);

  assert.match(press(state(), char('d')).hint ?? '', /^d only applies to detected signals/);
});

test('x and w are inert on signal rows and hint why', () => {
  const inert = press(state(), char('G'), char('x'));
  assert.deepEqual(inert.marks, {});
  assert.match(inert.hint ?? '', /^x only applies to declared items/);

  const waiting = press(state(), char('G'), char('w'));
  assert.deepEqual(waiting.marks, {});
  assert.equal(waiting.editing, null);
  assert.match(waiting.hint ?? '', /^w only applies to declared items/);
});

test('a hint clears on the next key', () => {
  const cleared = press(state(), char('p'), char('j'));
  assert.equal(cleared.hint, null);
});

test('the same action key twice unmarks the row', () => {
  const toggled = press(state(), char('x'), char('x'));
  assert.deepEqual(toggled.marks, {});

  const signalToggled = press(state(), char('G'), char('p'), char('p'));
  assert.deepEqual(signalToggled.marks, {});
});

test('a different action key switches the mark', () => {
  const switched = press(state(), char('G'), char('p'), char('d'));
  assert.deepEqual(switched.marks, { [DIRTY.key]: { action: 'dismiss' } });
});

test('u unmarks the row under the cursor and leaves the others alone', () => {
  const marked = press(state(), char('x'), char('G'), char('p'));
  const unmarked = press(marked, char('u'));
  assert.deepEqual(unmarked.marks, { [ALPHA.id]: { action: 'done' } });
  assert.deepEqual(press(unmarked, char('u')).marks, { [ALPHA.id]: { action: 'done' } });
});

test('w opens the editor for who the item is waiting on', () => {
  const waiting = press(state(), char('w'));
  assert.deepEqual(waiting.editing, { rowId: ALPHA.id, value: '', original: '' });

  const typed = type(waiting, 'ops');
  assert.equal(typed.editing?.value, 'ops');
  const committed = press(typed, key('enter'));
  assert.equal(committed.editing, null);
  assert.deepEqual(committed.marks, { [ALPHA.id]: { action: 'waiting', waitingOn: 'ops' } });
});

test('e edits the text field of the mark on the row', () => {
  const editing = press(state(), char('G'), char('p'), char('e'));
  assert.deepEqual(editing.editing, {
    rowId: DIRTY.key,
    value: DIRTY.title,
    original: DIRTY.title,
  });

  const retitled = press(type(press(editing, key('backspace')), 'X'), key('enter'));
  assert.deepEqual(retitled.marks, {
    [DIRTY.key]: { action: 'promote', title: `${DIRTY.title.slice(0, -1)}X` },
  });
});

test('e on a mark with no text field is inert and hints why', () => {
  const noMark = press(state(), char('e'));
  assert.equal(noMark.editing, null);
  assert.match(noMark.hint ?? '', /^e edits/);

  const done = press(state(), char('x'), char('e'));
  assert.equal(done.editing, null);
  assert.match(done.hint ?? '', /^e edits/);
});

test('backspace stops at an empty field', () => {
  const emptied = press(state(), char('w'), key('backspace'), key('backspace'));
  assert.equal(emptied.editing?.value, '');
});

test('while editing, keys go to the line editor rather than the picker', () => {
  const editing = type(press(state(), char('w')), 'pxdujgG');
  assert.equal(editing.editing?.value, 'pxdujgG');
  assert.equal(editing.cursor, ITEM_ROW);
  assert.deepEqual(editing.marks, { [ALPHA.id]: { action: 'waiting', waitingOn: '' } });

  const moved = press(editing, key('up'), key('down'));
  assert.equal(moved.cursor, ITEM_ROW);
  assert.equal(moved.editing?.value, 'pxdujgG');
});

test('esc reverts the field and leaves the mark as it was', () => {
  const reverted = press(state(), char('G'), char('p'), char('e'), char('!'), key('esc'));
  assert.equal(reverted.editing, null);
  assert.deepEqual(reverted.marks, { [DIRTY.key]: { action: 'promote', title: DIRTY.title } });
});

test('editing a waiting mark twice keeps the committed value as the revert point', () => {
  const committed = press(type(press(state(), char('w')), 'ops'), key('enter'));
  const reverted = press(committed, char('e'), char('!'), key('esc'));
  assert.deepEqual(reverted.marks, { [ALPHA.id]: { action: 'waiting', waitingOn: 'ops' } });
});

test('? toggles the full key table', () => {
  assert.equal(press(state(), char('?')).legend, true);
  assert.equal(press(state(), char('?'), char('?')).legend, false);
});

test('the viewport follows the cursor', () => {
  const start = state(4);
  assert.deepEqual(start.viewport, { top: 0, height: 4 });

  const down = press(start, char('j'), char('j'));
  assert.equal(down.cursor, SIGNAL_ROW);
  assert.deepEqual(down.viewport, { top: 5, height: 4 });

  const bottom = press(start, char('G'));
  assert.deepEqual(bottom.viewport, { top: 6, height: 4 });

  const top = press(bottom, char('g'));
  assert.deepEqual(top.viewport, { top: ITEM_ROW, height: 4 });
});

test('a viewport taller than the screen stays at the top', () => {
  assert.deepEqual(press(state(40), char('G')).viewport, { top: 0, height: 40 });
});

test('resize recomputes the viewport around the cursor', () => {
  const bottom = press(state(40), char('G'));
  const shrunk = resize(bottom, 3);
  assert.deepEqual(shrunk.viewport, { top: 7, height: 3 });
  assert.equal(shrunk.cursor, LAST_ROW);
});

test('the plan lists every mark in row order', () => {
  const marked = press(state(), char('x'), char('G'), char('p'), char('k'), char('d'));
  assert.deepEqual(
    plan(marked).map((entry) => [rowId(entry.row), entry.mark]),
    [
      [ALPHA.id, { action: 'done' }],
      [REVIEW.key, { action: 'dismiss' }],
      [DIRTY.key, { action: 'promote', title: DIRTY.title }],
    ],
  );
});

test('the plan carries the row a mark was made against', () => {
  const marked = press(state(), char('x'));
  const [entry] = plan(marked);
  assert.equal(entry?.row.kind, 'item');
  assert.deepEqual(entry?.row.kind === 'item' ? entry.row.item : null, ALPHA);
});
