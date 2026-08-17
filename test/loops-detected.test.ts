import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import type { Deps } from '../src/cli.ts';
import type { DoctorResult } from '../src/commands/doctor.ts';
import { GH_CACHE_VERSION } from '../src/gh.ts';
import type { Item, GhClient, GhPr, GitClient, Signal } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

type LoopsResult = {
  groups: { project: string | null; items: Item[] }[];
  detected: Signal[];
  dismissedCount: number;
  notes: string[];
};

const DAY_MS = 24 * 60 * 60 * 1000;

function tempDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'waid-loops-'));
}

/** Creates `<root>/<name>` and marks it a repo by giving it a `.git` directory. */
function makeRepo(root: string, name: string): string {
  const dir = path.join(root, name);
  fs.mkdirSync(path.join(dir, '.git'), { recursive: true });
  return dir;
}

/** A home configured to scan `root` with GitHub signals on, written before the CLI first runs. */
function setup(root: string): string {
  const home = makeHome();
  fs.writeFileSync(
    path.join(home, 'config.json'),
    JSON.stringify({ scanRoots: [root], scanMaxDepth: 3, activeWindowDays: 30, ghUser: 'me' }),
  );
  return home;
}

function seedLog(home: string, events: Record<string, unknown>[]): void {
  const lines = events.map((event) => JSON.stringify(event));
  fs.writeFileSync(path.join(home, 'events.jsonl'), `${lines.join('\n')}\n`);
}

function daysAgo(days: number): Date {
  return new Date(Date.now() - days * DAY_MS);
}

type RepoState = { head?: Date; branch?: string | null; ahead?: number | null; dirty?: number };

/** A `GitClient` answering from a per-repo table; anything unlisted reads as a quiet, clean repo. */
function fakeGit(repos: Record<string, RepoState>): GitClient {
  const at = (repo: string): RepoState => repos[repo] ?? {};
  return {
    headCommitDate: (repo) => at(repo).head ?? daysAgo(1),
    currentBranch: (repo) => {
      const branch = at(repo).branch;
      return branch === undefined ? 'main' : branch;
    },
    aheadCount: (repo) => at(repo).ahead ?? null,
    dirtyFileCount: (repo) => at(repo).dirty ?? 0,
    isRepo: () => true,
  };
}

/** A git client that survives the activity gate and then blows up mid-detection. */
function explodingGit(): GitClient {
  return {
    ...fakeGit({}),
    currentBranch: () => {
      throw new Error('git exploded');
    },
  };
}

function pr(repository: string, number: number, overrides: Partial<GhPr> = {}): GhPr {
  return {
    number,
    repository,
    title: `PR ${number}`,
    author: 'tmoore',
    isDraft: false,
    state: 'open',
    createdAt: daysAgo(3).toISOString(),
    url: `https://github.com/${repository}/pull/${number}`,
    branch: `pr-${number}-branch`,
    ...overrides,
  };
}

function fakeGh(reviewRequested: GhPr[], authored: GhPr[]): GhClient {
  return { reviewRequested: () => reviewRequested, authored: () => authored };
}

/** `--no-sync` keeps the real `~/.claude` out of every run now that `loops` needs sessions. */
async function loops(
  home: string,
  argv: string[],
  deps: Deps,
): Promise<{ code: number; out: string; err: string; data: LoopsResult }> {
  const result = await waid(home, ['loops', ...argv, '--no-sync', '--json'], deps);
  return { ...result, data: result.json() as LoopsResult };
}

