import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { loadConfig } from '../src/config.ts';
import { detectSignals } from '../src/detect.ts';
import type { CachedSession, Config, GhClient, GhPr, GitClient, State } from '../src/types.ts';

const DAY_MS = 24 * 60 * 60 * 1000;
const NOW = new Date('2026-08-14T12:00:00.000Z');

function tempDir(prefix = 'waid-detect-'): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

/** Creates `<root>/<name>` and marks it a repo by giving it a `.git` directory. */
function makeRepo(root: string, name: string): string {
  const dir = path.join(root, name);
  fs.mkdirSync(path.join(dir, '.git'), { recursive: true });
  return dir;
}

/** A config whose home is throwaway, so the gh cache each test writes cannot leak into another. */
function config(root: string, overrides: Partial<Config> = {}): Config {
  return {
    ...loadConfig(tempDir('waid-detect-home-')),
    scanRoots: [root],
    scanMaxDepth: 3,
    activeWindowDays: 30,
    ghUser: 'me',
    ...overrides,
  };
}

function daysAgo(days: number): Date {
  return new Date(NOW.getTime() - days * DAY_MS);
}

type RepoState = {
  head?: Date;
  branch?: string | null;
  ahead?: number | null;
  dirty?: number;
};

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

const NO_GH: GhClient = {
  reviewRequested: () => {
    throw new Error('gh: command not found');
  },
  authored: () => {
    throw new Error('gh: command not found');
  },
};

function state(dismissed: string[] = []): State {
  return { items: [], dismissed, problems: [] };
}

const NO_SESSIONS: CachedSession[] = [];

function keys(signals: { key: string }[]): string[] {
  return signals.map((signal) => signal.key);
}

test('detectSignals produces all four kinds with the documented key formats', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({ [repo]: { branch: 'feature', ahead: 2, dirty: 5 } }),
    gh: fakeGh([pr('acme/web', 123)], [pr('acme/web', 456)]),
  });

  assert.deepEqual(keys(result.signals), [
    'review:acme/web#123',
    'pr:acme/web#456',
    `ahead:${repo}:feature`,
    `dirty:${repo}`,
  ]);
  assert.deepEqual(result.notes, []);
  assert.equal(result.dismissedCount, 0);

  const kinds = result.signals.map((signal) => signal.kind);
  assert.deepEqual(kinds, ['review', 'pr', 'ahead', 'dirty']);
});

test('detectSignals fills title, subject, detail and project for each kind', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'web');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({ [repo]: { branch: 'inl-prt-fixes', ahead: 3, dirty: 14 } }),
    gh: fakeGh(
      [pr('acme/web', 123, { title: 'Fix the chart legend', branch: 'chart-legend' })],
      [],
    ),
  });

  const [review, ahead, dirty] = result.signals;

  assert.equal(review?.title, 'Fix the chart legend');
  assert.equal(review?.subject, 'Fix the chart legend');
  assert.equal(review?.detail, '@tmoore · chart-legend · 3d');
  assert.equal(review?.branch, 'chart-legend');
  // The PR's repo name matches exactly one local repo, so `-p` can narrow to it.
  assert.equal(review?.project, repo);
  assert.equal(review?.age, '3d');

  assert.equal(ahead?.project, repo);
  assert.equal(ahead?.subject, '3 commits ahead');
  assert.equal(ahead?.detail, 'inl-prt-fixes · 1d');
  assert.equal(ahead?.branch, 'inl-prt-fixes');

  assert.equal(dirty?.project, repo);
  assert.equal(dirty?.subject, '14 uncommitted files');
  assert.equal(dirty?.detail, 'inl-prt-fixes · 1d');
  assert.equal(dirty?.branch, 'inl-prt-fixes');
  assert.match(dirty?.title ?? '', /web/);
});

test('detectSignals reports the branch, draft and open state on authored PRs', () => {
  const root = tempDir();

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({}),
    gh: fakeGh([], [pr('acme/web', 1, { isDraft: true }), pr('acme/web', 2)]),
  });

  assert.equal(result.signals[0]?.detail, 'pr-1-branch · draft · 3d');
  assert.equal(result.signals[1]?.detail, 'pr-2-branch · open · 3d');
});

test('detectSignals omits a branch the search could not report', () => {
  const root = tempDir();

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({}),
    gh: fakeGh([pr('acme/web', 1, { branch: '' })], [pr('acme/web', 2, { branch: '' })]),
  });

  assert.equal(result.signals[0]?.detail, '@tmoore · 3d');
  assert.equal(result.signals[0]?.branch, null);
  assert.equal(result.signals[1]?.detail, 'open · 3d');
  assert.equal(result.signals[1]?.branch, null);
});

test('detectSignals leaves the branch off a detached HEAD', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'detached');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({ [repo]: { branch: null, dirty: 2 } }),
    gh: fakeGh([], []),
  });

  assert.equal(result.signals[0]?.subject, '2 uncommitted files');
  assert.equal(result.signals[0]?.detail, '1d');
  assert.equal(result.signals[0]?.branch, null);
});

