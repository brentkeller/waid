import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import { localYmd } from '../src/format.ts';
import { CACHE_VERSION } from '../src/sessions.ts';
import type { CachedSession } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

type WeekResult = {
  start: string;
  end: string;
  projects: {
    project: string | null;
    sessions: number;
    prompts: number;
    titles: string[];
    itemsClosed: number;
  }[];
  totals: { sessions: number; prompts: number; closed: number };
};

const DR = 'C:\\dev\\dr\\devresults';
const WAID = 'C:\\dev\\waid';

/** The Monday of the week `offsetWeeks` away from the current one, at local midnight. */
function monday(offsetWeeks = 0): Date {
  const now = new Date();
  const daysSinceMonday = (now.getDay() + 6) % 7;
  return new Date(
    now.getFullYear(),
    now.getMonth(),
    now.getDate() - daysSinceMonday + offsetWeeks * 7,
  );
}

/** A local wall-clock instant inside a week, `day` counted from Monday (0) to Sunday (6). */
function at(offsetWeeks: number, day: number, hours = 12): string {
  const start = monday(offsetWeeks);
  return new Date(
    start.getFullYear(),
    start.getMonth(),
    start.getDate() + day,
    hours,
    0,
    0,
    0,
  ).toISOString();
}

type SessionSpec = {
  id: string;
  title?: string;
  project?: string | null;
  started: string;
  ended?: string;
  prompts?: number;
};

function session(spec: SessionSpec): CachedSession {
  return {
    id: spec.id,
    title: spec.title ?? `session ${spec.id}`,
    project: spec.project ?? WAID,
    branch: null,
    started: spec.started,
    ended: spec.ended ?? spec.started,
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

async function week(home: string, argv: string[] = []): Promise<WeekResult> {
  const result = await waid(home, ['week', ...argv, '--json']);
  assert.equal(result.code, 0, result.err);
  return result.json() as WeekResult;
}

test('the current week covers Monday to Sunday and excludes everything outside it', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({ id: 'mon', started: at(0, 0, 9) }),
    session({ id: 'sun', started: at(0, 6, 23) }),
    session({ id: 'last-sun', started: at(-1, 6, 23) }),
    session({ id: 'next-mon', started: at(1, 0, 0) }),
  ]);

  const data = await week(home);

  assert.equal(data.start, localYmd(monday()));
  assert.equal(data.end, localYmd(new Date(at(0, 6))));
  assert.deepEqual(data.projects[0]?.titles, ['session mon', 'session sun']);
  assert.equal(data.totals.sessions, 2);
});

test('--last selects the previous window', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({ id: 'this', started: at(0, 1, 9) }),
    session({ id: 'prev', started: at(-1, 1, 9) }),
  ]);

  const data = await week(home, ['--last']);

  assert.equal(data.start, localYmd(monday(-1)));
  assert.equal(data.end, localYmd(new Date(at(-1, 6))));
  assert.deepEqual(data.projects[0]?.titles, ['session prev']);
  assert.equal(data.totals.sessions, 1);
});

test('projects are ordered by prompt count descending, each listing its session titles', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({ id: 'a', title: 'Fix the legend', started: at(0, 0, 9), prompts: 5 }),
    session({ id: 'b', title: 'Ship the rollup', started: at(0, 2, 9), prompts: 6 }),
    session({
      id: 'c',
      title: 'Migrate the aspx charts',
      project: DR,
      started: at(0, 1, 9),
      prompts: 40,
    }),
  ]);

  const data = await week(home);

  assert.deepEqual(
    data.projects.map((group) => group.project),
    [DR, WAID],
  );
  assert.deepEqual(data.projects[0], {
    project: DR,
    sessions: 1,
    prompts: 40,
    titles: ['Migrate the aspx charts'],
    itemsClosed: 0,
  });
  assert.deepEqual(data.projects[1]?.titles, ['Fix the legend', 'Ship the rollup']);
});