test('loops --json carries declared items and detected signals in separate arrays', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'inl-prt');
  const home = setup(root);
  seedLog(home, [
    { ts: '2026-08-09T09:00:00.000Z', ev: 'add', id: 'k3f9', title: 'Chart legend', project: repo },
    { ts: '2026-08-10T09:00:00.000Z', ev: 'dismiss', key: 'pr:acme/web#456' },
  ]);

  const { code, data } = await loops(home, [], {
    git: fakeGit({ [repo]: { branch: 'inl-prt-fixes', ahead: null, dirty: 14 } }),
    gh: fakeGh([pr('acme/web', 123)], [pr('acme/web', 456)]),
  });

  assert.equal(code, 0);
  assert.deepEqual(
    data.groups.map((group) => group.items.map((item) => item.id)),
    [['k3f9']],
  );
  assert.deepEqual(
    data.detected.map((signal) => signal.key),
    ['review:acme/web#123', `dirty:${repo}`],
  );
  assert.equal(data.dismissedCount, 1);
  assert.deepEqual(data.notes, []);
});

test('human loops renders DETECTED beneath the declared items, with the spec header and hint', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'inl-prt');
  const home = setup(root);
  seedLog(home, [
    { ts: '2026-08-09T09:00:00.000Z', ev: 'add', id: 'k3f9', title: 'Chart legend', project: repo },
    { ts: '2026-08-10T09:00:00.000Z', ev: 'dismiss', key: 'pr:acme/web#456' },
  ]);

  const result = await waid(home, ['loops', '--no-sync'], {
    git: fakeGit({ [repo]: { branch: 'inl-prt-fixes', ahead: null, dirty: 14 } }),
    gh: fakeGh(
      [pr('acme/web', 123, { title: 'Fix the chart legend', branch: 'chart-legend' })],
      [pr('acme/web', 456)],
    ),
  });

  assert.equal(result.code, 0, result.err);
  const lines = result.out.split('\n');

  assert.equal(lines[0], 'OPEN LOOPS');
  const header = lines.findIndex((line) => line.startsWith('DETECTED'));
  assert.ok(header > 0, 'no DETECTED header');
  assert.equal(lines[header], 'DETECTED  (2 shown, 1 dismissed)');
  assert.equal(lines[header - 1], '', 'the DETECTED section is not separated from the items');
  assert.equal(lines[header + 1], '');

  // The declared item stays above the header; the signals stay below it.
  const item = lines.findIndex((line) => line.includes('k3f9'));
  assert.ok(item > 0 && item < header, 'the declared item must render above DETECTED');
  assert.match(
    lines[header + 2] ?? '',
    /^ {2}review:acme\/web#123 +Fix the chart legend +@tmoore · chart-legend · 3d$/,
  );
  assert.match(
    lines[header + 3] ?? '',
    // The HEAD date is relative to when the fake was built, so the age is a shape, not a value.
    new RegExp(`^ {2}dirty:${escapeRe(repo)} +14 uncommitted files +inl-prt-fixes · \\d+[hd]$`),
  );
  assert.match(result.out, /waid promote <key> to track · waid dismiss <key> to hide/);
});

test('loops -p narrows declared items and detected signals together', async () => {
  const root = tempDir();
  const alpha = makeRepo(root, 'alpha');
  const beta = makeRepo(root, 'beta');
  const home = setup(root);
  seedLog(home, [
    { ts: '2026-08-09T09:00:00.000Z', ev: 'add', id: 'k3f9', title: 'In alpha', project: alpha },
    { ts: '2026-08-10T09:00:00.000Z', ev: 'add', id: 'm7qz', title: 'In beta', project: beta },
  ]);

  const deps: Deps = {
    git: fakeGit({ [alpha]: { branch: 'main', dirty: 2 }, [beta]: { branch: 'main', dirty: 3 } }),
    gh: fakeGh([], []),
  };

  const { data } = await loops(home, ['-p', 'beta'], deps);
  assert.deepEqual(
    data.groups.flatMap((group) => group.items.map((item) => item.id)),
    ['m7qz'],
  );
  assert.deepEqual(
    data.detected.map((signal) => signal.key),
    [`dirty:${beta}`],
  );
});

