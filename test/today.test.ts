import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import { localYmd } from '../src/format.ts';
import { CACHE_VERSION } from '../src/sessions.ts';
import type { CachedSession, Item } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

type TodayResult = {
  date: string;
  sessions: { id: string; title: string; project: string | null; prompts: number }[];
  projects: { project: string | null; sessions: number; prompts: number }[];
  items: { added: Item[]; closed: Item[]; noted: Item[] };
};

const DR = 'C:\\dev\\dr\\devresults';
const WAID = 'C:\\dev\\waid';

/** A local wall-clock instant as ISO, so day windows land the same way in every timezone. */
function at(ymd: string, hours: number, minutes = 0): string {
  const [year = 0, month = 1, day = 1] = ymd.split('-').map(Number);
  return new Date(year, month - 1, day, hours, minutes).toISOString();
}

type SessionSpec = {
  id: string;
  title?: string;
  project?: string | null;
  started: string;
  ended: string;
  prompts?: number;
};

function session(spec: SessionSpec): CachedSession {
  return {
    id: spec.id,
    title: spec.title ?? `session ${spec.id}`,
    project: spec.project ?? WAID,
    branch: null,
    started: spec.started,
    ended: spec.ended,
    prompts: spec.prompts ?? 1,
    _file: { path: `C:\\transcripts\\${spec.id}.jsonl`, mtimeMs: 1, size: 1 },
  };
}

/** Writes a cache stamped now, so the implicit sync sees it as fresh and leaves it alone. */
function seedSessions(home: string, sessions: CachedSession[]): void {
  fs.mkdirSync(path.join(home, 'cache'), { recursive: true });
  fs.writeFileSync(
    path.join(home, 'cache', 'sessions.json'),
    JSON.stringify({ version: CACHE_VERSION, syncedAt: new Date().toISOString(), sessions }),
  );
}

function seedLog(home: string, events: Record<string, unknown>[]): void {
  const lines = events.map((event) => JSON.stringify(event));
  fs.writeFileSync(path.join(home, 'events.jsonl'), `${lines.join('\n')}\n`);
}

async function today(home: string, argv: string[] = []): Promise<TodayResult> {
  const result = await waid(home, ['today', ...argv, '--json']);
  assert.equal(result.code, 0, result.err);
  return result.json() as TodayResult;
}

test('sessions on the requested day are grouped by project with prompt counts', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({ id: 'a', started: at('2026-08-10', 9), ended: at('2026-08-10', 10), prompts: 12 }),
    session({ id: 'b', started: at('2026-08-10', 14), ended: at('2026-08-10', 15), prompts: 18 }),
    session({
      id: 'c',
      project: DR,
      started: at('2026-08-10', 11),
      ended: at('2026-08-10', 12),
      prompts: 40,
    }),
    session({ id: 'd', started: at('2026-08-09', 9), ended: at('2026-08-09', 10), prompts: 99 }),
  ]);

  const data = await today(home, ['--date', '2026-08-10']);

  assert.equal(data.date, '2026-08-10');
  assert.deepEqual(
    data.sessions.map((s) => s.id),
    ['a', 'c', 'b'],
  );
  assert.deepEqual(data.projects, [
    { project: DR, sessions: 1, prompts: 40 },
    { project: WAID, sessions: 2, prompts: 30 },
  ]);
});

test('a zero-prompt session is omitted from the day', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({ id: 'real', started: at('2026-08-10', 9), ended: at('2026-08-10', 10), prompts: 4 }),
    session({ id: 'idle', started: at('2026-08-10', 9), ended: at('2026-08-10', 9), prompts: 0 }),
  ]);

  const data = await today(home, ['--date', '2026-08-10']);
  assert.deepEqual(
    data.sessions.map((s) => s.id),
    ['real'],
  );
  assert.deepEqual(data.projects, [{ project: WAID, sessions: 1, prompts: 4 }]);
});

test('a session spanning midnight appears on both days', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({
      id: 'owl',
      started: at('2026-08-10', 23, 30),
      ended: at('2026-08-11', 0, 30),
      prompts: 7,
    }),
  ]);

  const before = await today(home, ['--date', '2026-08-10']);
  const after = await today(home, ['--date', '2026-08-11']);

  assert.deepEqual(
    before.sessions.map((s) => s.id),
    ['owl'],
  );
  assert.deepEqual(
    after.sessions.map((s) => s.id),
    ['owl'],
  );
});

