import { test } from 'node:test';
import assert from 'node:assert/strict';

import { foldEvents } from '../src/events.ts';
import type { Problem } from '../src/types.ts';

/** Serialises objects (and raw strings, passed through) into the lines `foldEvents` consumes. */
function lines(...entries: (Record<string, unknown> | string)[]): string[] {
  return entries.map((entry) => (typeof entry === 'string' ? entry : JSON.stringify(entry)));
}

function reasons(problems: Problem[]): [number, string][] {
  return problems.map((problem) => [problem.line, problem.reason]);
}

test('add, note, update and close fold to a single fully populated item', () => {
  const state = foldEvents(
    lines(
      {
        ts: '2026-08-14T18:22:01.004Z',
        ev: 'add',
        id: 'k3f9',
        title: 'Chart legend overflows at 4+ series',
        project: 'C:\\dev\\dr\\devresults',
        status: 'open',
        session: '34782fc3',
        tags: ['bug'],
      },
      { ts: '2026-08-14T19:02:55.881Z', ev: 'note', id: 'k3f9', text: 'repro only in Firefox' },
      { ts: '2026-08-15T14:10:02.113Z', ev: 'update', id: 'k3f9', status: 'waiting', waitingOn: 'design review' },
      { ts: '2026-08-19T09:31:44.207Z', ev: 'close', id: 'k3f9' },
    ),
  );

  assert.deepEqual(state.problems, []);
  assert.deepEqual(state.items, [
    {
      id: 'k3f9',
      title: 'Chart legend overflows at 4+ series',
      status: 'done',
      waitingOn: 'design review',
      project: 'C:\\dev\\dr\\devresults',
      session: '34782fc3',
      tags: ['bug'],
      notes: [{ ts: '2026-08-14T19:02:55.881Z', text: 'repro only in Firefox' }],
      created: '2026-08-14T18:22:01.004Z',
      updated: '2026-08-19T09:31:44.207Z',
    },
  ]);
});

test('add applies defaults for every omitted field', () => {
  const state = foldEvents(lines({ ts: '2026-08-14T18:22:01.004Z', ev: 'add', id: 'aaaa', title: 'bare' }));

  assert.deepEqual(state.items, [
    {
      id: 'aaaa',
      title: 'bare',
      status: 'open',
      waitingOn: null,
      project: null,
      session: null,
      tags: [],
      notes: [],
      created: '2026-08-14T18:22:01.004Z',
      updated: '2026-08-14T18:22:01.004Z',
    },
  ]);
});

test('update patches only the supplied fields', () => {
  const state = foldEvents(
    lines(
      {
        ts: '2026-08-14T18:00:00.000Z',
        ev: 'add',
        id: 'aaaa',
        title: 'original',
        project: 'C:\\dev\\waid',
        tags: ['bug'],
        session: 'sess-1',
      },
      { ts: '2026-08-15T18:00:00.000Z', ev: 'update', id: 'aaaa', title: 'renamed' },
    ),
  );

  const item = state.items[0];
  assert.ok(item);
  assert.equal(item.title, 'renamed');
  assert.equal(item.project, 'C:\\dev\\waid');
  assert.deepEqual(item.tags, ['bug']);
  assert.equal(item.session, 'sess-1');
  assert.equal(item.status, 'open');
  assert.equal(item.created, '2026-08-14T18:00:00.000Z');
  assert.equal(item.updated, '2026-08-15T18:00:00.000Z');
});

test('reopen sets the status open and clears waitingOn', () => {
  const state = foldEvents(
    lines(
      { ts: '2026-08-14T18:00:00.000Z', ev: 'add', id: 'aaaa', title: 'blocked', status: 'waiting', waitingOn: 'review' },
      { ts: '2026-08-15T18:00:00.000Z', ev: 'reopen', id: 'aaaa' },
    ),
  );

  const item = state.items[0];
  assert.ok(item);
  assert.equal(item.status, 'open');
  assert.equal(item.waitingOn, null);
});

test('file order wins over timestamps and updated takes the last line ts', () => {
  const state = foldEvents(
    lines(
      { ts: '2026-08-14T18:00:00.000Z', ev: 'add', id: 'aaaa', title: 'first' },
      { ts: '2026-08-20T18:00:00.000Z', ev: 'update', id: 'aaaa', title: 'later clock' },
      { ts: '2026-08-01T09:00:00.000Z', ev: 'update', id: 'aaaa', title: 'earlier clock' },
    ),
  );

  const item = state.items[0];
  assert.ok(item);
  assert.equal(item.title, 'earlier clock');
  assert.equal(item.updated, '2026-08-01T09:00:00.000Z');
});

