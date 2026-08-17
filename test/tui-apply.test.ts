import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import { loadConfig } from '../src/config.ts';
import { applyPlan } from '../src/tui/apply.ts';
import type { Config, Item, PickRow, Plan, Signal } from '../src/types.ts';
import { makeHome } from './helpers.ts';

function home(): Config {
  return loadConfig(makeHome());
}

/** Every parsed line of the event log, in file order. */
function events(cfg: Config): Record<string, unknown>[] {
  const raw = fs.readFileSync(cfg.eventsPath, 'utf8');
  return raw
    .split(/\r?\n/)
    .filter((line) => line.trim() !== '')
    .map((line) => JSON.parse(line) as Record<string, unknown>);
}

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

function signalRow(value: Signal): PickRow {
  return { kind: 'signal', id: value.key, text: `  ${value.key}`, signal: value };
}

function itemRow(value: Item): PickRow {
  return { kind: 'item', id: value.id, text: `  ${value.id}`, item: value };
}

/** Seeds the log so `applyPlan` has an item to act on without going through the CLI. */
function seed(cfg: Config, value: Item): void {
  const line = JSON.stringify({
    ts: value.created,
    ev: 'add',
    id: value.id,
    title: value.title,
    status: value.status,
    project: value.project,
    tags: value.tags,
    waitingOn: value.waitingOn,
  });
  fs.appendFileSync(cfg.eventsPath, `${line}\n`);
}

test('promote writes an add carrying the edited title then a dismiss of the key', () => {
  const cfg = home();
  const detected = signal('dirty:C:\\dev\\waid', { title: '6 uncommitted files in waid' });
  const plan: Plan = [{ row: signalRow(detected), mark: { action: 'promote', title: 'Land the picker' } }];

  const receipts = applyPlan(cfg, plan);
  const log = events(cfg);

  assert.equal(log.length, 2);
  assert.equal(log[0]?.ev, 'add');
  assert.equal(log[0]?.title, 'Land the picker');
  assert.equal(log[0]?.status, 'open');
  assert.deepEqual(log[0]?.tags, ['promoted']);
  assert.equal(log[0]?.project, 'C:\\dev\\waid');
  assert.equal(log[1]?.ev, 'dismiss');
  assert.equal(log[1]?.key, detected.key);

  assert.deepEqual(receipts, [`promoted ${log[0]?.id as string}  Land the picker`]);
});

test('dismiss writes one dismiss event', () => {
  const cfg = home();
  const detected = signal('review:acme/web#123');

  const receipts = applyPlan(cfg, [{ row: signalRow(detected), mark: { action: 'dismiss' } }]);

  const log = events(cfg);
  assert.equal(log.length, 1);
  assert.equal(log[0]?.ev, 'dismiss');
  assert.equal(log[0]?.key, 'review:acme/web#123');
  assert.match(String(log[0]?.ts), /^\d{4}-\d{2}-\d{2}T/);
  assert.deepEqual(receipts, ['dismissed review:acme/web#123']);
});

test('done writes a close event against the row id', () => {
  const cfg = home();
  const declared = item('k3f9', { title: 'Ship the loops screen' });
  seed(cfg, declared);

  const receipts = applyPlan(cfg, [{ row: itemRow(declared), mark: { action: 'done' } }]);
  const log = events(cfg);

  assert.equal(log.length, 2);
  assert.equal(log[1]?.ev, 'close');
  assert.equal(log[1]?.id, 'k3f9');
  assert.deepEqual(receipts, ['closed k3f9  Ship the loops screen']);
});

test('waiting writes an update carrying the status and who it waits on', () => {
  const cfg = home();
  const declared = item('m7qz', { title: 'Review the design' });
  seed(cfg, declared);

  const receipts = applyPlan(cfg, [
    { row: itemRow(declared), mark: { action: 'waiting', waitingOn: 'tmoore' } },
  ]);
  const log = events(cfg);

  assert.equal(log.length, 2);
  assert.equal(log[1]?.ev, 'update');
  assert.equal(log[1]?.id, 'm7qz');
  assert.equal(log[1]?.status, 'waiting');
  assert.equal(log[1]?.waitingOn, 'tmoore');
  assert.deepEqual(receipts, ['waiting m7qz  Review the design ← tmoore']);
});

test('an empty who is written as null rather than an empty string', () => {
  const cfg = home();
  const declared = item('p2xd', { title: 'Chase the release' });
  seed(cfg, declared);

  const receipts = applyPlan(cfg, [
    { row: itemRow(declared), mark: { action: 'waiting', waitingOn: '  ' } },
  ]);
  const log = events(cfg);

  assert.equal(log[1]?.waitingOn, null);
  assert.deepEqual(receipts, ['waiting p2xd  Chase the release']);
});

test('the plan is applied in row order', () => {
  const cfg = home();
  const declared = item('k3f9');
  seed(cfg, declared);
  const first = signal('review:acme/web#1');
  const second = signal('review:acme/web#2');

  const receipts = applyPlan(cfg, [
    { row: signalRow(first), mark: { action: 'promote', title: 'first' } },
    { row: itemRow(declared), mark: { action: 'done' } },
    { row: signalRow(second), mark: { action: 'dismiss' } },
  ]);

  const log = events(cfg).slice(1);
  assert.deepEqual(
    log.map((event) => event.ev),
    ['add', 'dismiss', 'close', 'dismiss'],
  );
  assert.equal(log[1]?.key, 'review:acme/web#1');
  assert.equal(log[3]?.key, 'review:acme/web#2');
  assert.equal(receipts.length, 3);
  assert.match(receipts[0] ?? '', /^promoted [a-z0-9]{4}  first$/);
  assert.equal(receipts[2], 'dismissed review:acme/web#2');
});

test('two promotes in one batch get distinct ids', () => {
  const cfg = home();
  const receipts = applyPlan(cfg, [
    { row: signalRow(signal('review:acme/web#1')), mark: { action: 'promote', title: 'first' } },
    { row: signalRow(signal('review:acme/web#2')), mark: { action: 'promote', title: 'second' } },
  ]);

  const ids = events(cfg)
    .filter((event) => event.ev === 'add')
    .map((event) => event.id);
  assert.equal(ids.length, 2);
  assert.notEqual(ids[0], ids[1]);
  assert.equal(receipts.length, 2);
});

test('marks whose kind does not match their row are skipped rather than written', () => {
  const cfg = home();
  const declared = item('k3f9');
  seed(cfg, declared);

  const receipts = applyPlan(cfg, [
    { row: itemRow(declared), mark: { action: 'promote', title: 'nope' } },
    { row: signalRow(signal('review:acme/web#1')), mark: { action: 'done' } },
  ]);

  assert.equal(events(cfg).length, 1);
  assert.deepEqual(receipts, []);
});

test('an empty plan writes nothing', () => {
  const cfg = home();
  assert.deepEqual(applyPlan(cfg, []), []);
  assert.equal(fs.existsSync(path.join(cfg.home, 'events.jsonl')), false);
});
