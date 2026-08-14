import path from 'node:path';

import { relTime } from './format.ts';
import { fetchGh } from './gh.ts';
import { realGitClient } from './git.ts';
import { activeRepos, discoverRepos } from './repos.ts';
import type {
  CachedSession,
  Config,
  GhClient,
  GhPr,
  GitClient,
  Signal,
  SignalKind,
  State,
} from './types.ts';

/** Rank order of the kinds: work blocking someone else outranks my own uncommitted scratch. */
const KIND_RANK: Record<SignalKind, number> = { review: 0, pr: 1, ahead: 2, dirty: 3 };

export type DetectDeps = {
  sessions: readonly CachedSession[];
  /** The folded log, read for its `dismissed` keys. */
  state: State;
  now: Date;
  git?: GitClient;
  gh?: GhClient;
};

export type DetectResult = {
  signals: Signal[];
  /** How many signals were suppressed because their key had been dismissed. */
  dismissedCount: number;
  /** Non-fatal degradations, today only an unavailable `gh`. */
  notes: string[];
};

/** A signal plus the instant it is timed by, which ranks it before the timestamp is dropped. */
type Candidate = Signal & { at: number };

/**
 * Assembles every detected signal, ranks them, and removes the ones already dismissed. Nothing here
 * throws: an unavailable `gh` becomes a note and the git signals still return, so detection works
 * offline. GitHub is queried account-wide (two calls) while git is queried per *active* repo only.
 */
export function detectSignals(cfg: Config, deps: DetectDeps): DetectResult {
  const git = deps.git ?? realGitClient();
  const gh = fetchGh(cfg, { client: deps.gh, now: deps.now });

  const repos = discoverRepos(cfg);
  const active = activeRepos(cfg, deps.sessions, { now: deps.now, git, repos });

  const candidates = [
    ...gh.reviewRequested.map((pr) => reviewSignal(pr, repos, deps.now)),
    ...gh.authored.map((pr) => prSignal(pr, repos, deps.now)),
    ...active.flatMap((repo) => repoSignals(repo, git, deps.now)),
  ].sort(byRank);

  const dismissed = new Set(deps.state.dismissed);
  const signals: Signal[] = [];
  let dismissedCount = 0;
  for (const { at: _at, ...signal } of candidates) {
    if (dismissed.has(signal.key)) dismissedCount += 1;
    else signals.push(signal);
  }

  const notes = gh.available ? [] : [`GitHub signals unavailable: ${gh.reason ?? 'gh failed'}`];
  return { signals, dismissedCount, notes };
}

/** Kind first, then oldest first — an ancient review request is the most urgent thing on the list. */
function byRank(a: Candidate, b: Candidate): number {
  return KIND_RANK[a.kind] - KIND_RANK[b.kind] || a.at - b.at;
}

function reviewSignal(pr: GhPr, repos: readonly string[], now: Date): Candidate {
  const age = relTime(pr.createdAt, now);
  const detail = ['Awaiting your review', pr.author === '' ? '' : `@${pr.author}`, age]
    .filter((part) => part !== '')
    .join(' · ');

  return {
    key: `review:${pr.repository}#${pr.number}`,
    kind: 'review',
    title: pr.title,
    detail,
    project: repoForGhRepository(pr.repository, repos),
    age,
    at: instant(pr.createdAt, now),
  };
}

function prSignal(pr: GhPr, repos: readonly string[], now: Date): Candidate {
  const age = relTime(pr.createdAt, now);
  // `gh search prs` exposes no review decision, so draft state is the most a search can say.
  const state = pr.isDraft ? 'draft' : pr.state.toLowerCase() || 'open';

  return {
    key: `pr:${pr.repository}#${pr.number}`,
    kind: 'pr',
    title: pr.title,
    detail: `Yours, ${state} · ${age}`,
    project: repoForGhRepository(pr.repository, repos),
    age,
    at: instant(pr.createdAt, now),
  };
}

/** The `ahead` and `dirty` signals for one active repo, in rank order. */
function repoSignals(repo: string, git: GitClient, now: Date): Candidate[] {
  const branch = git.currentBranch(repo);
  const head = git.headCommitDate(repo);
  const age = head === null ? '?' : relTime(head.toISOString(), now);
  const at = head?.getTime() ?? now.getTime();
  const name = path.basename(repo);

  const signals: Candidate[] = [];

  // A null count covers both "no upstream" and "not a repo"; neither is something to push.
  const ahead = git.aheadCount(repo);
  if (branch !== null && ahead !== null && ahead > 0) {
    signals.push({
      key: `ahead:${repo}:${branch}`,
      kind: 'ahead',
      title: `${plural(ahead, 'unpushed commit')} on ${branch} in ${name}`,
      detail: `${plural(ahead, 'commit')} ahead on ${branch}`,
      project: repo,
      age,
      at,
    });
  }

  const dirty = git.dirtyFileCount(repo);
  if (dirty > 0) {
    const where = branch === null ? '' : ` on ${branch}`;
    signals.push({
      key: `dirty:${repo}`,
      kind: 'dirty',
      title: `${plural(dirty, 'uncommitted file')} in ${name}`,
      detail: `${plural(dirty, 'uncommitted file')}${where}`,
      project: repo,
      age,
      at,
    });
  }

  return signals;
}

/**
 * The local checkout a GitHub `owner/repo` refers to, matched on directory name. Only an unambiguous
 * match counts — two clones of the same name give no answer rather than an arbitrary one.
 */
function repoForGhRepository(repository: string, repos: readonly string[]): string | null {
  const name = repository.split('/').at(-1)?.toLowerCase() ?? '';
  if (name === '') return null;

  const matches = repos.filter((repo) => path.basename(repo).toLowerCase() === name);
  return matches.length === 1 ? (matches[0] ?? null) : null;
}

/** A timestamp for ranking; an unparseable one sorts as "now", i.e. last within its kind. */
function instant(iso: string, now: Date): number {
  const at = Date.parse(iso);
  return Number.isNaN(at) ? now.getTime() : at;
}

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`;
}