test('dismiss and undismiss maintain the key set', () => {
  const state = foldEvents(
    lines(
      { ts: '2026-08-14T18:00:00.000Z', ev: 'dismiss', key: 'pr:o/r#123' },
      { ts: '2026-08-14T18:00:01.000Z', ev: 'dismiss', key: 'pr:o/r#124' },
      { ts: '2026-08-14T18:00:02.000Z', ev: 'dismiss', key: 'pr:o/r#123' },
      { ts: '2026-08-14T18:00:03.000Z', ev: 'undismiss', key: 'pr:o/r#124' },
      { ts: '2026-08-14T18:00:04.000Z', ev: 'undismiss', key: 'dirty:C:\\dev\\waid' },
    ),
  );

  assert.deepEqual(state.dismissed, ['pr:o/r#123']);
  assert.deepEqual(state.problems, []);
});

test('a mixed malformed batch yields the surviving items and the exact problem list', () => {
  const state = foldEvents([
    ...lines({ ts: '2026-08-14T18:00:00.000Z', ev: 'add', id: 'aaaa', title: 'good' }),
    '',
    '   ',
    '{not json',
    ...lines(
      ['an', 'array'] as unknown as Record<string, unknown>,
      { ts: '2026-08-14T18:00:01.000Z', ev: 'note', id: 'zzzz', text: 'orphan' },
      { ts: '2026-08-14T18:00:02.000Z', ev: 'teleport', id: 'aaaa' },
      { ts: '2026-08-14T18:00:03.000Z', ev: 'add', id: 'aaaa', title: 'duplicate' },
      { ts: '2026-08-14T18:00:04.000Z', ev: 'add', id: 'bbbb' },
      { ts: '2026-08-14T18:00:05.000Z', ev: 'update', id: 'aaaa', status: 'sideways' },
      { ts: '2026-08-14T18:00:06.000Z', ev: 'note', id: 'aaaa' },
      { ts: '2026-08-14T18:00:07.000Z', ev: 'dismiss' },
    ),
  ]);

  assert.deepEqual(
    state.items.map((item) => [item.id, item.title, item.status]),
    [['aaaa', 'good', 'open']],
  );
  assert.deepEqual(state.dismissed, []);
  assert.deepEqual(reasons(state.problems), [
    [4, 'unparseable'],
    [5, 'not-an-object'],
    [6, 'unknown-id'],
    [7, 'unknown-ev'],
    [8, 'duplicate-id'],
    [9, 'add-missing-fields'],
    [10, 'bad-status'],
    [11, 'note-missing-text'],
    [12, 'missing-key'],
  ]);
  assert.deepEqual(state.problems[2], { line: 6, reason: 'unknown-id', id: 'zzzz', ev: 'note' });
  assert.deepEqual(state.problems[3], { line: 7, reason: 'unknown-ev', id: 'aaaa', ev: 'teleport' });
});

test('a bad status is reported without discarding the rest of the event', () => {
  const state = foldEvents(
    lines(
      { ts: '2026-08-14T18:00:00.000Z', ev: 'add', id: 'aaaa', title: 'kept', status: 'sideways' },
      { ts: '2026-08-14T18:00:01.000Z', ev: 'update', id: 'aaaa', status: 'nope', title: 'still patched' },
    ),
  );

  const item = state.items[0];
  assert.ok(item);
  assert.equal(item.status, 'open');
  assert.equal(item.title, 'still patched');
  assert.deepEqual(reasons(state.problems), [
    [1, 'bad-status'],
    [2, 'bad-status'],
  ]);
});

test('foldEvents never throws on hostile input', () => {
  const state = foldEvents(['null', '42', '"a string"', '[]', '{}', '{"ev":"add"}']);
  assert.deepEqual(state.items, []);
  assert.equal(state.problems.length, 6);
});

test('foldEvents on an empty log returns empty arrays', () => {
  const state = foldEvents([]);
  assert.deepEqual(state, { items: [], dismissed: [], problems: [] });
});
