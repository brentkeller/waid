import { execFileSync } from 'node:child_process';

import type { GitClient } from './types.ts';

/** Upper bound on any single `git` invocation, so an unreachable path cannot stall a command. */
export const GIT_TIMEOUT_MS = 5000;

/**
 * Runs `git -C repo …` and returns trimmed stdout, or null when git exits non-zero, times out, is
 * missing, or the directory does not exist. Callers never see an exception.
 */
function gitOutput(repo: string, args: string[]): string | null {
  try {
    const out = execFileSync('git', ['-C', repo, ...args], {
      encoding: 'utf8',
      timeout: GIT_TIMEOUT_MS,
      stdio: ['ignore', 'pipe', 'ignore'],
      windowsHide: true,
    });
    return out.trim();
  } catch {
    return null;
  }
}

/** The production `GitClient`, backed by `git` subprocesses. */
export function realGitClient(): GitClient {
  return {
    headCommitDate(repo) {
      const out = gitOutput(repo, ['log', '-1', '--format=%cI']);
      if (!out) return null;
      const date = new Date(out);
      return Number.isNaN(date.getTime()) ? null : date;
    },

    currentBranch(repo) {
      // `symbolic-ref` fails on a detached HEAD, where `rev-parse --abbrev-ref` would say "HEAD".
      return gitOutput(repo, ['symbolic-ref', '--quiet', '--short', 'HEAD']) || null;
    },

    aheadCount(repo) {
      const out = gitOutput(repo, ['rev-list', '--count', '@{u}..HEAD']);
      if (out === null) return null;
      const count = Number.parseInt(out, 10);
      return Number.isNaN(count) ? null : count;
    },

    dirtyFileCount(repo) {
      const out = gitOutput(repo, ['status', '--porcelain']);
      if (!out) return 0;
      return out.split('\n').filter((line) => line.trim() !== '').length;
    },

    isRepo(dir) {
      return gitOutput(dir, ['rev-parse', '--is-inside-work-tree']) === 'true';
    },
  };
}
