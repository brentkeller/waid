import { test } from 'node:test';
import assert from 'node:assert/strict';

import { loadConfig } from '../src/config.ts';
import { loops } from '../src/commands/loops.ts';
import { scan } from '../src/commands/scan.ts';
import type { ScanResult } from '../src/commands/scan.ts';
import type { Ctx, Item, LoopsResult, PickRow, Signal } from '../src/types.ts';

const NOW = new Date('2026-08-14T12:00:00.000Z');

/** A context carrying only what `rows` reads: the clock. Config is derived, never read from disk. */
function ctx(): Ctx {
  return {
    cfg: loadConfig('C:\\nowhere\\waid'),
    flags: {},
    args: [],
    cwd: 'C:\\dev\\waid',
    now: NOW,
    notes: [],
  };
}

function signal(key: string, overrides: Partial<Signal> = {}): Signal {
  return {
    key,
    kind: 'dirty',
    title: `Signal ${key}`,
    detail: `detail for ${key}`,
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

function scanResult(overrides: Partial<ScanResult> = {}): ScanResult {
  return { signals: [], dismissedCount: 0, notes: [], ...overrides };
}

function loopsResult(overrides: Partial<LoopsResult> = {}): LoopsResult {
  return { groups: [], detected: [], dismissedCount: 0, notes: [], ...overrides };
}

function texts(rows: PickRow[]): string[] {
  return rows.map((row) => row.text);
}

function kinds(rows: PickRow[]): string[] {
  return rows.map((row) => row.kind);
}

test('scan rows reproduce the printed screen line for line', () => {
  const data = scanResult({
    signals: [signal('review:acme/web#123'), signal(`dirty:C:\\dev\\waid`)],
    dismissedCount: 1,
    notes: ['GitHub signals unavailable'],
  });

  const rows = scan.rows?.(data, ctx());
  assert.ok(rows, 'scan must expose rows()');
  assert.equal(texts(rows).join('\n'), scan.render(data, ctx()));
});

test('scan rows carry the signal on the signal lines and headings everywhere else', () => {
  const first = signal('review:acme/web#123');
  const second = signal(`dirty:C:\\dev\\waid`);
  const rows = scan.rows?.(scanResult({ signals: [first, second], dismissedCount: 1 }), ctx()) ?? [];

  assert.deepEqual(kinds(rows), ['heading', 'heading', 'signal', 'signal', 'heading', 'heading']);
  assert.equal(rows[0]?.text, 'DETECTED  (2 shown, 1 dismissed)');
  assert.equal(rows[1]?.text, '');
  assert.equal(rows[4]?.text, '');
  assert.match(rows[5]?.text ?? '', /waid promote <key> to track/);

  const signals = rows.filter((row) => row.kind === 'signal');
  assert.deepEqual(
    signals.map((row) => row.id),
    [first.key, second.key],
  );
  assert.deepEqual(
    signals.map((row) => row.signal),
    [first, second],
  );
  for (const row of signals) assert.ok(row.text.includes(row.id), 'a signal row must show its key');
});

test('scan rows offer nothing selectable when nothing was detected', () => {
  const rows = scan.rows?.(scanResult(), ctx()) ?? [];
  assert.deepEqual(kinds(rows), ['heading']);
  assert.equal(rows[0]?.text, 'nothing detected');
});

test('loops rows reproduce the printed screen line for line', () => {
  const data = loopsResult({
    groups: [
      { project: 'C:\\dev\\waid', items: [item('k3f9'), item('m7qz')] },
      { project: null, items: [item('p2xd', { project: null })] },
    ],
    detected: [signal('review:acme/web#123')],
    dismissedCount: 2,
    notes: ['detection failed (git exploded); showing declared items only'],
  });

  const rows = loops.rows?.(data, ctx());
  assert.ok(rows, 'loops must expose rows()');
  assert.equal(texts(rows).join('\n'), loops.render(data, ctx()));
});

test('loops rows keep headings, items and signals distinct with unique ids', () => {
  const alpha = item('k3f9');
  const beta = item('m7qz');
  const detected = signal('review:acme/web#123');
  const data = loopsResult({
    groups: [
      { project: 'C:\\dev\\waid', items: [alpha] },
      { project: null, items: [beta] },
    ],
    detected: [detected],
    dismissedCount: 0,
  });

  const rows = loops.rows?.(data, ctx()) ?? [];

  assert.equal(rows[0]?.text, 'OPEN LOOPS');
  assert.equal(rows[0]?.kind, 'heading');
  assert.equal(rows[1]?.text, '');
  assert.equal(rows[2]?.text, '  C:\\dev\\waid');
  assert.equal(rows[2]?.kind, 'heading');
  assert.equal(rows[5]?.text, '  (no project)');

  const items = rows.filter((row) => row.kind === 'item');
  assert.deepEqual(
    items.map((row) => row.id),
    ['k3f9', 'm7qz'],
  );
  assert.deepEqual(
    items.map((row) => row.item),
    [alpha, beta],
  );

  const signals = rows.filter((row) => row.kind === 'signal');
  assert.deepEqual(
    signals.map((row) => row.signal),
    [detected],
  );

  const header = rows.findIndex((row) => row.text.startsWith('DETECTED'));
  assert.ok(header > rows.indexOf(items[1] as PickRow), 'DETECTED must follow the declared items');
  assert.equal(rows[header - 1]?.text, '');

  const ids = rows.filter((row) => row.kind !== 'heading').map((row) => row.id);
  assert.equal(new Set(ids).size, ids.length, 'ids must be unique within a screen');
});

test('loops rows offer nothing selectable on an empty screen', () => {
  const rows = loops.rows?.(loopsResult(), ctx()) ?? [];
  assert.deepEqual(kinds(rows), ['heading']);
  assert.equal(rows[0]?.text, 'no open loops');
});
