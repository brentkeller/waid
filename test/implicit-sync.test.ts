import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { COMMANDS } from '../src/cli.ts';
import type { Deps } from '../src/cli.ts';
import { CACHE_VERSION } from '../src/sessions.ts';
import type { CachedSession, CommandModule, Config } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

const FIXTURES = path.join(import.meta.dirname, 'fixtures', 'transcripts');

/** What the probe command reports back, so a test can see what the CLI attached. */
type Probe = {
  ids: string[] | null;
  notes: string[];
};

/** Registers a command for the duration of `body`, then removes it again. */
async function withCommand(
  name: string,
  mod: CommandModule<unknown>,
  body: () => Promise<void>,
): Promise<void> {
  COMMANDS[name] = mod;
  try {
    await body();
  } finally {
    delete COMMANDS[name];
  }
}

/** A command that reports the sessions it was handed; `needsSessions` decides whether it gets any. */
function probe(needsSessions: boolean): CommandModule<Probe> {
  return {
    run: async (ctx) => ({
      ids: ctx.sessions === undefined ? null : ctx.sessions.map((session) => session.id),
      notes: ctx.notes,
    }),
    render: (data) => JSON.stringify(data),
    needsSessions,
  };
}

function cachedSession(id: string): CachedSession {
  return {
    id,
    title: `session ${id}`,
    project: 'C:\\dev\\waid',
    branch: null,
    started: '2026-08-14T09:00:00.000Z',
    ended: '2026-08-14T09:30:00.000Z',
    prompts: 3,
    _file: { path: `C:\\transcripts\\${id}.jsonl`, mtimeMs: 1, size: 1 },
  };
}

/** Writes `cache/sessions.json` with a `syncedAt` the given number of minutes in the past. */
function writeCache(home: string, ageMinutes: number, ids: string[]): void {
  const syncedAt = new Date(Date.now() - ageMinutes * 60_000).toISOString();
  fs.mkdirSync(path.join(home, 'cache'), { recursive: true });
  fs.writeFileSync(
    path.join(home, 'cache', 'sessions.json'),
    JSON.stringify({ version: CACHE_VERSION, syncedAt, sessions: ids.map(cachedSession) }, null, 2),
  );
}

/** A sync seam that records its calls and answers with sessions the cache never held. */
function fakeSync(calls: Config[]): Deps & { calls: Config[] } {
  return {
    calls,
    syncSessions: (cfg) => {
      calls.push(cfg);
      return { sessions: [cachedSession('synced')] };
    },
  };
}

/** The probe renders one JSON line; any further lines are notes the CLI appended itself. */
function readProbe(out: string): Probe {
  return JSON.parse(out.split('\n')[0] ?? '') as Probe;
}

test('a needsSessions command syncs when the cache is missing', async () => {
  const home = makeHome();
  const calls: Config[] = [];

  await withCommand('probe', probe(true), async () => {
    const result = await waid(home, ['probe'], fakeSync(calls));

    assert.equal(result.code, 0);
    assert.equal(calls.length, 1);
    assert.deepEqual(readProbe(result.out).ids, ['synced']);
  });
});

test('a needsSessions command syncs when the cache is older than five minutes', async () => {
  const home = makeHome();
  writeCache(home, 6, ['stale']);
  const calls: Config[] = [];

  await withCommand('probe', probe(true), async () => {
    const result = await waid(home, ['probe'], fakeSync(calls));

    assert.equal(calls.length, 1);
    assert.deepEqual(readProbe(result.out).ids, ['synced']);
  });
});

test('a fresh cache is used as is, with no sync', async () => {
  const home = makeHome();
  writeCache(home, 1, ['fresh']);
  const calls: Config[] = [];

  await withCommand('probe', probe(true), async () => {
    const result = await waid(home, ['probe'], fakeSync(calls));

    assert.equal(calls.length, 0);
    assert.deepEqual(readProbe(result.out).ids, ['fresh']);
  });
});

test('--no-sync suppresses the sync for a stale cache and for a missing one', async () => {
  const stale = makeHome();
  writeCache(stale, 90, ['stale']);
  const missing = makeHome();
  const calls: Config[] = [];

  await withCommand('probe', probe(true), async () => {
    const staleRun = await waid(stale, ['probe', '--no-sync'], fakeSync(calls));
    assert.equal(calls.length, 0);
    assert.deepEqual(readProbe(staleRun.out).ids, ['stale']);

    const missingRun = await waid(missing, ['probe', '--no-sync'], fakeSync(calls));
    assert.equal(calls.length, 0);
    assert.deepEqual(readProbe(missingRun.out).ids, []);
  });
});

test('a command without needsSessions never syncs and gets no sessions', async () => {
  const home = makeHome();
  const calls: Config[] = [];

  await withCommand('probe', probe(false), async () => {
    const result = await waid(home, ['probe'], fakeSync(calls));

    assert.equal(calls.length, 0);
    assert.equal(readProbe(result.out).ids, null);
  });
});

test('a throwing sync leaves the command on the stale cache with a note and exit 0', async () => {
  const home = makeHome();
  writeCache(home, 120, ['stale']);

  await withCommand('probe', probe(true), async () => {
    const result = await waid(home, ['probe'], {
      syncSessions: () => {
        throw new Error('claudeDir is on fire');
      },
    });

    assert.equal(result.code, 0);
    const data = readProbe(result.out);
    assert.deepEqual(data.ids, ['stale']);
    assert.equal(data.notes.length, 1);
    assert.match(data.notes[0] ?? '', /claudeDir is on fire/);

    // The note is the CLI's to render, so it lands beneath the command's own output.
    assert.match(result.out.split('\n')[1] ?? '', /session sync failed/);
  });
});

test('under --json a sync failure keeps stdout a single document and notes on stderr', async () => {
  const home = makeHome();
  writeCache(home, 120, ['stale']);

  await withCommand('probe', probe(true), async () => {
    const result = await waid(home, ['probe', '--json'], {
      syncSessions: () => {
        throw new Error('claudeDir is on fire');
      },
    });

    assert.equal(result.code, 0);
    assert.deepEqual((result.json() as Probe).ids, ['stale']);
    assert.match(result.err, /session sync failed/);
  });
});

test('the real sync runs implicitly and leaves a cache behind', async () => {
  const home = makeHome();
  const claudeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'waid-claude-'));
  const projectDir = path.join(claudeDir, 'projects', 'C--dev-waid');
  fs.mkdirSync(projectDir, { recursive: true });
  fs.copyFileSync(path.join(FIXTURES, 'normal.jsonl'), path.join(projectDir, 'a1b2c3d4.jsonl'));

  fs.mkdirSync(home, { recursive: true });
  fs.writeFileSync(path.join(home, 'config.json'), JSON.stringify({ claudeDir }, null, 2));

  await withCommand('probe', probe(true), async () => {
    const result = await waid(home, ['probe']);

    assert.equal(result.code, 0);
    assert.deepEqual(readProbe(result.out), { ids: ['a1b2c3d4'], notes: [] });
    assert.ok(fs.existsSync(path.join(home, 'cache', 'sessions.json')));
  });
});
