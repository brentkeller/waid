import { test } from 'node:test';
import assert from 'node:assert/strict';

import { pick } from '../src/tui/run.ts';
import type { Input, Terminal } from '../src/tui/term.ts';
import type { Item, Key, PickRow, Signal } from '../src/types.ts';

const SIGNAL: Signal = {
  key: 'dirty:C:\\dev\\waid',
  kind: 'dirty',
  title: '6 uncommitted files in waid',
  detail: '6 files · 20m',
  project: 'C:\\dev\\waid',
  age: '20m',
};

const ITEM: Item = {
  id: 'k3f9',
  title: 'ship the picker',
  status: 'open',
  waitingOn: null,
  project: 'C:\\dev\\waid',
  session: null,
  tags: [],
  notes: [],
  created: '2026-08-12T09:00:00.000Z',
  updated: '2026-08-13T09:00:00.000Z',
};

function screen(): PickRow[] {
  return [
    { kind: 'heading', text: 'OPEN LOOPS' },
    { kind: 'item', id: ITEM.id, text: `  ${ITEM.id}  ${ITEM.title}`, item: ITEM },
    { kind: 'heading', text: 'DETECTED  (1 shown, 0 dismissed)' },
    { kind: 'signal', id: SIGNAL.key, text: `  ${SIGNAL.key}  ${SIGNAL.detail}`, signal: SIGNAL },
  ];
}

function key(value: string): Key {
  if (value === 'enter' || value === 'esc' || value === 'backspace') return { name: value };
  return { name: 'char', value };
}

/** Scripted inputs, one per `read`, with every painted screen recorded. */
function fakeTerminal(script: Input[], rows = 20): Terminal & { painted: string[][] } {
  const inputs = [...script];
  const painted: string[][] = [];
  return {
    painted,
    rows: () => rows,
    read: async () => inputs.shift() ?? { kind: 'end' },
    write: (lines) => {
      painted.push(lines);
    },
    restore: () => {},
  };
}

function keys(...values: string[]): Input[] {
  return values.map((value) => ({ kind: 'key', key: key(value) }) as Input);
}

test('enter returns the staged marks in row order', async () => {
  const term = fakeTerminal(keys('x', 'j', 'p', 'enter'));

  const plan = await pick(screen(), term);

  assert.deepEqual(plan, [
    { row: screen()[1], mark: { action: 'done' } },
    { row: screen()[3], mark: { action: 'promote', title: SIGNAL.title } },
  ]);
});

test('q and esc cancel, discarding the marks', async () => {
  assert.equal(await pick(screen(), fakeTerminal(keys('x', 'q'))), null);
  assert.equal(await pick(screen(), fakeTerminal(keys('x', 'esc'))), null);
});

test('stdin closing under the picker cancels', async () => {
  assert.equal(await pick(screen(), fakeTerminal([{ kind: 'end' }])), null);
});

test('enter and esc reach the line editor rather than the loop while it is open', async () => {
  // `w` opens the editor for who, `esc` reverts the field, and a second `esc` then cancels.
  assert.equal(await pick(screen(), fakeTerminal(keys('w', 'a', 'esc', 'esc'))), null);

  // The same editor closed with `enter` commits the field; the next `enter` applies the batch.
  const term = fakeTerminal(keys('w', 'b', 'o', 'b', 'enter', 'enter'));
  assert.deepEqual(await pick(screen(), term), [
    { row: screen()[1], mark: { action: 'waiting', waitingOn: 'bob' } },
  ]);
});

test('paints once before the first key and once after each key that is not the last', async () => {
  const term = fakeTerminal(keys('j', 'q'));

  await pick(screen(), term);

  // The initial screen and the one `j` moved the cursor on; `q` exits without a final repaint.
  assert.equal(term.painted.length, 2);
});

test('sizes the viewport to the terminal, leaving the footer its room', async () => {
  const term = fakeTerminal(keys('q'), 6);

  await pick(screen(), term);

  // 6 rows: 3 of list, a blank, the status line and the short legend.
  assert.equal(term.painted[0]?.length, 6);
});

test('a resize repaints at the new height without touching the marks', async () => {
  let rows = 20;
  const term = fakeTerminal(keys('x'), rows);
  const resizing: Terminal & { painted: string[][] } = {
    ...term,
    rows: () => rows,
    read: async () => {
      const next = await term.read();
      if (next.kind === 'end') return next;
      rows = 6;
      return next;
    },
  };

  await pick(screen(), resizing);

  assert.equal(resizing.painted.at(-1)?.length, 6);
});