test('detectSignals ranks by kind, breaking ties by age descending', () => {
  const root = tempDir();
  const fresh = makeRepo(root, 'fresh');
  const stale = makeRepo(root, 'stale');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({
      [fresh]: { head: daysAgo(1), branch: 'main', ahead: 1, dirty: 2 },
      [stale]: { head: daysAgo(9), branch: 'main', ahead: 1, dirty: 2 },
    }),
    gh: fakeGh(
      [
        pr('acme/web', 10, { createdAt: daysAgo(1).toISOString() }),
        pr('acme/web', 11, { createdAt: daysAgo(8).toISOString() }),
      ],
      [pr('acme/web', 12)],
    ),
  });

  assert.deepEqual(keys(result.signals), [
    'review:acme/web#11',
    'review:acme/web#10',
    'pr:acme/web#12',
    `ahead:${stale}:main`,
    `ahead:${fresh}:main`,
    `dirty:${stale}`,
    `dirty:${fresh}`,
  ]);
});

test('detectSignals filters dismissed keys and counts them', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(['review:acme/web#123', `dirty:${repo}`]),
    now: NOW,
    git: fakeGit({ [repo]: { branch: 'main', ahead: 4, dirty: 1 } }),
    gh: fakeGh([pr('acme/web', 123)], []),
  });

  assert.deepEqual(keys(result.signals), [`ahead:${repo}:main`]);
  assert.equal(result.dismissedCount, 2);
});

test('detectSignals scopes a dismissal to the exact PR it names', () => {
  const root = tempDir();

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(['pr:o/r#123']),
    now: NOW,
    git: fakeGit({}),
    gh: fakeGh([], [pr('o/r', 123), pr('o/r', 124)]),
  });

  assert.deepEqual(keys(result.signals), ['pr:o/r#124']);
  assert.equal(result.dismissedCount, 1);
});

test('detectSignals returns git signals plus a note when gh is unavailable', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({ [repo]: { branch: 'main', ahead: 2, dirty: 3 } }),
    gh: NO_GH,
  });

  assert.deepEqual(keys(result.signals), [`ahead:${repo}:main`, `dirty:${repo}`]);
  assert.equal(result.notes.length, 1);
  assert.match(result.notes[0] ?? '', /gh: command not found/);
});

test('detectSignals produces no ahead signal without an upstream or a branch', () => {
  const root = tempDir();
  const noUpstream = makeRepo(root, 'no-upstream');
  const detached = makeRepo(root, 'detached');
  const levelWith = makeRepo(root, 'level');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({
      [noUpstream]: { ahead: null, dirty: 1 },
      [detached]: { branch: null, ahead: 3, dirty: 1 },
      [levelWith]: { ahead: 0, dirty: 1 },
    }),
    gh: fakeGh([], []),
  });

  assert.deepEqual(keys(result.signals), [
    `dirty:${detached}`,
    `dirty:${levelWith}`,
    `dirty:${noUpstream}`,
  ]);
});

test('detectSignals produces no dirty signal for a clean repo', () => {
  const root = tempDir();
  const clean = makeRepo(root, 'clean');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({ [clean]: { branch: 'main', ahead: null, dirty: 0 } }),
    gh: fakeGh([], []),
  });

  assert.deepEqual(result.signals, []);
});

test('detectSignals reports a repo that is both dirty and ahead as two signals', () => {
  const root = tempDir();
  const repo = makeRepo(root, 'busy');

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({ [repo]: { branch: 'topic', ahead: 7, dirty: 2 } }),
    gh: fakeGh([], []),
  });

  assert.deepEqual(keys(result.signals), [`ahead:${repo}:topic`, `dirty:${repo}`]);
  assert.equal(new Set(keys(result.signals)).size, 2);
});

test('detectSignals ignores repos outside the activity window', () => {
  const root = tempDir();
  const stale = makeRepo(root, 'stale');

  const result = detectSignals(config(root, { activeWindowDays: 30 }), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({ [stale]: { head: daysAgo(90), branch: 'main', ahead: 5, dirty: 5 } }),
    gh: fakeGh([], []),
  });

  assert.deepEqual(result.signals, []);
});

test('detectSignals leaves a PR project null when the repo name is not local or is ambiguous', () => {
  const root = tempDir();
  makeRepo(root, path.join('one', 'web'));
  makeRepo(root, path.join('two', 'web'));

  const result = detectSignals(config(root), {
    sessions: NO_SESSIONS,
    state: state(),
    now: NOW,
    git: fakeGit({}),
    gh: fakeGh([pr('acme/web', 1), pr('acme/nowhere', 2)], []),
  });

  assert.deepEqual(
    result.signals.map((signal) => signal.project),
    [null, null],
  );
});
