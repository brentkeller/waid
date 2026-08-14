import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ALPHABET } from '../src/ids.ts';
import { makeHome, waid } from './helpers.ts';

type AddResult = {
  id: string;
  title: string;
  status: string;
  project: string | null;
  tags: string[];
  waitingOn: string | null;
  session: string | null;
  created: string;
};

async function add(home: string, argv: string[]): Promise<AddResult> {
  const result = await waid(home, ['add', ...argv, '--json']);
  assert.equal(result.code, 0, result.err);
  return result.json() as AddResult;
}

test('add returns a complete record with a 4-char id', async () => {
  const home = makeHome();
  const record = await add(home, [
    'Chart legend overflows',
    '-p',
    'C:\\dev\\waid',
    '--tag',
    'bug',
    '--tag',
    'ui',
    '--session',
    'sess-1',
  ]);

  assert.equal(record.title, 'Chart legend overflows');
  assert.equal(record.status, 'open');
  assert.equal(record.project, 'C:\\dev\\waid');
  assert.deepEqual(record.tags, ['bug', 'ui']);
  assert.equal(record.waitingOn, null);
  assert.equal(record.session, 'sess-1');
  assert.equal(record.id.length, 4);
  for (const char of record.id) assert.ok(ALPHABET.includes(char), `unexpected id char ${char}`);
  assert.ok(!Number.isNaN(Date.parse(record.created)), 'created is not a timestamp');
});

test('add joins its positionals into the title', async () => {
  const home = makeHome();
  const record = await add(home, ['Decide', 'whether', 'to', 'ship']);

  assert.equal(record.title, 'Decide whether to ship');
  assert.equal(record.project, null);
});

test('add --waiting-on yields waiting status and a populated waitingOn', async () => {
  const home = makeHome();
  const record = await add(home, ['Approve the copy', '--waiting-on', 'Dan']);

  assert.equal(record.status, 'waiting');
  assert.equal(record.waitingOn, 'Dan');
});

test('a partial -p resolves against a project used by an earlier add', async () => {
  const home = makeHome();
  await add(home, ['First', '-p', 'C:\\dev\\dr\\devresults']);
  const record = await add(home, ['Second', '-p', 'devresults']);

  assert.equal(record.project, 'C:\\dev\\dr\\devresults');
});

test('an ambiguous partial -p exits 1 and lists the candidates', async () => {
  const home = makeHome();
  await add(home, ['First', '-p', 'C:\\dev\\alpha']);
  await add(home, ['Second', '-p', 'C:\\dev\\beta']);

  const result = await waid(home, ['add', 'Third', '-p', 'dev']);
  assert.equal(result.code, 1);
  assert.match(result.err, /C:\\dev\\alpha/);
  assert.match(result.err, /C:\\dev\\beta/);
});

test('add with no title exits 1', async () => {
  const home = makeHome();
  const result = await waid(home, ['add']);

  assert.equal(result.code, 1);
  assert.match(result.err, /title/);
});

test('add with a blank title exits 1', async () => {
  const home = makeHome();
  const result = await waid(home, ['add', '   ']);

  assert.equal(result.code, 1);
  assert.match(result.err, /title/);
});

test('human output is a single added <id>  <title> line', async () => {
  const home = makeHome();
  const result = await waid(home, ['add', 'Ship the thing']);

  assert.equal(result.code, 0);
  const lines = result.out.trimEnd().split('\n');
  assert.equal(lines.length, 1);
  assert.match(lines[0] ?? '', /^added [0-9a-z]{4} {2}Ship the thing$/);
});

test('each add appends one event and ids stay unique', async () => {
  const home = makeHome();
  const ids = new Set<string>();
  for (let i = 0; i < 5; i += 1) ids.add((await add(home, [`Item ${i}`])).id);

  assert.equal(ids.size, 5);

  const listed = await waid(home, ['add', 'Sixth', '--json']);
  assert.equal(listed.code, 0);
  assert.ok(!ids.has((listed.json() as AddResult).id));
});
