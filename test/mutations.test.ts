import { test } from 'node:test';
import assert from 'node:assert/strict';

import { loadConfig } from '../src/config.ts';
import { loadState, readEventLines } from '../src/store.ts';
import type { Item } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

/**
 * The folded state is the observable surface until Task 7 lands `list` and `show`; those commands
 * get their own coverage there.
 */
function item(home: string, id: string): Item {
  const found = loadState(loadConfig(home)).items.find((candidate) => candidate.id === id);
  assert.ok(found, `no item ${id}`);
  return found;
}

async function addItem(home: string, argv: string[]): Promise<string> {
  const result = await waid(home, ['add', ...argv, '--json']);
  assert.equal(result.code, 0, result.err);
  return (result.json() as { id: string }).id;
}

test('done closes an item', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Ship the thing']);

  const result = await waid(home, ['done', id, '--json']);
  assert.equal(result.code, 0, result.err);
  assert.deepEqual(
    { ...(result.json() as Record<string, unknown>), ts: undefined },
    { id, title: 'Ship the thing', status: 'done', ts: undefined },
  );
  assert.equal(item(home, id).status, 'done');
});

test('reopen on a waiting item clears waitingOn', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Approve the copy', '--waiting-on', 'Dan']);
  assert.equal(item(home, id).status, 'waiting');

  const result = await waid(home, ['reopen', id, '--json']);
  assert.equal(result.code, 0, result.err);
  assert.equal((result.json() as { status: string }).status, 'open');

  const reopened = item(home, id);
  assert.equal(reopened.status, 'open');
  assert.equal(reopened.waitingOn, null);
});

test('reopen brings a done item back', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Ship the thing']);
  await waid(home, ['done', id]);

  const result = await waid(home, ['reopen', id]);
  assert.equal(result.code, 0, result.err);
  assert.equal(item(home, id).status, 'open');
});

test('note appends text to the item', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Chart legend overflows']);

  const result = await waid(home, ['note', id, 'blocked', 'on', 'the', 'API', '--json']);
  assert.equal(result.code, 0, result.err);
  assert.equal((result.json() as { text: string }).text, 'blocked on the API');

  const notes = item(home, id).notes;
  assert.equal(notes.length, 1);
  assert.equal(notes[0]?.text, 'blocked on the API');
  assert.ok(!Number.isNaN(Date.parse(notes[0]?.ts ?? '')), 'note ts is not a timestamp');
});

test('notes accumulate in order', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Chart legend overflows']);
  await waid(home, ['note', id, 'first']);
  await waid(home, ['note', id, 'second']);

  assert.deepEqual(
    item(home, id).notes.map((entry) => entry.text),
    ['first', 'second'],
  );
});

for (const command of ['done', 'reopen', 'note']) {
  test(`${command} with an unknown id exits 1 and writes nothing`, async () => {
    const home = makeHome();
    await addItem(home, ['Ship the thing']);
    const before = readEventLines(loadConfig(home)).length;

    const result = await waid(home, [command, 'zzzz', 'some text']);
    assert.equal(result.code, 1);
    assert.match(result.err, /unknown item id: zzzz/);
    assert.equal(readEventLines(loadConfig(home)).length, before);
  });

  test(`${command} with no id exits 1`, async () => {
    const home = makeHome();
    const result = await waid(home, [command]);

    assert.equal(result.code, 1);
    assert.match(result.err, /item id/);
  });
}

test('note with no text exits 1 and writes nothing', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Chart legend overflows']);
  const before = readEventLines(loadConfig(home)).length;

  const result = await waid(home, ['note', id, '   ']);
  assert.equal(result.code, 1);
  assert.match(result.err, /text/);
  assert.equal(readEventLines(loadConfig(home)).length, before);
});

test('human output is a single line per mutation', async () => {
  const home = makeHome();
  const id = await addItem(home, ['Ship the thing']);

  const closed = await waid(home, ['done', id]);
  assert.equal(closed.out.trimEnd(), `closed ${id}  Ship the thing`);

  const reopened = await waid(home, ['reopen', id]);
  assert.equal(reopened.out.trimEnd(), `reopened ${id}  Ship the thing`);

  const noted = await waid(home, ['note', id, 'still going']);
  assert.equal(noted.out.trimEnd(), `noted ${id}  still going`);
});
