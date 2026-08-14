import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { loadConfig } from '../src/config.ts';
import { GH_CACHE_VERSION, fetchGh } from '../src/gh.ts';
import type { Config, GhClient, GhPr } from '../src/types.ts';

const NOW = new Date('2026-08-14T12:00:00.000Z');
const MINUTE_MS = 60 * 1000;

function config(overrides: Partial<Config> = {}): Config {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'waid-gh-'));
  fs.mkdirSync(path.join(home, 'cache'), { recursive: true });
  return { ...loadConfig(home), ghUser: 'octocat', ghCacheTtlMinutes: 15, ...overrides };
}

function pr(number: number, overrides: Partial<GhPr> = {}): GhPr {
  return {
    number,
    repository: 'octo/widgets',
    title: `pull request ${number}`,
    author: 'octocat',
    isDraft: false,
    state: 'open',
    createdAt: '2026-08-10T09:00:00Z',
    url: `https://github.com/octo/widgets/pull/${number}`,
    ...overrides,
  };
}

/** A client that records how often each query ran, so cache hits are observable. */
function countingClient(
  reviewRequested: GhPr[],
  authored: GhPr[],
): { client: GhClient; calls: { reviewRequested: number; authored: number } } {
  const calls = { reviewRequested: 0, authored: 0 };
  return {
    calls,
    client: {
      reviewRequested: () => {
        calls.reviewRequested += 1;
        return reviewRequested;
      },
      authored: () => {
        calls.authored += 1;
        return authored;
      },
    },
  };
}

function throwingClient(message: string): GhClient {
  return {
    reviewRequested: () => {
      throw new Error(message);
    },
    authored: () => {
      throw new Error(message);
    },
  };
}

function writeCache(cfg: Config, minutesOld: number, reviewRequested: GhPr[], authored: GhPr[]): void {
  const cache = {
    version: GH_CACHE_VERSION,
    cachedAt: new Date(NOW.getTime() - minutesOld * MINUTE_MS).toISOString(),
    reviewRequested,
    authored,
  };
  fs.writeFileSync(cfg.ghCachePath, `${JSON.stringify(cache, null, 2)}\n`);
}

test('a cache younger than the TTL is returned without calling gh', () => {
  const cfg = config();
  writeCache(cfg, 5, [pr(1)], [pr(2)]);
  const { client, calls } = countingClient([pr(9)], [pr(9)]);

  const result = fetchGh(cfg, { client, now: NOW });

  assert.equal(result.available, true);
  assert.equal(calls.reviewRequested, 0);
  assert.equal(calls.authored, 0);
  assert.deepEqual(
    result.reviewRequested.map((item) => item.number),
    [1],
  );
  assert.deepEqual(
    result.authored.map((item) => item.number),
    [2],
  );
  assert.equal(result.cachedAt, new Date(NOW.getTime() - 5 * MINUTE_MS).toISOString());
});

test('an expired cache triggers exactly one call per query and rewrites the cache', () => {
  const cfg = config();
  writeCache(cfg, 60, [pr(1)], [pr(2)]);
  const { client, calls } = countingClient([pr(11)], [pr(12), pr(13)]);

  const result = fetchGh(cfg, { client, now: NOW });

  assert.equal(result.available, true);
  assert.equal(calls.reviewRequested, 1);
  assert.equal(calls.authored, 1);
  assert.equal(result.cachedAt, NOW.toISOString());

  const written = JSON.parse(fs.readFileSync(cfg.ghCachePath, 'utf8')) as Record<string, unknown>;
  assert.equal(written['version'], GH_CACHE_VERSION);
  assert.equal(written['cachedAt'], NOW.toISOString());
  assert.deepEqual(written['reviewRequested'], [pr(11)]);
  assert.deepEqual(written['authored'], [pr(12), pr(13)]);
});

test('a missing cache fetches and writes one', () => {
  const cfg = config();
  const { client, calls } = countingClient([pr(1)], []);

  const result = fetchGh(cfg, { client, now: NOW });

  assert.equal(result.available, true);
  assert.equal(calls.reviewRequested, 1);
  assert.equal(fs.existsSync(cfg.ghCachePath), true);
});

test('a throwing client reports unavailable with a reason and leaves the cache intact', () => {
  const cfg = config();
  writeCache(cfg, 60, [pr(1)], [pr(2)]);
  const before = fs.readFileSync(cfg.ghCachePath, 'utf8');

  const result = fetchGh(cfg, { client: throwingClient('gh: not logged in'), now: NOW });

  assert.equal(result.available, false);
  assert.match(String(result.reason), /not logged in/);
  assert.deepEqual(result.reviewRequested, []);
  assert.deepEqual(result.authored, []);
  assert.equal(fs.readFileSync(cfg.ghCachePath, 'utf8'), before);
});

test('a corrupt gh cache is a miss, not an error', () => {
  const cfg = config();
  fs.writeFileSync(cfg.ghCachePath, '{not json at all');
  const { client, calls } = countingClient([pr(4)], []);

  const result = fetchGh(cfg, { client, now: NOW });

  assert.equal(result.available, true);
  assert.equal(calls.reviewRequested, 1);
  assert.deepEqual(
    result.reviewRequested.map((item) => item.number),
    [4],
  );
});

test('a cache written by another version is a miss', () => {
  const cfg = config();
  fs.writeFileSync(
    cfg.ghCachePath,
    JSON.stringify({ version: GH_CACHE_VERSION + 1, cachedAt: NOW.toISOString(), reviewRequested: [pr(1)], authored: [] }),
  );
  const { client, calls } = countingClient([pr(5)], []);

  const result = fetchGh(cfg, { client, now: NOW });

  assert.equal(calls.reviewRequested, 1);
  assert.deepEqual(
    result.reviewRequested.map((item) => item.number),
    [5],
  );
});

test('ghUser null short-circuits to unavailable without calling gh', () => {
  const cfg = config({ ghUser: null });
  const { client, calls } = countingClient([pr(1)], [pr(2)]);

  const result = fetchGh(cfg, { client, now: NOW });

  assert.equal(result.available, false);
  assert.match(String(result.reason), /ghUser/);
  assert.equal(calls.reviewRequested, 0);
  assert.equal(calls.authored, 0);
  assert.equal(result.cachedAt, null);
  assert.equal(fs.existsSync(cfg.ghCachePath), false);
});

test('cached entries that are not pull requests are dropped rather than trusted', () => {
  const cfg = config();
  fs.writeFileSync(
    cfg.ghCachePath,
    JSON.stringify({
      version: GH_CACHE_VERSION,
      cachedAt: NOW.toISOString(),
      reviewRequested: [pr(1), { number: 'seven' }, null],
      authored: 'not an array',
    }),
  );
  const { client } = countingClient([], []);

  const result = fetchGh(cfg, { client, now: NOW });

  assert.deepEqual(
    result.reviewRequested.map((item) => item.number),
    [1],
  );
  assert.deepEqual(result.authored, []);
});