test('items closed in the window are counted per project', async () => {
  const home = makeHome();
  seedSessions(home, [session({ id: 'a', started: at(0, 0, 9), prompts: 3 })]);
  seedLog(home, [
    { ts: at(-2, 0, 9), ev: 'add', id: 'zz11', title: 'Ship the thing', project: WAID },
    { ts: at(-2, 0, 9), ev: 'add', id: 'k3f9', title: 'Chart legend overflows', project: DR },
    { ts: at(-2, 0, 9), ev: 'add', id: 'p2vn', title: 'Old news', project: DR },
    { ts: at(0, 1, 10), ev: 'close', id: 'zz11' },
    { ts: at(0, 2, 10), ev: 'close', id: 'k3f9' },
    // Closed before the window opened, so it belongs to an earlier week.
    { ts: at(-1, 2, 10), ev: 'close', id: 'p2vn' },
  ]);

  const data = await week(home);

  const groups = new Map(data.projects.map((group) => [group.project, group]));
  assert.equal(groups.get(WAID)?.itemsClosed, 1);
  assert.equal(groups.get(DR)?.itemsClosed, 1);
  // A project with a closure but no sessions still earns a group.
  assert.equal(groups.get(DR)?.sessions, 0);
});

test('totals are the sum of the groups', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({ id: 'a', started: at(0, 0, 9), prompts: 5 }),
    session({ id: 'b', started: at(0, 2, 9), prompts: 6 }),
    session({ id: 'c', project: DR, started: at(0, 1, 9), prompts: 40 }),
  ]);
  seedLog(home, [
    { ts: at(-2, 0, 9), ev: 'add', id: 'zz11', title: 'Ship the thing', project: WAID },
    { ts: at(0, 1, 10), ev: 'close', id: 'zz11' },
  ]);

  const data = await week(home);
  const sum = (pick: (group: WeekResult['projects'][number]) => number): number =>
    data.projects.reduce((total, group) => total + pick(group), 0);

  assert.equal(data.totals.sessions, sum((group) => group.sessions));
  assert.equal(data.totals.prompts, sum((group) => group.prompts));
  assert.equal(data.totals.closed, sum((group) => group.itemsClosed));
  assert.deepEqual(data.totals, { sessions: 3, prompts: 51, closed: 1 });
});

test('a week with nothing renders an explicit message and exits 0', async () => {
  const home = makeHome();
  seedSessions(home, []);

  const result = await waid(home, ['week']);
  assert.equal(result.code, 0, result.err);
  assert.match(result.out, /nothing recorded for the week of \d{4}-\d{2}-\d{2}/);
});

test('human week lists project rollups with their titles and a total', async () => {
  const home = makeHome();
  seedSessions(home, [
    session({ id: 'a', title: 'Fix the legend', started: at(0, 0, 9), prompts: 5 }),
    session({
      id: 'c',
      title: 'Migrate the aspx charts',
      project: DR,
      started: at(0, 1, 9),
      prompts: 40,
    }),
  ]);
  seedLog(home, [
    { ts: at(-2, 0, 9), ev: 'add', id: 'zz11', title: 'Ship the thing', project: WAID },
    { ts: at(0, 1, 10), ev: 'close', id: 'zz11' },
  ]);

  const result = await waid(home, ['week']);
  assert.equal(result.code, 0, result.err);

  const lines = result.out.split('\n');
  assert.equal(lines[0], `WEEK  ${localYmd(monday())} → ${localYmd(new Date(at(0, 6)))}`);

  const headings = lines.filter((line) => /^ {2}\S/.test(line));
  assert.equal(headings[0], `  ${DR}  1 session, 40 prompts, 0 closed`);
  assert.equal(headings[1], `  ${WAID}  1 session, 5 prompts, 1 closed`);
  assert.equal(headings[2], '  TOTAL  2 sessions, 45 prompts, 1 closed');

  assert.ok(
    lines.some((line) => line === '    Migrate the aspx charts'),
    'the session title is missing',
  );
});
