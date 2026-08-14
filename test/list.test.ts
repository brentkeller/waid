import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Item } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

type ListResult = {
  items: Item[];
  filters: { status: string | null; project: string | null; tag: string[]; all: boolean };
};

type ShowResult = {
  item: Item;
  history: { ts: string; ev: string; line: number }[];
};

async function addItem(home: string, argv: string[]): Promise<string> {
  const result = await waid(home, ['add', ...argv, '--json']);
  assert.equal(result.code, 0, result.err);
  return (result.json() as { id: string }).id;
}

async function list(home: string, argv: string[] = []): Promise<ListResult> {
  const result = await waid(home, ['list', ...argv, '--json']);
  assert.equal(result.code, 0, result.err);
  return result.json() as ListResult;
}

/** Two projects, three items, one of them closed and one waiting. */
async function seed(home: string): Promise<Record<string, string>> {
  const chart = await addItem(home, [
    'Chart legend overflows',
    '-p',
    'C:\\dev\\dr\\devresults',
    '--tag',
    'bug',
  ]);
  const copy = await addItem(home, [
    'Approve report copy',
    '-p',
    'C:\\dev\\dr\\devresults',
    '--waiting-on',
    'Dan',
  ]);
  const repo = await addItem(home, [
    'Decide on the events repo',
    '-p',
    'C:\\dev\\waid',
    '--tag',
    'design',
    '--tag',
    'bug',
  ]);
  const shipped = await addItem(home, ['Ship the thing', '-p', 'C:\\dev\\waid']);
  await waid(home, ['done', shipped]);

  return { chart, copy, repo, shipped };
}

test('list hides done items by default and --all reveals them', async () => {
  const home = makeHome();
  const ids = await seed(home);

  const visible = await list(home);
  assert.deepEqual(
    visible.items.map((item) => item.id).sort(),
    [ids['chart'], ids['copy'], ids['repo']].sort(),
  );
  assert.equal(visible.filters.all, false);

  const all = await list(home, ['--all']);
  assert.ok(all.items.some((item) => item.id === ids['shipped']));
  assert.equal(all.filters.all, true);
});

test('--status filters to one status and implies --all', async () => {
  const home = makeHome();
  const ids = await seed(home);

  const waiting = await list(home, ['--status', 'waiting']);
  assert.deepEqual(
    waiting.items.map((item) => item.id),
    [ids['copy']],
  );
  assert.equal(waiting.filters.status, 'waiting');

  const doneOnly = await list(home, ['--status', 'done']);
  assert.deepEqual(
    doneOnly.items.map((item) => item.id),
    [ids['shipped']],
  );
});

test('an unknown --status exits 1', async () => {
  const home = makeHome();
  await seed(home);

  const result = await waid(home, ['list', '--status', 'nope']);
  assert.equal(result.code, 1);
  assert.match(result.err, /status/);
});

test('--tag filters to items carrying the tag', async () => {
  const home = makeHome();
  const ids = await seed(home);

  const bugs = await list(home, ['--tag', 'bug']);
  assert.deepEqual(bugs.items.map((item) => item.id).sort(), [ids['chart'], ids['repo']].sort());
  assert.deepEqual(bugs.filters.tag, ['bug']);

  const both = await list(home, ['--tag', 'bug', '--tag', 'design']);
  assert.deepEqual(
    both.items.map((item) => item.id),
    [ids['repo']],
  );
});

test('a partial --project resolves against known projects and filters', async () => {
  const home = makeHome();
  const ids = await seed(home);

  const waidOnly = await list(home, ['-p', 'waid']);
  assert.deepEqual(
    waidOnly.items.map((item) => item.id),
    [ids['repo']],
  );
  assert.equal(waidOnly.filters.project, 'C:\\dev\\waid');
});

test('filters combine conjunctively', async () => {
  const home = makeHome();
  const ids = await seed(home);

  const result = await list(home, ['-p', 'devresults', '--tag', 'bug']);
  assert.deepEqual(
    result.items.map((item) => item.id),
    [ids['chart']],
  );
});

test('human list groups by project', async () => {
  const home = makeHome();
  const ids = await seed(home);
  await addItem(home, ['Loose end with no project']);

  const result = await waid(home, ['list']);
  assert.equal(result.code, 0, result.err);

  const lines = result.out.split('\n');
  const headings = lines.filter((line) => line.startsWith('  ') && !line.startsWith('    '));
  assert.deepEqual(
    headings.map((line) => line.trim()),
    ['C:\\dev\\dr\\devresults', 'C:\\dev\\waid', '(no project)'],
  );

  const itemLine = lines.find((line) => line.includes(ids['chart'] ?? ''));
  assert.ok(itemLine, 'no line for the chart item');
  assert.match(itemLine, /^ {4}/);
  assert.match(itemLine, /open/);
  assert.match(itemLine, /Chart legend overflows/);
  assert.match(itemLine, /\[bug]/);
});

test('an empty list renders an explicit message and exits 0', async () => {
  const home = makeHome();

  const result = await waid(home, ['list']);
  assert.equal(result.code, 0, result.err);
  assert.match(result.out, /no items/i);
});

test('show --json returns the item with notes and one history entry per event', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Chart legend overflows', '-p', 'C:\\dev\\waid']);
  await waid(home, ['note', id, 'blocked on the API']);
  await waid(home, ['done', id]);

  const result = await waid(home, ['show', id, '--json']);
  assert.equal(result.code, 0, result.err);

  const data = result.json() as ShowResult;
  assert.equal(data.item.id, id);
  assert.equal(data.item.status, 'done');
  assert.equal(data.item.project, 'C:\\dev\\waid');
  assert.deepEqual(
    data.item.notes.map((note) => note.text),
    ['blocked on the API'],
  );
  assert.deepEqual(
    data.history.map((entry) => entry.ev),
    ['add', 'note', 'close'],
  );
  assert.deepEqual(
    data.history.map((entry) => entry.line),
    [1, 2, 3],
  );
  for (const entry of data.history) {
    assert.ok(!Number.isNaN(Date.parse(entry.ts)), `history ts is not a timestamp: ${entry.ts}`);
  }
});

test('show history ignores events for other items', async () => {
  const home = makeHome();
  const mine = await addItem(home, ['Mine']);
  const other = await addItem(home, ['Other']);
  await waid(home, ['note', other, 'not mine']);

  const result = await waid(home, ['show', mine, '--json']);
  const data = result.json() as ShowResult;
  assert.deepEqual(
    data.history.map((entry) => entry.ev),
    ['add'],
  );
});

test('show on an unknown id exits 1', async () => {
  const home = makeHome();
  await addItem(home, ['Chart legend overflows']);

  const result = await waid(home, ['show', 'zzzz']);
  assert.equal(result.code, 1);
  assert.match(result.err, /unknown item id: zzzz/);
});

test('show with no id exits 1', async () => {
  const home = makeHome();

  const result = await waid(home, ['show']);
  assert.equal(result.code, 1);
  assert.match(result.err, /item id/);
});

test('human show prints the item, its notes and its history', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Approve report copy', '--waiting-on', 'Dan', '--tag', 'copy']);
  await waid(home, ['note', id, 'pinged him again']);

  const result = await waid(home, ['show', id]);
  assert.equal(result.code, 0, result.err);
  assert.match(result.out, new RegExp(`${id}\\s+Approve report copy`));
  assert.match(result.out, /waiting/);
  assert.match(result.out, /Dan/);
  assert.match(result.out, /copy/);
  assert.match(result.out, /pinged him again/);
  assert.match(result.out, /note/);
});
