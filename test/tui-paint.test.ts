import { test } from 'node:test';
import assert from 'node:assert/strict';

import { createPickState, reduce } from '../src/tui/pick.ts';
import { paint } from '../src/tui/paint.ts';
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
const REVIEW = signal('review:acme/web#123', { kind: 'review', title: 'review acme/web#123' });
const DIRTY = signal('dirty:C:\\dev\\waid');

/** A `loops` screen: headings at both ends, one item row and two signal rows. Selectable: 3, 6, 7. */
function screen(): PickRow[] {
  return [
    { kind: 'heading', text: 'OPEN LOOPS' },
    { kind: 'heading', text: '' },
    { kind: 'heading', text: '  C:\\dev\\waid' },
    { kind: 'item', id: ALPHA.id, text: `  ${ALPHA.id}  ${ALPHA.title}`, item: ALPHA },
    { kind: 'heading', text: '' },
    { kind: 'heading', text: 'DETECTED  (2 shown, 0 dismissed)' },
    { kind: 'signal', id: REVIEW.key, text: `  ${REVIEW.key}  ${REVIEW.detail}`, signal: REVIEW },
    { kind: 'signal', id: DIRTY.key, text: `  ${DIRTY.key}  ${DIRTY.detail}`, signal: DIRTY },
  ];
}

function state(height = 20): PickState {
  return createPickState(screen(), height);
}

function press(start: PickState, ...keys: string[]): PickState {
  return keys.reduce<PickState>((current, value) => reduce(current, key(value)), start);
}

const NAMED = new Set(['up', 'down', 'enter', 'esc', 'backspace', 'ctrl-c']);

/** A key by its picker name, or a printable character when the name is not one of the specials. */
function key(value: string): Key {
  return NAMED.has(value) ? { name: value as 'up' } : { name: 'char', value };
}

const ANSI = /\x1b\[[0-9;]*m/g;

function strip(lines: string[]): string[] {
  return lines.map((line) => line.replace(ANSI, ''));
}

/** The screen without colour, which is what every layout assertion is made against. */
function plain(current: PickState): string[] {
  return paint(current, false);
}

test('the gutter carries the cursor and the mark letter', () => {
  const marked = press(state(), 'x', 'down', 'p');
  const lines = plain(marked);

  assert.equal(lines[3], '  X  k3f9  Item k3f9');
  assert.equal(lines[6], '> P  review:acme/web#123  detail for review:acme/web#123');
  assert.equal(lines[7], `     dirty:C:\\dev\\waid  detail for dirty:C:\\dev\\waid`);
});

test('headings paint verbatim, so the screen reproduces the printed layout', () => {
  const lines = plain(state());

  assert.equal(lines[0], 'OPEN LOOPS');
  assert.equal(lines[1], '');
  assert.equal(lines[2], '  C:\\dev\\waid');
  assert.equal(lines[5], 'DETECTED  (2 shown, 0 dismissed)');
});

test('the footer counts the marks and shows the short legend', () => {
  const empty = plain(state());
  assert.ok(empty.includes('  nothing marked'), 'an unmarked screen says so');
  assert.ok(
    empty.some((line) => line.includes('enter apply') && line.includes('? keys')),
    'the short legend names apply and the key table',
  );

  const marked = press(state(), 'x', 'down', 'p', 'down', 'p');
  const lines = plain(marked);
  assert.ok(lines.includes('  2 to promote, 1 to mark done'), lines.join('\n'));
});

test('a hint replaces the count line until the next key', () => {
  const hinted = press(state(), 'p');
  assert.ok(
    plain(hinted).includes('  p only applies to detected signals'),
    'the inert key explains itself in the footer',
  );

  const cleared = press(hinted, 'x');
  assert.ok(plain(cleared).includes('  1 to mark done'), 'the next key clears the hint');
});

test('? swaps the short legend for the full key table', () => {
  const lines = plain(press(state(), '?'));
  const footer = lines.join('\n');

  assert.ok(!lines.some((line) => line.includes('enter apply · q cancel')), 'the short legend goes');
  assert.match(footer, /ctrl-c/);
  assert.match(footer, /apply every mark and exit/);
  assert.match(footer, /promote a detected signal/);

  const back = plain(press(state(), '?', '?'));
  assert.deepEqual(back, plain(state()), '? toggles');
});

test('edit mode renders the field being edited', () => {
  const editing = press(state(), 'down', 'p', 'e');
  const lines = plain(editing);

  assert.ok(lines.includes('  title: review acme/web#123▌'), lines.join('\n'));
  assert.ok(lines.some((line) => line.includes('esc cancel')), 'the editor names its own keys');

  const typed = plain(press(editing, 'backspace', '4'));
  assert.ok(typed.includes('  title: review acme/web#124▌'), typed.join('\n'));

  const waiting = plain(press(state(), 'w', 'B', 'o'));
  assert.ok(waiting.includes('  waiting on: Bo▌'), waiting.join('\n'));
});

test('the painted rows are exactly the viewport slice, padded to its height', () => {
  const scrolled = press(state(4), 'G');
  const lines = plain(scrolled);

  assert.deepEqual(scrolled.viewport, { top: 4, height: 4 });
  assert.deepEqual(lines.slice(0, 4), [
    '',
    'DETECTED  (2 shown, 0 dismissed)',
    '     review:acme/web#123  detail for review:acme/web#123',
    `>    dirty:C:\\dev\\waid  detail for dirty:C:\\dev\\waid`,
  ]);

  const short = plain(createPickState(screen().slice(0, 4), 6));
  assert.deepEqual(short.slice(0, 6).slice(4), ['', ''], 'a short screen pads to the height');
});

test('the screen reads correctly with every escape stripped', () => {
  const marked = press(state(), 'x', 'down', 'p');

  assert.deepEqual(strip(paint(marked, true)), paint(marked, false));
  assert.notDeepEqual(paint(marked, true), paint(marked, false), 'colour is actually emitted');
  assert.deepEqual(strip(paint(press(marked, '?'), true)), paint(press(marked, '?'), false));
});
