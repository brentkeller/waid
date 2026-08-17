import { execFileSync } from 'node:child_process';
import fs from 'node:fs';

import type { Config, GhClient, GhPr } from './types.ts';

/** Upper bound on any single `gh` invocation, so a hanging network call cannot stall a command. */
export const GH_TIMEOUT_MS = 10_000;

/** Cap on each search, well above a plausible number of open PRs for one person. */
export const GH_SEARCH_LIMIT = 100;

/** Bumped whenever a cache written by an older waid can no longer be reused. */
export const GH_CACHE_VERSION = 2;

/**
 * The search runs through GraphQL rather than `gh search prs` for one reason: `--json` on the search
 * command cannot report a head branch, and GraphQL can, at the same cost of one request per query.
 */
const GH_QUERY = `query($q: String!, $limit: Int!) {
  search(query: $q, type: ISSUE, first: $limit) {
    nodes {
      ... on PullRequest {
        number
        title
        headRefName
        isDraft
        state
        createdAt
        url
        repository { nameWithOwner }
        author { login }
      }
    }
  }
}`;

const MINUTE_MS = 60 * 1000;

/** `cache/gh.json` as written. Disposable: deleting it costs two API calls, nothing more. */
export type GhCache = {
  version: typeof GH_CACHE_VERSION;
  cachedAt: string;
  reviewRequested: GhPr[];
  authored: GhPr[];
};

/** What detection sees: the two PR lists, or an explanation of why GitHub could not answer. */
export type GhResult = {
  available: boolean;
  /** Set only when `available` is false; suitable for a single dim note beneath the signals. */
  reason?: string;
  reviewRequested: GhPr[];
  authored: GhPr[];
  /** When the returned lists were fetched; null when nothing was fetched. */
  cachedAt: string | null;
};

/** Names a file of recorded `gh` responses served in place of a live query. */
export const GH_FIXTURE_ENV = 'WAID_GH_FIXTURE';

/**
 * The client the run should query: one served from the recorded responses `GH_FIXTURE_ENV` names
 * when it is set, otherwise undefined, which leaves detection to reach for the real `gh`.
 */
export function pinnedGhClient(): GhClient | undefined {
  const path = (process.env[GH_FIXTURE_ENV] ?? '').trim();
  return path === '' ? undefined : fixtureGhClient(path);
}

/**
 * A `GhClient` served from a recorded response file rather than the network, so detection can be
 * diffed against fixed data. The file holds a recorded `gh api graphql` response per query:
 *
 *     {"reviewRequested": {"data": …}, "authored": {"data": …}}
 *
 * An `error` key instead records a `gh` that could not answer, so the degraded path is reachable.
 */
export function fixtureGhClient(file: string): GhClient {
  return {
    reviewRequested: () => fixtureQuery(file, 'reviewRequested'),
    authored: () => fixtureQuery(file, 'authored'),
  };
}

function fixtureQuery(file: string, name: 'reviewRequested' | 'authored'): GhPr[] {
  const document: unknown = JSON.parse(fs.readFileSync(file, 'utf8'));
  if (!isRecord(document)) throw new Error(`the gh fixture at ${file} is not a JSON object`);

  const recorded = document['error'];
  if (typeof recorded === 'string') throw Object.assign(new Error(recorded), { stderr: recorded });
  if (!(name in document)) throw new Error(`the gh fixture at ${file} records no ${name} response`);

  return narrowPrs(searchNodes(document[name]));
}

/** The production `GhClient`, backed by `gh api graphql`. Throws when `gh` cannot answer. */
export function realGhClient(): GhClient {
  return {
    reviewRequested: () => search('is:pr is:open review-requested:@me'),
    authored: () => search('is:pr is:open author:@me'),
  };
}

/**
 * Runs one account-wide search. Account-wide rather than per-repo on purpose: detection costs two
 * calls regardless of how many repos are active.
 */
function search(query: string): GhPr[] {
  const out = execFileSync(
    'gh',
    [
      'api',
      'graphql',
      '-f',
      // Sent on one line so a failure message, which echoes the command, stays one line too.
      `query=${GH_QUERY.replace(/\s+/g, ' ')}`,
      '-f',
      `q=${query}`,
      '-F',
      `limit=${GH_SEARCH_LIMIT}`,
    ],
    {
      encoding: 'utf8',
      timeout: GH_TIMEOUT_MS,
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true,
    },
  );
  return narrowPrs(searchNodes(JSON.parse(out)));
}

/** The `data.search.nodes` array, or nothing when the response is not shaped as expected. */
function searchNodes(value: unknown): unknown {
  if (!isRecord(value)) return [];
  const data = value['data'];
  if (!isRecord(data)) return [];
  const search = data['search'];
  return isRecord(search) ? search['nodes'] : [];
}

/**
 * The two PR lists, served from `cache/gh.json` while it is younger than `ghCacheTtlMinutes` and
 * refetched otherwise. A missing, unauthenticated, timed-out or erroring `gh` yields
 * `{available: false, reason}` and leaves any existing cache untouched; nothing here throws.
 */
