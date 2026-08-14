import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { loadConfig } from '../src/config.ts';
import { activeRepos, discoverRepos, repoForPath } from '../src/repos.ts';
import type { CachedSession, Config, GitClient } from '../src/types.ts';

const DAY_MS = 24 * 60 * 60 * 1000;
const NOW = new Date('2026-08-14T12:00:00.000Z');

function tempDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'waid-repos-'));
}

/** Creates `<root>/<relative>` and marks it a repo by giving it a `.git` directory. */
function makeRepo(root: string, relative: string): string {
  const dir = path.join(root, relative);
  fs.mkdirSync(path.join(dir, '.git'), { recursive: true });
  return dir;
}

function config(overrides: Partial<Config>): Config {
  return { ...loadConfig(tempDir()), ...overrides };
}

/** A `GitClient` whose only meaningful answer is the HEAD date the activity gate asks for. */
function fakeGit(heads: Map<string, Date | null> = new Map()): GitClient {
  return {
    headCommitDate: (repo) => heads.get(repo) ?? null,
    currentBranch: () => 'main',
    aheadCount: () => null,
    dirtyFileCount: () => 0,
    isRepo: () => true,
  };
}

function session(project: string, endedDaysAgo: number): CachedSession {
  const ended = new Date(NOW.getTime() - endedDaysAgo * DAY_MS).toISOString();
  return {
    id: `s-${project}`,
    title: 'a session',
    project,
    branch: 'main',
    started: ended,
    ended,
    prompts: 3,
    _file: { path: `${project}.jsonl`, mtimeMs: 0, size: 0 },
  };
}

function daysAgo(days: number): Date {
  return new Date(NOW.getTime() - days * DAY_MS);
}

test('discoverRepos finds repos at depth 1 and at scanMaxDepth', () => {
  const root = tempDir();
  const shallow = makeRepo(root, 'alpha');
  const deep = makeRepo(root, path.join('a', 'b', 'gamma'));

  const found = discoverRepos(config({ scanRoots: [root], scanMaxDepth: 3 }));
  assert.deepEqual(found.sort(), [shallow, deep].sort());
});

test('discoverRepos ignores a repo deeper than scanMaxDepth', () => {
  const root = tempDir();
  makeRepo(root, path.join('a', 'b', 'c', 'too-deep'));

  assert.deepEqual(discoverRepos(config({ scanRoots: [root], scanMaxDepth: 3 })), []);
});

test('discoverRepos treats a scan root that is itself a repo as the only result', () => {
  const root = tempDir();
  fs.mkdirSync(path.join(root, '.git'), { recursive: true });
  makeRepo(root, 'child');

  assert.deepEqual(discoverRepos(config({ scanRoots: [root], scanMaxDepth: 4 })), [root]);
});

test('discoverRepos skips node_modules, bin and obj', () => {
  const root = tempDir();
  const real = makeRepo(root, 'alpha');
  makeRepo(root, path.join('node_modules', 'vendored'));
  makeRepo(root, path.join('bin', 'built'));
  makeRepo(root, path.join('obj', 'generated'));

  assert.deepEqual(discoverRepos(config({ scanRoots: [root], scanMaxDepth: 4 })), [real]);
});

test('discoverRepos does not descend into .git internals', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  fs.mkdirSync(path.join(repo, '.git', 'modules', 'sub', '.git'), { recursive: true });

  assert.deepEqual(discoverRepos(config({ scanRoots: [root], scanMaxDepth: 4 })), [repo]);
});

test('discoverRepos does not report a repo nested inside another repo', () => {
  const root = tempDir();
  const outer = makeRepo(root, 'alpha');
  makeRepo(root, path.join('alpha', 'vendor', 'inner'));

  assert.deepEqual(discoverRepos(config({ scanRoots: [root], scanMaxDepth: 4 })), [outer]);
});

test('discoverRepos skips a missing scan root without error', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  const missing = path.join(root, 'not', 'there');

  const found = discoverRepos(config({ scanRoots: [missing, root], scanMaxDepth: 4 }));
  assert.deepEqual(found, [repo]);
});

