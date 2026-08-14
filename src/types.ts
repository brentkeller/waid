/** Settings persisted in `$WAID_HOME/config.json`, plus paths derived from the home directory. */
export type Config = {
  /** Directories walked when looking for git repos. */
  scanRoots: string[];
  /** Root of the Claude Code data directory holding the per-project transcript files. */
  claudeDir: string;
  /** Recency gate, in days, for repo activity and detection. */
  activeWindowDays: number;
  /** GitHub login used for "PRs I authored" / "awaiting my review"; null disables gh signals. */
  ghUser: string | null;
  /** Lifetime of `cache/gh.json` before gh is queried again. */
  ghCacheTtlMinutes: number;
  /** Repo discovery depth under each scan root. */
  scanMaxDepth: number;

  home: string;
  configPath: string;
  eventsPath: string;
  cacheDir: string;
  sessionsCachePath: string;
  ghCachePath: string;
};

/** Config keys that live in `config.json`; the rest of `Config` is derived from the home path. */
export type ConfigDefaults = Omit<
  Config,
  'home' | 'configPath' | 'eventsPath' | 'cacheDir' | 'sessionsCachePath' | 'ghCachePath'
>;

/** Every status an item can hold, in display order. */
export const STATUSES = ['open', 'waiting', 'done'] as const;

export type Status = (typeof STATUSES)[number];

/** A timestamped note attached to an item. */
export type Note = {
  ts: string;
  text: string;
};

/** An item as it exists after folding the event log; never stored in this shape. */
export type Item = {
  id: string;
  title: string;
  status: Status;
  /** Free text, only meaningful while `status` is `waiting`. */
  waitingOn: string | null;
  /** Absolute path of the project the item belongs to. */
  project: string | null;
  /** Claude Code session the item was captured from. */
  session: string | null;
  tags: string[];
  notes: Note[];
  /** `ts` of the `add` event. */
  created: string;
  /** `ts` of the most recent event touching the item. */
  updated: string;
};

/**
 * A line of `events.jsonl` as waid writes it. Lines read back from disk are untrusted and must be
 * narrowed before use — this type describes output, never assumptions about input.
 */
export type WaidEvent =
  | {
      ts: string;
      ev: 'add';
      id: string;
      title: string;
      status?: Status;
      project?: string | null;
      session?: string | null;
      tags?: string[];
      waitingOn?: string | null;
    }
  | {
      ts: string;
      ev: 'update';
      id: string;
      title?: string;
      status?: Status;
      project?: string | null;
      tags?: string[];
      waitingOn?: string | null;
    }
  | { ts: string; ev: 'note'; id: string; text: string }
  | { ts: string; ev: 'close'; id: string }
  | { ts: string; ev: 'reopen'; id: string }
  | { ts: string; ev: 'dismiss'; key: string }
  | { ts: string; ev: 'undismiss'; key: string };

type WithOptionalTs<T> = T extends unknown ? Omit<T, 'ts'> & { ts?: string } : never;

/** An event as handed to `appendEvent`, which stamps `ts` when the caller omits it. */
export type EventInput = WithOptionalTs<WaidEvent>;

/** Why a log line could not be applied. Reported by `waid doctor`, never fatal. */
export type ProblemReason =
  | 'unparseable'
  | 'not-an-object'
  | 'add-missing-fields'
  | 'duplicate-id'
  | 'unknown-id'
  | 'note-missing-text'
  | 'bad-status'
  | 'missing-key'
  | 'unknown-ev';

/** A log line the fold could not fully apply, located by its 1-based line number. */
export type Problem = {
  line: number;
  reason: ProblemReason;
  id?: string | null;
  ev?: string | null;
};

/** The result of folding the whole event log. */
export type State = {
  items: Item[];
  /** Detected-signal keys hidden by `dismiss`. */
  dismissed: string[];
  problems: Problem[];
};

/** One Claude Code session, distilled from its transcript file. */
export type Session = {
  /** Claude Code session id; the transcript file is named after it. */
  id: string;
  title: string;
  /** Absolute path the session ran in. */
  project: string | null;
  branch: string | null;
  /** ISO timestamp of the first record, including sidechains. */
  started: string | null;
  /** ISO timestamp of the last record, including sidechains. */
  ended: string | null;
  /** User messages that were neither sidechain nor meta. */
  prompts: number;
};

/** A `Session` plus the transcript stat fields that let a sync skip re-reading an unchanged file. */
export type CachedSession = Session & {
  _file: {
    path: string;
    mtimeMs: number;
    size: number;
  };
};