export function fetchGh(cfg: Config, deps: { client?: GhClient; now: Date }): GhResult {
  if (cfg.ghUser === null) {
    return unavailable('ghUser is not set in config.json, so GitHub signals are off');
  }

  const cached = readCache(cfg);
  if (cached !== null && ageMinutes(cached.cachedAt, deps.now) < cfg.ghCacheTtlMinutes) {
    return {
      available: true,
      reviewRequested: cached.reviewRequested,
      authored: cached.authored,
      cachedAt: cached.cachedAt,
    };
  }

  const client = deps.client ?? realGhClient();
  let reviewRequested: GhPr[];
  let authored: GhPr[];
  try {
    reviewRequested = client.reviewRequested();
    authored = client.authored();
  } catch (error) {
    return unavailable(describe(error));
  }

  const cache: GhCache = {
    version: GH_CACHE_VERSION,
    cachedAt: deps.now.toISOString(),
    reviewRequested,
    authored,
  };
  writeCache(cfg, cache);

  return { available: true, reviewRequested, authored, cachedAt: cache.cachedAt };
}

/** Minutes since the cache was written, or null when there is no readable cache. */
export function ghCacheAgeMinutes(cfg: Config, now: Date): number | null {
  const cached = readCache(cfg);
  if (cached === null) return null;
  const age = ageMinutes(cached.cachedAt, now);
  return Number.isFinite(age) ? Math.floor(age) : null;
}

/** Minutes since `cachedAt`, or `Infinity` when it is unparseable, which forces a refetch. */
function ageMinutes(cachedAt: string, now: Date): number {
  const at = Date.parse(cachedAt);
  if (Number.isNaN(at)) return Number.POSITIVE_INFINITY;
  return Math.max(0, (now.getTime() - at) / MINUTE_MS);
}

function unavailable(reason: string): GhResult {
  return { available: false, reason, reviewRequested: [], authored: [], cachedAt: null };
}

/** A missing, corrupt or foreign-version cache reads as absent — a miss, never an error. */
function readCache(cfg: Config): GhCache | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(fs.readFileSync(cfg.ghCachePath, 'utf8'));
  } catch {
    return null;
  }

  if (!isRecord(parsed) || parsed['version'] !== GH_CACHE_VERSION) return null;
  const cachedAt = parsed['cachedAt'];
  if (typeof cachedAt !== 'string') return null;

  return {
    version: GH_CACHE_VERSION,
    cachedAt,
    reviewRequested: narrowPrs(parsed['reviewRequested']),
    authored: narrowPrs(parsed['authored']),
  };
}

function writeCache(cfg: Config, cache: GhCache): void {
  try {
    fs.mkdirSync(cfg.cacheDir, { recursive: true });
    fs.writeFileSync(cfg.ghCachePath, `${JSON.stringify(cache, null, 2)}\n`);
  } catch {
    // An unwritable cache costs freshness, not correctness: the fetched lists still return.
  }
}

/** Narrows either `gh`'s output or our own cache; anything that is not a PR is dropped. */
function narrowPrs(value: unknown): GhPr[] {
  if (!Array.isArray(value)) return [];
  return value.map(narrowPr).filter((item): item is GhPr => item !== null);
}

function narrowPr(value: unknown): GhPr | null {
  if (!isRecord(value)) return null;

  const number = value['number'];
  const title = value['title'];
  if (typeof number !== 'number' || !Number.isFinite(number) || typeof title !== 'string') return null;

  const repository = repositoryName(value['repository']);
  if (repository === null) return null;

  return {
    number,
    repository,
    title,
    author: authorLogin(value['author']),
    isDraft: value['isDraft'] === true,
    state: typeof value['state'] === 'string' ? value['state'] : 'open',
    createdAt: typeof value['createdAt'] === 'string' ? value['createdAt'] : '',
    url: typeof value['url'] === 'string' ? value['url'] : '',
    branch: branchName(value),
  };
}

/** GraphQL names the head branch `headRefName`; our own cache stores it flat as `branch`. */
function branchName(value: Record<string, unknown>): string {
  const head = value['headRefName'];
  if (typeof head === 'string') return head;
  return typeof value['branch'] === 'string' ? value['branch'] : '';
}

/** `gh` nests the repo as `{name, nameWithOwner}`; our own cache stores the flat `owner/repo`. */
function repositoryName(value: unknown): string | null {
  if (typeof value === 'string') return value || null;
  if (!isRecord(value)) return null;
  const withOwner = value['nameWithOwner'];
  return typeof withOwner === 'string' && withOwner !== '' ? withOwner : null;
}

function authorLogin(value: unknown): string {
  if (typeof value === 'string') return value;
  if (isRecord(value) && typeof value['login'] === 'string') return value['login'];
  return '';
}

/**
 * The first line of a failure, short enough to sit on one dim note line. `gh`'s own stderr is
 * preferred over the message, which only echoes the command that failed.
 */
function describe(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  const stderr = isRecord(error) ? String(error['stderr'] ?? '') : '';
  const text = firstLine(stderr) || firstLine(message) || 'gh failed';
  return text.length > 200 ? `${text.slice(0, 199)}…` : text;
}

function firstLine(text: string): string {
  return text.split('\n').find((line) => line.trim() !== '')?.trim() ?? '';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