test('loops surfaces the unavailable-gh note without losing git signals', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  const home = setup(root);

  const { data } = await loops(home, [], {
    git: fakeGit({ [repo]: { branch: 'main', dirty: 3 } }),
    gh: {
      reviewRequested: () => {
        throw new Error('gh: command not found');
      },
      authored: () => [],
    },
  });

  assert.deepEqual(
    data.detected.map((signal) => signal.key),
    [`dirty:${repo}`],
  );
  assert.equal(data.notes.length, 1);
  assert.match(data.notes[0] ?? '', /GitHub signals unavailable/);
});

test('a failed detection degrades to declared items plus a note and still exits 0', async () => {
  const root = tempDir();
  makeRepo(root, 'alpha');
  const home = setup(root);
  seedLog(home, [
    { ts: '2026-08-09T09:00:00.000Z', ev: 'add', id: 'k3f9', title: 'Chart legend' },
  ]);

  const deps: Deps = { git: explodingGit(), gh: fakeGh([], []) };

  const { code, data } = await loops(home, [], deps);
  assert.equal(code, 0);
  assert.deepEqual(
    data.groups.flatMap((group) => group.items.map((item) => item.id)),
    ['k3f9'],
  );
  assert.deepEqual(data.detected, []);
  assert.equal(data.dismissedCount, 0);
  assert.equal(data.notes.length, 1);
  assert.match(data.notes[0] ?? '', /detection failed.*git exploded/);

  const human = await waid(home, ['loops', '--no-sync'], deps);
  assert.equal(human.code, 0, human.err);
  assert.match(human.out, /OPEN LOOPS/);
  assert.match(human.out, /Chart legend/);
  assert.match(human.out, /detection failed/);
  assert.doesNotMatch(human.out, /DETECTED/);
});

test('doctor reports gh availability, both cache ages and repo counts', async () => {
  const root = tempDir();
  const alpha = makeRepo(root, 'alpha');
  makeRepo(root, 'stale');
  const home = setup(root);

  fs.mkdirSync(path.join(home, 'cache'), { recursive: true });
  fs.writeFileSync(
    path.join(home, 'cache', 'gh.json'),
    JSON.stringify({
      version: GH_CACHE_VERSION,
      cachedAt: new Date(Date.now() - 7 * 60 * 1000).toISOString(),
      reviewRequested: [],
      authored: [],
    }),
  );

  const result = await waid(home, ['doctor', '--json'], {
    git: fakeGit({ [alpha]: { head: daysAgo(1) }, [path.join(root, 'stale')]: { head: daysAgo(400) } }),
  });
  assert.equal(result.code, 0, result.err);
  const data = result.json() as DoctorResult;

  // `WAID_SKIP_GH_DETECT` is set by `makeHome`, so detection always fails and config supplies the login.
  assert.equal(data.gh.available, false);
  assert.equal(data.gh.user, 'me');
  assert.equal(data.cache.gh.ageMinutes, 7);
  assert.equal(data.cache.sessions.exists, false);
  assert.deepEqual(data.repos.roots, [root]);
  assert.equal(data.repos.discovered, 2);
  assert.equal(data.repos.active, 1);
  assert.equal(data.ok, true);

  const human = await waid(home, ['doctor'], {
    git: fakeGit({ [alpha]: { head: daysAgo(1) }, [path.join(root, 'stale')]: { head: daysAgo(400) } }),
  });
  assert.match(human.out, /\n {2}repos {2,}2 discovered, 1 active/);
  assert.match(human.out, /gh fetched 7m ago/);
});

test('doctor reports a missing gh cache without inventing an age', async () => {
  const home = setup(tempDir());

  const result = await waid(home, ['doctor', '--json'], { git: fakeGit({}) });
  const data = result.json() as DoctorResult;
  assert.equal(data.cache.gh.ageMinutes, null);
  assert.equal(data.repos.discovered, 0);
  assert.equal(data.repos.active, 0);
});

function escapeRe(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
