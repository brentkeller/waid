import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import type { Deps } from '../src/cli.ts';
import type { GhClient, GhPr, GitClient, Signal } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

type ScanResult = { signals: Signal[]; dismissedCount: number; notes: string[] };
type DismissResult = { key: string; dismissed: true; already: boolean };
type PromoteResult = { id: string; key: string; title: string; project: string | null };
type LoopsResult = { groups: { project: string | null; items: { id: string; tags: string[] }[] }[] };

const DAY_MS = 24 * 60 * 60 * 1000;

function tempDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'waid-promote-'));
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

function daysAgo(days: number): Date {
  return new Date(Date.now() - days * DAY_MS);
}

/** Every parsed line of the event log, in file order. */
function events(home: string): Record<string, unknown>[] {
  const raw = fs.readFileSync(path.join(home, 'events.jsonl'), 'utf8');
  return raw
    .split(/\r?\n/)
    .filter((line) => line.trim() !== '')
    .map((line) => JSON.parse(line) as Record<string, unknown>);
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

/** Runs a command with detection fakes; `--no-sync` keeps the real `~/.claude` out of the test. */
async function json<T>(home: string, argv: string[], deps: Deps): Promise<{ code: number; data: T }> {
  const result = await waid(home, [...argv, '--no-sync', '--json'], deps);
  return { code: result.code, data: result.json() as T };
}

test('dismiss appends one event and the key stops appearing in scan', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  const home = setup(root);
  const deps: Deps = {
    git: fakeGit({ [repo]: { branch: 'main', dirty: 4 } }),
    gh: fakeGh([pr('acme/web', 123)], []),
  };

  const before = await json<ScanResult>(home, ['scan'], deps);
  assert.deepEqual(
    before.data.signals.map((signal) => signal.key),
    ['review:acme/web#123', `dirty:${repo}`],
  );

  const dismissed = await json<DismissResult>(home, ['dismiss', 'review:acme/web#123'], deps);
  assert.equal(dismissed.code, 0);
  assert.equal(dismissed.data.key, 'review:acme/web#123');
  assert.equal(dismissed.data.already, false);

  const log = events(home);
  assert.equal(log.length, 1);
  assert.equal(log[0]?.ev, 'dismiss');
  assert.equal(log[0]?.key, 'review:acme/web#123');

  const after = await json<ScanResult>(home, ['scan'], deps);
  assert.deepEqual(
    after.data.signals.map((signal) => signal.key),
    [`dirty:${repo}`],
  );
  assert.equal(after.data.dismissedCount, 1);
});

test('dismissing an already-dismissed key is a no-op that exits 0', async () => {
  const root = tempDir();
  const home = setup(root);
  const deps: Deps = { git: fakeGit({}), gh: fakeGh([], []) };

  await json<DismissResult>(home, ['dismiss', 'pr:acme/web#9'], deps);
  const again = await json<DismissResult>(home, ['dismiss', 'pr:acme/web#9'], deps);

  assert.equal(again.code, 0);
  assert.equal(again.data.already, true);
  assert.equal(events(home).length, 1);
});

test('dismiss with no key exits 1', async () => {
  const home = setup(tempDir());
  const result = await waid(home, ['dismiss', '--no-sync'], {});
  assert.equal(result.code, 1);
  assert.match(result.err, /dismiss requires a signal key/);
});

test('promote writes an add then a dismiss and moves the signal into loops', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  const home = setup(root);
  const deps: Deps = {
    git: fakeGit({ [repo]: { branch: 'topic', ahead: 3, dirty: 0 } }),
    gh: fakeGh([], []),
  };

  const key = `ahead:${repo}:topic`;
  const promoted = await json<PromoteResult>(home, ['promote', key], deps);

  assert.equal(promoted.code, 0);
  assert.equal(promoted.data.key, key);
  assert.equal(promoted.data.project, repo);
  assert.equal(promoted.data.title, '3 unpushed commits on topic in alpha');
  assert.match(promoted.data.id, /^[a-z0-9]{4}$/);

  const log = events(home);
  assert.equal(log.length, 2);
  assert.equal(log[0]?.ev, 'add');
  assert.equal(log[0]?.id, promoted.data.id);
  assert.deepEqual(log[0]?.tags, ['promoted']);
  assert.equal(log[0]?.project, repo);
  assert.equal(log[1]?.ev, 'dismiss');
  assert.equal(log[1]?.key, key);

  const scanned = await json<ScanResult>(home, ['scan'], deps);
  assert.deepEqual(scanned.data.signals, []);
  assert.equal(scanned.data.dismissedCount, 1);

  const loops = await json<LoopsResult>(home, ['loops'], deps);
  const item = loops.data.groups.flatMap((group) => group.items).find((i) => i.id === promoted.data.id);
  assert.ok(item, 'the promoted item should appear in loops');
  assert.deepEqual(item.tags, ['promoted']);
  assert.deepEqual(
    loops.data.groups.map((group) => group.project),
    [repo],
  );
});

test('promote seeds a PR signal from its title', async () => {
  const root = tempDir();
  const home = setup(root);
  const deps: Deps = {
    git: fakeGit({}),
    gh: fakeGh([pr('acme/web', 123, { title: 'Fix the chart legend' })], []),
  };

  const promoted = await json<PromoteResult>(home, ['promote', 'review:acme/web#123'], deps);
  assert.equal(promoted.code, 0);
  assert.equal(promoted.data.title, 'Fix the chart legend');
  // No local clone was discovered, so the item carries no project.
  assert.equal(promoted.data.project, null);
});

test('promote renders one line naming the new id', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  const home = setup(root);

  const result = await waid(home, ['promote', `dirty:${repo}`, '--no-sync'], {
    git: fakeGit({ [repo]: { branch: 'main', dirty: 14 } }),
    gh: fakeGh([], []),
  });

  assert.equal(result.code, 0, result.err);
  assert.match(result.out, /^promoted [a-z0-9]{4}  14 uncommitted files in alpha\n$/);
});

test('promoting an unknown key exits 1', async () => {
  const root = tempDir();
  const home = setup(root);

  const result = await waid(home, ['promote', 'pr:acme/web#404', '--no-sync'], {
    git: fakeGit({}),
    gh: fakeGh([], []),
  });

  assert.equal(result.code, 1);
  assert.match(result.err, /unknown signal key: pr:acme\/web#404/);
  assert.equal(fs.readFileSync(path.join(home, 'events.jsonl'), 'utf8'), '');
});

test('promoting an already-dismissed key exits 1 and writes nothing', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  const home = setup(root);
  const deps: Deps = { git: fakeGit({ [repo]: { branch: 'main', dirty: 2 } }), gh: fakeGh([], []) };

  await json<DismissResult>(home, ['dismiss', `dirty:${repo}`], deps);
  const result = await waid(home, ['promote', `dirty:${repo}`, '--no-sync'], deps);

  assert.equal(result.code, 1);
  assert.equal(events(home).length, 1);
});

test('promote with no key exits 1', async () => {
  const home = setup(tempDir());
  const result = await waid(home, ['promote', '--no-sync'], { git: fakeGit({}), gh: fakeGh([], []) });
  assert.equal(result.code, 1);
  assert.match(result.err, /promote requires a signal key/);
});