test('without --date the day is today, and --date selects a past one', async () => {
  const home = makeHome();
  const now = new Date();
  seedSessions(home, [
    session({ id: 'now', started: now.toISOString(), ended: now.toISOString(), prompts: 5 }),
    session({ id: 'then', started: at('2026-08-10', 9), ended: at('2026-08-10', 10), prompts: 2 }),
  ]);

  const current = await today(home);
  assert.equal(current.date, localYmd(now));
  assert.deepEqual(
    current.sessions.map((s) => s.id),
    ['now'],
  );

  const past = await today(home, ['--date', '2026-08-10']);
  assert.deepEqual(
    past.sessions.map((s) => s.id),
    ['then'],
  );
});

test('items added, closed and noted that day are listed', async () => {
  const home = makeHome();
  seedSessions(home, []);
  seedLog(home, [
    { ts: at('2026-08-09', 9), ev: 'add', id: 'zz11', title: 'Ship the thing', project: WAID },
    { ts: at('2026-08-09', 9), ev: 'add', id: 'p2vn', title: 'Decide on the repo', project: WAID },
    { ts: at('2026-08-10', 9), ev: 'add', id: 'k3f9', title: 'Chart legend overflows', project: DR },
    { ts: at('2026-08-10', 10), ev: 'close', id: 'zz11' },
    { ts: at('2026-08-10', 11), ev: 'note', id: 'p2vn', text: 'leaning yes' },
    { ts: at('2026-08-10', 12), ev: 'note', id: 'p2vn', text: 'still leaning yes' },
    { ts: at('2026-08-11', 9), ev: 'add', id: 'x9x9', title: 'Tomorrow', project: WAID },
  ]);

  const data = await today(home, ['--date', '2026-08-10']);

  assert.deepEqual(
    data.items.added.map((item) => item.id),
    ['k3f9'],
  );
  assert.deepEqual(
    data.items.closed.map((item) => item.id),
    ['zz11'],
  );
  // Two notes on one item still list it once.
  assert.deepEqual(
    data.items.noted.map((item) => item.id),
    ['p2vn'],
  );
  assert.equal(data.items.added[0]?.title, 'Chart legend overflows');
});

test('a day with nothing renders an explicit message and exits 0', async () => {
  const home = makeHome();
  seedSessions(home, []);

  const result = await waid(home, ['today', '--date', '2026-08-10']);
  assert.equal(result.code, 0, result.err);
  assert.match(result.out, /nothing recorded for 2026-08-10/);
});

test('human today lists sessions under project headings and item activity beneath', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({
      id: 'a',
      title: 'Fix budget chart legend overflow',
      started: at('2026-08-10', 9),
      ended: at('2026-08-10', 10),
      prompts: 12,
    }),
    session({
      id: 'c',
      title: 'Migrate the aspx charts',
      project: DR,
      started: at('2026-08-10', 11),
      ended: at('2026-08-10', 12),
      prompts: 40,
    }),
  ]);
  seedLog(home, [
    { ts: at('2026-08-10', 9), ev: 'add', id: 'k3f9', title: 'Chart legend overflows', project: DR },
  ]);

  const result = await waid(home, ['today', '--date', '2026-08-10']);
  assert.equal(result.code, 0, result.err);

  const lines = result.out.split('\n');
  assert.equal(lines[0], 'TODAY  2026-08-10');

  const headings = lines.filter((line) => /^ {2}\S/.test(line));
  assert.equal(headings[0], `  ${DR}  1 session, 40 prompts`);
  assert.equal(headings[1], `  ${WAID}  1 session, 12 prompts`);

  assert.ok(
    lines.some((line) => line.startsWith('    Migrate the aspx charts')),
    'the session title is missing',
  );
  assert.ok(
    lines.some((line) => /^ {4}added {3}k3f9 {2}Chart legend overflows$/.test(line)),
    'the item activity row is missing',
  );
});

test('an unparseable --date is a user error', async () => {
  const home = makeHome();
  seedSessions(home, []);

  const result = await waid(home, ['today', '--date', 'yesterday']);
  assert.equal(result.code, 1);
  assert.match(result.err, /--date/);
});