/**
 * The git subprocess surface detection needs, declared as an interface so tests inject a fake with
 * a compiler-checked shape. No method throws: a failed or absent `git` reads as null / zero / false.
 */
export type GitClient = {
  /** Commit date of HEAD; null when the repo has no commits or is not a repo. */
  headCommitDate: (repo: string) => Date | null;
  /** Checked-out branch; null on a detached HEAD or an unborn branch. */
  currentBranch: (repo: string) => string | null;
  /** Commits on HEAD that the upstream lacks; null when the branch has no upstream. */
  aheadCount: (repo: string) => number | null;
  /** Files reported by `git status --porcelain`, tracked or not. */
  dirtyFileCount: (repo: string) => number;
  /** Whether `dir` sits inside a git work tree. */
  isRepo: (dir: string) => boolean;
};

/**
 * A pull request reduced to the fields waid renders. `gh`'s JSON carries far more and is narrowed
 * into this shape rather than trusted, so a schema change upstream cannot reach the renderers.
 */
export type GhPr = {
  number: number;
  /** `owner/repo` — the form the signal key embeds. */
  repository: string;
  title: string;
  /** Login of the PR author. */
  author: string;
  /**
   * `gh search prs` exposes no review decision, so draft state is the closest thing to a review
   * state it can report; a richer state would cost a per-PR API call.
   */
  isDraft: boolean;
  state: string;
  createdAt: string;
  url: string;
};

/**
 * The two account-wide GitHub queries detection needs, declared as an interface so tests inject a
 * fake. Unlike `GitClient`, these methods may throw — `fetchGh` is what turns a failure into an
 * unavailable result.
 */
export type GhClient = {
  /** Open PRs waiting on my review. */
  reviewRequested: () => GhPr[];
  /** Open PRs I authored. */
  authored: () => GhPr[];
};

/** A project and the items belonging to it, as the grouped commands render them. */
export type ProjectGroup = {
  project: string | null;
  items: Item[];
};

/** The four things detection can notice, in rank order: a review blocks someone else first. */
export type SignalKind = 'review' | 'pr' | 'ahead' | 'dirty';

/**
 * A loop found in git or GitHub rather than declared. `key` is the stable identity `dismiss` and
 * `promote` address a signal by — `review:owner/repo#123`, `dirty:<repo>` and so on.
 */
export type Signal = {
  key: string;
  kind: SignalKind;
  /** Human summary, good enough to become an item title when the signal is promoted. */
  title: string;
  /** The line rendered beside the key: state, counts, and the age where the spec shows one. */
  detail: string;
  /** Absolute repo path the signal belongs to; null when no local repo could be matched. */
  project: string | null;
  /** Compact relative age of whatever the signal is timed by — a PR's creation, a repo's HEAD. */
  age: string;
};

/** Declared open items grouped by project, with the signals detected alongside them. */
export type LoopsResult = {
  groups: ProjectGroup[];
  detected: Signal[];
  /** Signals suppressed because their key was dismissed. */
  dismissedCount: number;
  /** Non-fatal degradations — an unavailable `gh`, a failed detection pass — to show beneath. */
  notes: string[];
};

/** Parsed command-line flags. Boolean flags are present only when passed. */
export type Flags = {
  json?: boolean;
  'no-sync'?: boolean;
  full?: boolean;
  last?: boolean;
  all?: boolean;
  help?: boolean;
  version?: boolean;
  project?: string;
  tag?: string[];
  [key: string]: string | string[] | boolean | undefined;
};

/** Everything a command needs to run. */
export type Ctx = {
  cfg: Config;
  flags: Flags;
  args: string[];
  cwd: string;
  now: Date;
  /** Harvested sessions, attached by the CLI for commands declaring `needsSessions`. */
  sessions?: CachedSession[];
  /**
   * Non-fatal degradations noticed before dispatch, such as a failed implicit sync. The CLI renders
   * these itself, so a command only reads them when it wants them inside its own `--json` payload.
   */
  notes: string[];
};

/**
 * A command module. `run` returns plain data so `--json` prints it directly and `render` turns
 * the same data into human output; the two can never diverge.
 */
export type CommandModule<D> = {
  run: (ctx: Ctx) => Promise<D>;
  /** Method syntax on purpose: bivariant parameters let the dispatch table erase `D` to `unknown`. */
  render(data: D, ctx: Ctx): string;
  /** Set when the command needs `ctx.sessions` populated. */
  needsSessions?: boolean;
};
