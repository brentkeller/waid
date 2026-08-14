import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import type { Deps } from '../src/cli.ts';
import type { Item } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

type LoopsResult = {
  groups: { project: string | null; items: Item[] }[];
  detected: { key: string }[];
  dismissedCount: number;
  notes: string[];
};

const DR = 'C:\\dev\\dr\\devresults';
const WAID = 'C:\\dev\\waid';

/** Detection seams that find nothing, so these tests stay about declared items only. */
const QUIET: Deps = { gh: { reviewRequested: () => [], authored: () => [] } };

/**
 * A home with no scan roots, so detection has no repo to look at. `config.json` is written before
 * the CLI runs, since `ensureHome` never overwrites an existing one.
 */
function setup(): string {
  const dir = makeHome();
  fs.writeFileSync(
    path.join(dir, 'config.json'),
    JSON.stringify({ scanRoots: [], ghUser: 'me' }),
  );
  return dir;
}

/** Writes the log directly so `updated` timestamps — and therefore ordering — are deterministic. */
function seedLog(home: string, events: Record<string, unknown>[]): void {
  const lines = events.map((event) => JSON.stringify(event));
  fs.writeFileSync(path.join(home, 'events.jsonl'), `${lines.join('\n')}\n`);
}

/** Two projects plus an orphan, one closed item, spread across five days. */
function seed(home: string): void {
  seedLog(home, [
    { ts: '2026-08-08T09:00:00.000Z', ev: 'add', id: 'q8xt', title: 'Loose end with no project' },
    {
      ts: '2026-08-09T09:00:00.000Z',
      ev: 'add',
      id: 'k3f9',
      title: 'Chart legend overflows at 4+ series',
      project: DR,
      tags: ['bug'],
    },
    { ts: '2026-08-10T09:00:00.000Z', ev: 'add', id: 'zz11', title: 'Ship the thing', project: WAID },
    { ts: '2026-08-11T09:00:00.000Z', ev: 'close', id: 'zz11' },
    {
      ts: '2026-08-12T09:00:00.000Z',
      ev: 'add',
      id: 'm7qz',
      title: 'Approve report-template copy',
      project: DR,
      status: 'waiting',
      waitingOn: 'Dan',
    },
    {
      ts: '2026-08-13T09:00:00.000Z',
      ev: 'add',
      id: 'p2vn',
      title: 'Decide whether events.jsonl gets its own repo',
      project: WAID,
    },
  ]);
}

async function loops(home: string, argv: string[] = []): Promise<LoopsResult> {
  const result = await waid(home, ['loops', ...argv, '--no-sync', '--json'], QUIET);
  assert.equal(result.code, 0, result.err);
  return result.json() as LoopsResult;
}

function ids(data: LoopsResult): string[] {
  return data.groups.flatMap((group) => group.items.map((item) => item.id));
}

test('loops excludes done items', async () => {
  const home = setup();
  seed(home);

  const data = await loops(home);
  assert.ok(!ids(data).includes('zz11'), 'a closed item appeared in loops');
  assert.deepEqual(ids(data).sort(), ['k3f9', 'm7qz', 'p2vn', 'q8xt']);
});

test('loops keeps waiting items alongside open ones', async () => {
  const home = setup();
  seed(home);

  const data = await loops(home);
  const waiting = data.groups
    .flatMap((group) => group.items)
    .find((item) => item.id === 'm7qz');
  assert.ok(waiting, 'the waiting item is missing');
  assert.equal(waiting.status, 'waiting');
  assert.equal(waiting.waitingOn, 'Dan');
});

test('loops groups by project with (no project) last and oldest-updated first', async () => {
  const home = setup();
  seed(home);

  const data = await loops(home);
  assert.deepEqual(
    data.groups.map((group) => group.project),
    [DR, WAID, null],
  );
  assert.deepEqual(data.groups[0]?.items.map((item) => item.id), ['k3f9', 'm7qz']);
  assert.deepEqual(data.groups[1]?.items.map((item) => item.id), ['p2vn']);
  assert.deepEqual(data.groups[2]?.items.map((item) => item.id), ['q8xt']);
});

test('-p narrows loops to one project', async () => {
  const home = setup();
  seed(home);

  const data = await loops(home, ['-p', 'waid']);
  assert.deepEqual(
    data.groups.map((group) => group.project),
    [WAID],
  );
  assert.deepEqual(ids(data), ['p2vn']);
});

test('loops reserves the detection fields even with nothing detected', async () => {
  const home = setup();
  seed(home);

  const data = await loops(home);
  assert.deepEqual(data.detected, []);
  assert.equal(data.dismissedCount, 0);
  assert.deepEqual(data.notes, []);
});

test('an empty state renders a friendly no-open-loops line and exits 0', async () => {
  const home = setup();

  const result = await waid(home, ['loops', '--no-sync'], QUIET);
  assert.equal(result.code, 0, result.err);
  assert.match(result.out, /no open loops/i);
});

test('a state of only closed items renders no open loops', async () => {
  const home = setup();
  seedLog(home, [
    { ts: '2026-08-10T09:00:00.000Z', ev: 'add', id: 'zz11', title: 'Ship the thing' },
    { ts: '2026-08-11T09:00:00.000Z', ev: 'close', id: 'zz11' },
  ]);

  const result = await waid(home, ['loops', '--no-sync'], QUIET);
  assert.equal(result.code, 0, result.err);
  assert.match(result.out, /no open loops/i);
});

test('human loops matches the spec layout', async () => {
  const home = setup();
  seed(home);

  const result = await waid(home, ['loops', '--no-sync'], QUIET);
  assert.equal(result.code, 0, result.err);

  const lines = result.out.split('\n');
  assert.equal(lines[0], 'OPEN LOOPS');
  assert.equal(lines[1], '');

  const headings = lines.filter((line) => line.startsWith('  ') && !line.startsWith('    '));
  assert.deepEqual(
    headings.map((line) => line.trim()),
    [DR, WAID, '(no project)'],
  );

  const chart = lines.find((line) => line.includes('k3f9'));
  assert.ok(chart, 'no line for the chart item');
  assert.match(chart, /^ {4}k3f9 {2}open {6}Chart legend overflows at 4\+ series/);
  assert.match(chart, /\[bug]/);
  assert.match(chart, /\d+[mhdwy]$/);

  const copy = lines.find((line) => line.includes('m7qz'));
  assert.ok(copy, 'no line for the copy item');
  assert.match(copy, /waiting/);
  assert.match(copy, /← Dan/);
});
