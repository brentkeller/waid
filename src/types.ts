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
  sessions?: unknown[];
};

/**
 * A command module. `run` returns plain data so `--json` prints it directly and `render` turns
 * the same data into human output; the two can never diverge.
 */
export type CommandModule<D> = {
  run: (ctx: Ctx) => Promise<D>;
  render: (data: D, ctx: Ctx) => string;
  /** Set when the command needs `ctx.sessions` populated. */
  needsSessions?: boolean;
};
