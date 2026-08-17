import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import type { Deps } from '../src/cli.ts';
import type { GhClient, GhPr, GitClient, Signal } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

type ScanResult = {
  signals: Signal[];
  dismissedCount: number;
  notes: string[];
};

const DAY_MS = 24 * 60 * 60 * 1000;

function tempDir(prefix = 'waid-scan-'): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

/** Creates `<root>/<name>` and marks it a repo by giving it a `.git` directory. */
function makeRepo(root: string, name: string): string {
  const dir = path.join(root, name);
  fs.mkdirSync(path.join(dir, '.git'), { recursive: true });
  return dir;
}

/**
 * A home configured to scan `root` with GitHub signals on. `config.json` is written before the CLI
 * runs, since `ensureHome` never overwrites an existing one.
 */
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

/** Runs `waid scan` with detection fakes; `--no-sync` keeps the real `~/.claude` out of the test. */
async function scan(
  home: string,
  argv: string[],
  deps: Deps,
): Promise<{ code: number; out: string; err: string; data: ScanResult }> {
  const result = await waid(home, ['scan', ...argv, '--no-sync', '--json'], deps);
  return { ...result, data: result.json() as ScanResult };
}

test('scan --json returns every signal in rank order', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  const home = setup(root);

  const { code, data } = await scan(home, [], {
    git: fakeGit({ [repo]: { branch: 'topic', ahead: 2, dirty: 5 } }),
    gh: fakeGh([pr('acme/web', 123)], [pr('acme/web', 456)]),
  });

  assert.equal(code, 0);
  assert.deepEqual(
    data.signals.map((signal) => signal.key),
    ['review:acme/web#123', 'pr:acme/web#456', `ahead:${repo}:topic`, `dirty:${repo}`],
  );
  assert.equal(data.dismissedCount, 0);
  assert.deepEqual(data.notes, []);
});

test('scan -p narrows to the resolved project', async () => {
  const root = tempDir();
  const alpha = makeRepo(root, 'alpha');
  const beta = makeRepo(root, 'beta');
  const home = setup(root);

  const deps: Deps = {
    git: fakeGit({
      [alpha]: { branch: 'main', ahead: 1, dirty: 1 },
      [beta]: { branch: 'main', ahead: 1, dirty: 1 },
    }),
    gh: fakeGh([], []),
  };

  const absolute = await scan(home, ['-p', alpha], deps);
  assert.deepEqual(
    absolute.data.signals.map((signal) => signal.key),
    [`ahead:${alpha}:main`, `dirty:${alpha}`],
  );

  // A partial resolves because the detected repo paths join the known projects.
  const partial = await scan(home, ['-p', 'beta'], deps);
  assert.deepEqual(
    partial.data.signals.map((signal) => signal.key),
    [`ahead:${beta}:main`, `dirty:${beta}`],
  );
});

test('scan -p drops PR signals with no local checkout', async () => {
  const root = tempDir();
  const alpha = makeRepo(root, 'alpha');
  const home = setup(root);

  const { data } = await scan(home, ['-p', alpha], {
    git: fakeGit({ [alpha]: { branch: 'main', ahead: 0, dirty: 2 } }),
    gh: fakeGh([pr('acme/nowhere', 7)], []),
  });

  assert.deepEqual(
    data.signals.map((signal) => signal.key),
    [`dirty:${alpha}`],
  );
});

test('scan on a clean machine says so and exits 0', async () => {
  const root = tempDir();
  makeRepo(root, 'clean');
  const home = setup(root);

  const deps: Deps = { git: fakeGit({}), gh: fakeGh([], []) };
  const { code, data } = await scan(home, [], deps);
  assert.equal(code, 0);
  assert.deepEqual(data.signals, []);

  const human = await waid(home, ['scan', '--no-sync'], deps);
  assert.equal(human.code, 0);
  assert.match(human.out, /nothing detected/);
});

test('scan renders the DETECTED header, three aligned columns and the footer', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'inl-prt');
  const home = setup(root);
  seedLog(home, [{ ts: daysAgo(1).toISOString(), ev: 'dismiss', key: 'pr:acme/web#456' }]);

  const human = await waid(home, ['scan', '--no-sync'], {
    git: fakeGit({ [repo]: { branch: 'inl-prt-fixes', ahead: null, dirty: 14 } }),
    gh: fakeGh(
      [pr('acme/web', 123, { title: 'Fix the chart legend', branch: 'chart-legend' })],
      [pr('acme/web', 456)],
    ),
  });

  assert.equal(human.code, 0, human.err);
  const lines = human.out.split('\n');
  assert.equal(lines[0], 'DETECTED  (2 shown, 1 dismissed)');
  assert.equal(lines[1], '');
  assert.match(
    lines[2] ?? '',
    /^ {2}review:acme\/web#123 +Fix the chart legend +@tmoore · chart-legend · 3d$/,
  );
  assert.match(
    lines[3] ?? '',
    // The HEAD date is relative to when the fake was built, so the age is a shape, not a value.
    new RegExp(`^ {2}dirty:${escapeRe(repo)} +14 uncommitted files +inl-prt-fixes · \\d+[hd]$`),
  );
  assert.match(human.out, /waid promote <key>/);
  assert.match(human.out, /waid dismiss <key>/);
});

test('scan aligns the subject column across signals whose keys differ in length', async () => {
  const root = tempDir();
  const home = setup(root);

  const human = await waid(home, ['scan', '--no-sync'], {
    git: fakeGit({}),
    gh: fakeGh([], [pr('acme/web', 1, { title: 'Short key' }), pr('acme/a-much-longer-repo', 22222, { title: 'Long key' })]),
  });

  assert.equal(human.code, 0, human.err);
  const [first, second] = human.out.split('\n').slice(2, 4);
  assert.equal(first?.indexOf('Short key'), second?.indexOf('Long key'));
});

test('scan truncates a subject too wide for its column rather than shifting the meta', async () => {
  const root = tempDir();
  const home = setup(root);
  const long = 'Migrate the BudgetBreakdown chart from ASP.NET to AngularJS and D3 in one pass';

  const human = await waid(home, ['scan', '--no-sync'], {
    git: fakeGit({}),
    gh: fakeGh([], [pr('acme/web', 1, { title: long }), pr('acme/web', 2, { title: 'Short' })]),
  });

  const [first, second] = human.out.split('\n').slice(2, 4);
  assert.ok(!first?.includes(long), 'an over-wide subject must be truncated');
  assert.match(first ?? '', /…/);
  assert.equal(first?.indexOf('pr-1-branch'), second?.indexOf('pr-2-branch'));
});

test('scan notes an unavailable gh once and still reports git signals', async () => {
  const root = tempDir();
  const repo = makeRepo(root, 'alpha');
  const home = setup(root);

  const deps: Deps = { git: fakeGit({ [repo]: { branch: 'main', dirty: 3 } }), gh: NO_GH };
  const { data } = await scan(home, [], deps);
  assert.deepEqual(
    data.signals.map((signal) => signal.key),
    [`dirty:${repo}`],
  );
  assert.equal(data.notes.length, 1);
  assert.match(data.notes[0] ?? '', /GitHub signals unavailable/);

  const human = await waid(home, ['scan', '--no-sync'], deps);
  const noteLines = human.out.split('\n').filter((line) => line.includes('GitHub signals unavailable'));
  assert.equal(noteLines.length, 1);
  assert.match(noteLines[0] ?? '', /^ {2}\S/);
});

function escapeRe(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
