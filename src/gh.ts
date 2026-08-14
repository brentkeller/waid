import { execFileSync } from 'node:child_process';
import fs from 'node:fs';

import type { Config, GhClient, GhPr } from './types.ts';

/** Upper bound on any single `gh` invocation, so a hanging network call cannot stall a command. */
export const GH_TIMEOUT_MS = 10_000;

/** Cap on each search, well above a plausible number of open PRs for one person. */
export const GH_SEARCH_LIMIT = 100;

/** Bumped whenever a cache written by an older waid can no longer be reused. */
export const GH_CACHE_VERSION = 1;

const GH_FIELDS = 'number,repository,title,author,isDraft,state,createdAt,url';

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

/** The production `GhClient`, backed by `gh search prs`. Throws when `gh` cannot answer. */
export function realGhClient(): GhClient {
  return {
    reviewRequested: () => search('--review-requested=@me'),
    authored: () => search('--author=@me'),
  };
}

/**
 * Runs one account-wide search. Account-wide rather than per-repo on purpose: detection costs two
 * calls regardless of how many repos are active.
 */
function search(filter: string): GhPr[] {
  const out = execFileSync(
    'gh',
    ['search', 'prs', filter, '--state=open', '--limit', String(GH_SEARCH_LIMIT), '--json', GH_FIELDS],
    {
      encoding: 'utf8',
      timeout: GH_TIMEOUT_MS,
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true,
    },
  );
  return narrowPrs(JSON.parse(out));
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
  };
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

/** The first line of a failure, short enough to sit on one dim note line. */
function describe(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  const firstLine = message.split('\n')[0]?.trim() ?? '';
  const text = firstLine || 'gh failed';
  return text.length > 200 ? `${text.slice(0, 199)}…` : text;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
