import fs from 'node:fs';
import path from 'node:path';

import { realGitClient } from './git.ts';
import type { CachedSession, Config, GitClient } from './types.ts';

/** Directory names never walked: build output, vendored trees, and git's own internals. */
const SKIP_DIRS = new Set(['node_modules', 'bin', 'obj', '.git']);

const MS_PER_DAY = 24 * 60 * 60 * 1000;

/**
 * Walks each `scanRoots` entry to `scanMaxDepth` looking for a `.git` entry, in sorted order. A
 * directory that is a repo is reported and not descended into, so a vendored repo inside a repo is
 * never double-reported. Unreadable or missing roots are skipped rather than failing the scan.
 */
export function discoverRepos(cfg: Pick<Config, 'scanRoots' | 'scanMaxDepth'>): string[] {
  const found: string[] = [];
  const seen = new Set<string>();

  const visit = (dir: string, depth: number): void => {
    if (isRepoRoot(dir)) {
      const key = compareKey(dir);
      if (!seen.has(key)) {
        seen.add(key);
        found.push(dir);
      }
      return;
    }
    if (depth >= cfg.scanMaxDepth) return;
    for (const name of childDirectories(dir)) {
      if (SKIP_DIRS.has(name.toLowerCase())) continue;
      visit(path.join(dir, name), depth + 1);
    }
  };

  for (const root of cfg.scanRoots) visit(root, 0);
  return found;
}

/** Whether `dir` holds a `.git` entry — a directory in a normal clone, a file in a worktree. */
function isRepoRoot(dir: string): boolean {
  return fs.existsSync(path.join(dir, '.git'));
}

/** Sorted subdirectory names of `dir`, or nothing when it is missing or unreadable. */
function childDirectories(dir: string): string[] {
  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return [];
  }
  return entries
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
}

/**
 * The discovered repos worth scanning: those a harvested session ran inside within
 * `activeWindowDays`, or whose HEAD commit falls in the same window. Order follows discovery.
 */
export function activeRepos(
  cfg: Pick<Config, 'scanRoots' | 'scanMaxDepth' | 'activeWindowDays'>,
  sessions: readonly CachedSession[],
  deps: { now: Date; git?: GitClient; repos?: string[] },
): string[] {
  const repos = deps.repos ?? discoverRepos(cfg);
  if (repos.length === 0) return [];

  const git = deps.git ?? realGitClient();
  const cutoff = deps.now.getTime() - cfg.activeWindowDays * MS_PER_DAY;

  const recentlyUsed = new Set<string>();
  for (const session of sessions) {
    if (session.project === null) continue;
    const at = sessionTime(session);
    if (at === null || at < cutoff) continue;
    const repo = repoForPath(session.project, repos);
    if (repo !== null) recentlyUsed.add(repo);
  }

  return repos.filter((repo) => {
    if (recentlyUsed.has(repo)) return true;
    const head = git.headCommitDate(repo);
    return head !== null && head.getTime() >= cutoff;
  });
}

/** When a session last showed activity, preferring its end; null when neither timestamp parses. */
function sessionTime(session: CachedSession): number | null {
  const stamp = session.ended ?? session.started;
  if (stamp === null) return null;
  const time = new Date(stamp).getTime();
  return Number.isNaN(time) ? null : time;
}

/**
 * The repo `p` lies inside, preferring the longest match so a repo vendored inside another wins.
 * Matching is separator- and (on Windows) case-insensitive, and only at a path boundary.
 */
export function repoForPath(p: string, repos: readonly string[]): string | null {
  if (p.trim() === '') return null;
  const target = compareKey(p);

  let best: string | null = null;
  let bestLength = -1;
  for (const repo of repos) {
    const key = compareKey(repo);
    if (key === '') continue;
    if (target !== key && !target.startsWith(`${key}/`)) continue;
    if (key.length > bestLength) {
      best = repo;
      bestLength = key.length;
    }
  }
  return best;
}

/** A path reduced to a comparable form: forward separators, no trailing separator, case-folded. */
function compareKey(p: string): string {
  const unified = p.trim().replace(/\\/g, '/').replace(/\/+$/, '');
  return process.platform === 'win32' ? unified.toLowerCase() : unified;
}