test('discoverRepos reports a repo found under two scan roots once', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');

  const found = discoverRepos(config({ scanRoots: [root, root], scanMaxDepth: 4 }));
  assert.deepEqual(found, [repo]);
});

test('activeRepos admits a repo with a recent session and one with a recent HEAD', () => {
  const root = tempDir();
  const bySession = makeRepo(root, 'session-repo');
  const byHead = makeRepo(root, 'head-repo');
  const stale = makeRepo(root, 'stale-repo');

  const cfg = config({ scanRoots: [root], scanMaxDepth: 4, activeWindowDays: 30 });
  const heads = new Map([
    [bySession, daysAgo(200)],
    [byHead, daysAgo(2)],
    [stale, daysAgo(200)],
  ]);
  const sessions = [session(path.join(bySession, 'src'), 1), session(path.join(stale, 'src'), 90)];

  const active = activeRepos(cfg, sessions, { now: NOW, git: fakeGit(heads) });
  assert.deepEqual(active.sort(), [bySession, byHead].sort());
});

test('activeRepos rejects a repo with neither a recent session nor a recent HEAD', () => {
  const root = tempDir();
  const stale = makeRepo(root, 'stale-repo');

  const cfg = config({ scanRoots: [root], scanMaxDepth: 4, activeWindowDays: 30 });
  const heads = new Map([[stale, daysAgo(31)]]);
  const sessions = [session(path.join(stale, 'src'), 31)];

  assert.deepEqual(activeRepos(cfg, sessions, { now: NOW, git: fakeGit(heads) }), []);
});

test('activeRepos tolerates a repo with no commits and a session with no timestamps', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'empty-repo');

  const cfg = config({ scanRoots: [root], scanMaxDepth: 4, activeWindowDays: 30 });
  const undated = { ...session(path.join(repo, 'src'), 0), started: null, ended: null };

  assert.deepEqual(activeRepos(cfg, [undated], { now: NOW, git: fakeGit() }), []);
});

test('activeRepos ignores sessions outside every discovered repo', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');

  const cfg = config({ scanRoots: [root], scanMaxDepth: 4, activeWindowDays: 30 });
  const sessions = [session(path.join(root, 'elsewhere'), 1)];

  assert.deepEqual(activeRepos(cfg, sessions, { now: NOW, git: fakeGit() }), []);
  assert.deepEqual(
    activeRepos(cfg, sessions, { now: NOW, git: fakeGit(new Map([[repo, daysAgo(1)]])) }),
    [repo],
  );
});

test('repoForPath prefers the longest matching repo prefix', () => {
  const repos = [
    path.join('C:', 'dev', 'outer'),
    path.join('C:', 'dev', 'outer', 'inner'),
    path.join('C:', 'dev', 'other'),
  ];

  assert.equal(repoForPath(path.join('C:', 'dev', 'outer', 'src'), repos), repos[0]);
  assert.equal(repoForPath(path.join('C:', 'dev', 'outer', 'inner', 'src'), repos), repos[1]);
  assert.equal(repoForPath(path.join('C:', 'dev', 'outer'), repos), repos[0]);
});

test('repoForPath returns null when nothing matches', () => {
  const repos = [path.join('C:', 'dev', 'outer')];

  assert.equal(repoForPath(path.join('C:', 'dev', 'elsewhere'), repos), null);
  // A sibling whose name merely starts with the repo name is not inside it.
  assert.equal(repoForPath(path.join('C:', 'dev', 'outerly'), repos), null);
  assert.equal(repoForPath('', repos), null);
  assert.equal(repoForPath(path.join('C:', 'dev', 'outer'), []), null);
});

test('repoForPath tolerates trailing separators and mixed separators', () => {
  const repos = ['C:\\dev\\outer'];

  assert.equal(repoForPath('C:\\dev\\outer\\', repos), repos[0]);
  assert.equal(repoForPath('C:/dev/outer/src', repos), repos[0]);
});
