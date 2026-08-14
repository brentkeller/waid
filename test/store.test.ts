import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { ensureHome, loadConfig } from '../src/config.ts';
import { UserError } from '../src/errors.ts';
import { appendEvent, loadState, readEventLines, readEvents, requireItem } from '../src/store.ts';
import type { Config } from '../src/types.ts';

/** An initialised home, ready for appends. */
function makeConfig(): Config {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'waid-store-'));
  ensureHome(home, { detectUser: () => null });
  return loadConfig(home);
}

/** A home whose `config.json` exists but which has never been written to. */
function makeBareConfig(): Config {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'waid-store-bare-'));
  return loadConfig(home);
}

test('appendEvent stamps a valid ISO ts and writes exactly one line per call', () => {
  const cfg = makeConfig();
  const before = Date.now();
  const event = appendEvent(cfg, { ev: 'add', id: 'aaaa', title: 'stamped' });
  const after = Date.now();

  assert.equal(event.ts, new Date(event.ts).toISOString());
  const stamped = Date.parse(event.ts);
  assert.ok(stamped >= before && stamped <= after, `${event.ts} outside the call window`);

  appendEvent(cfg, { ev: 'note', id: 'aaaa', text: 'second' });
  const raw = fs.readFileSync(cfg.eventsPath, 'utf8');
  assert.equal(raw.endsWith('\n'), true);
  assert.equal(raw.trimEnd().split('\n').length, 2);
});

test('appendEvent preserves a caller-supplied ts', () => {
  const cfg = makeConfig();
  const event = appendEvent(cfg, { ts: '2020-01-02T03:04:05.006Z', ev: 'add', id: 'aaaa', title: 'backdated' });

  assert.equal(event.ts, '2020-01-02T03:04:05.006Z');
  assert.equal(loadState(cfg).items[0]?.created, '2020-01-02T03:04:05.006Z');
});

test('appendEvent returns the event it wrote', () => {
  const cfg = makeConfig();
  const event = appendEvent(cfg, { ev: 'dismiss', key: 'pr:o/r#7' });

  const line = readEventLines(cfg).at(-1);
  assert.ok(line);
  assert.deepEqual(JSON.parse(line), event);
});

test('readEvents returns parsed events with 1-based line numbers', () => {
  const cfg = makeConfig();
  appendEvent(cfg, { ev: 'add', id: 'aaaa', title: 'first' });
  appendEvent(cfg, { ev: 'add', id: 'bbbb', title: 'second' });

  const events = readEvents(cfg);
  assert.deepEqual(
    events.map((entry) => entry.line),
    [1, 2],
  );
  assert.equal((events[0]?.ev as { id: string }).id, 'aaaa');
});

test('readEvents skips unparseable lines but keeps the surviving line numbers', () => {
  const cfg = makeConfig();
  fs.writeFileSync(cfg.eventsPath, '{not json\n{"ev":"add","id":"bbbb","title":"second","ts":"2026-01-01T00:00:00.000Z"}\n');

  const events = readEvents(cfg);
  assert.equal(events.length, 1);
  assert.equal(events[0]?.line, 2);
});

test('loadState folds what was appended', () => {
  const cfg = makeConfig();
  appendEvent(cfg, { ev: 'add', id: 'aaaa', title: 'folded' });
  appendEvent(cfg, { ev: 'note', id: 'aaaa', text: 'a note' });
  appendEvent(cfg, { ev: 'close', id: 'aaaa' });
  appendEvent(cfg, { ev: 'dismiss', key: 'dirty:C:\\dev\\waid' });

  const state = loadState(cfg);
  assert.deepEqual(state.problems, []);
  assert.deepEqual(state.dismissed, ['dirty:C:\\dev\\waid']);
  const item = requireItem(state, 'aaaa');
  assert.equal(item.status, 'done');
  assert.deepEqual(
    item.notes.map((note) => note.text),
    ['a note'],
  );
});

test('requireItem throws a UserError naming the unknown id', () => {
  const cfg = makeConfig();
  const state = loadState(cfg);

  assert.throws(
    () => requireItem(state, 'zzzz'),
    (error: unknown) => error instanceof UserError && /unknown item id: zzzz/.test(error.message),
  );
});

test('loadState on a home with no events.jsonl returns empty arrays', () => {
  const cfg = makeBareConfig();
  assert.equal(fs.existsSync(cfg.eventsPath), false);

  assert.deepEqual(loadState(cfg), { items: [], dismissed: [], problems: [] });
  assert.deepEqual(readEventLines(cfg), []);
  assert.deepEqual(readEvents(cfg), []);
});
