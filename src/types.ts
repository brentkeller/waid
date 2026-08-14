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
