import { parseArgv } from './args.ts';
import { add } from './commands/add.ts';
import { dismiss } from './commands/dismiss.ts';
import { doctor } from './commands/doctor.ts';
import { done } from './commands/done.ts';
import { list } from './commands/list.ts';
import { loops } from './commands/loops.ts';
import { note } from './commands/note.ts';
import { promote } from './commands/promote.ts';
import { reopen } from './commands/reopen.ts';
import { scan } from './commands/scan.ts';
import { show } from './commands/show.ts';
import { sync } from './commands/sync.ts';
import { today } from './commands/today.ts';
import { week } from './commands/week.ts';
import { ensureHome, loadConfig, resolveHome } from './config.ts';
import { UserError } from './errors.ts';
import { pinnedGhClient } from './gh.ts';
import { cacheAgeMinutes, loadSessions, syncSessions } from './sessions.ts';
import { applyPlan } from './tui/apply.ts';
import { pick } from './tui/run.ts';
import type {
  CachedSession,
  CommandModule,
  Config,
  Ctx,
  GhClient,
  GitClient,
  PickRow,
  Plan,
} from './types.ts';

export type Io = {
  out: (text: string) => void;
  err: (text: string) => void;
};

/** Injectable collaborators. Tests replace these; production runs with the defaults. */
export type Deps = {
  syncSessions?: (cfg: Config, opts: { now: Date }) => { sessions: CachedSession[] };
  /** Detection seams, forwarded onto `ctx` so commands never reach for a module directly. */
  git?: GitClient;
  gh?: GhClient;
  /** Whether both stdio ends are a terminal; the `-i` guard's only view of the outside world. */
  isTty?: () => boolean;
  /** The picker loop, replaced in tests so no suite ever enters raw mode. */
  pick?: (rows: PickRow[]) => Promise<Plan | null>;
};

/** How stale `cache/sessions.json` may be before a read command refreshes it. */
export const SYNC_STALE_MINUTES = 5;

/** Dispatch table; each command module registers itself here. */
export const COMMANDS: Record<string, CommandModule<unknown>> = {
  sync,
  today,
  week,
  add,
  done,
  reopen,
  note,
  list,
  loops,
  scan,
  promote,
  dismiss,
  show,
  doctor,
};

/** Entry point: returns the process exit code. */
export async function run(argv: string[], io: Io = defaultIo, deps: Deps = {}): Promise<number> {
  let json = false;
  try {
    const { command, args, flags } = parseArgv(argv);
    json = flags.json === true;

    const override = typeof flags['waid-home'] === 'string' ? flags['waid-home'] : undefined;
    const home = resolveHome(override);
    ensureHome(home);
    const cfg = loadConfig(home);

    if (command === null || flags.help === true) {
      io.out(USAGE);
      return 0;
    }

    const mod = COMMANDS[command];
    if (mod === undefined) throw new UserError(`unknown command: ${command}`);

    const interactive = flags.interactive === true;
    if (interactive) guardInteractive(command, mod, json, deps);

    const ctx: Ctx = {
      cfg,
      flags,
      args,
      cwd: process.cwd(),
      now: pinnedNow(),
      notes: [],
      git: deps.git,
      gh: deps.gh ?? pinnedGhClient(),
    };
    if (mod.needsSessions === true) attachSessions(ctx, deps);

    const data = await mod.run(ctx);

    if (interactive && mod.rows !== undefined) {
      const rows = mod.rows(data, ctx);
      // Nothing to point a cursor at is not an error: print what the plain command prints.
      if (rows.some(isSelectable)) return await interact(rows, ctx, io, deps);
    }

    if (json) {
      io.out(`${JSON.stringify(data, null, 2)}\n`);
      // Stdout stays a single JSON document, so degradations are reported alongside it.
      for (const note of ctx.notes) io.err(`${note}\n`);
    } else {
      const text = compose(mod.render(data, ctx), ctx.notes);
      if (text) io.out(text.endsWith('\n') ? text : `${text}\n`);
    }
    return 0;
  } catch (error) {
    if (error instanceof UserError) {
      const payload = { error: error.message, candidates: error.candidates };
      io.err(json ? jsonLine(payload) : humanUserError(error));
      return 1;
    }
    const message = error instanceof Error ? error.message : String(error);
    const stack = error instanceof Error ? (error.stack ?? message) : String(error);
    io.err(json ? jsonLine({ error: message, stack }) : `${stack}\n`);
    return 2;
  }
}

/**
 * The three conditions under which `-i` is a user error rather than a silent fallback. A scripted
 * `waid scan -i` that quietly printed and exited 0 would look like it had worked, so each of these
 * exits 1 — and all three are checked before the command runs, so a rejected `-i` does no detection.
 */
function guardInteractive(
  command: string,
  mod: CommandModule<unknown>,
  json: boolean,
  deps: Deps,
): void {
  if (json) throw new UserError('-i cannot be combined with --json');
  if (mod.rows === undefined) throw new UserError(`${command} does not support -i`);
  const isTty = deps.isTty ?? defaultIsTty;
  if (!isTty()) throw new UserError('-i requires an interactive terminal');
}

/**
 * Opens the picker, then applies whatever the user confirmed. Cancelling writes nothing and is
 * still a success: declining to triage is a valid outcome, not a failure.
 *
 * Receipts are printed only once the picker has restored the terminal, so they land in the real
 * scrollback rather than on the alt screen that is about to be discarded.
 */
async function interact(rows: PickRow[], ctx: Ctx, io: Io, deps: Deps): Promise<number> {
  const plan = await (deps.pick ?? pick)(rows);
  if (plan === null) return 0;

  const lines = [...applyPlan(ctx.cfg, plan), ...ctx.notes];
  if (lines.length > 0) io.out(`${lines.join('\n')}\n`);
  return 0;
}

/** A row the cursor can land on; a screen of nothing but headings never opens the picker. */
function isSelectable(row: PickRow): boolean {
  return row.kind !== 'heading';
}

/**
 * The clock the command runs against: the instant pinned through `WAID_NOW`, otherwise now. The
 * seam the differential harness pins so a spawned process is reproducible.
 */
function pinnedNow(): Date {
  const pinned = process.env.WAID_NOW;
  return pinned === undefined ? new Date() : new Date(pinned);
}

function defaultIsTty(): boolean {
  return process.stdin.isTTY === true && process.stdout.isTTY === true;
}

/**
 * Fills `ctx.sessions` for a command that asked for them, refreshing the cache first when it is
 * missing or stale. A sync that fails is not worth failing the command over: the cached sessions
 * are still useful, so the command runs on them and the reason is noted.
 */
function attachSessions(ctx: Ctx, deps: Deps): void {
  if (ctx.flags['no-sync'] !== true) {
    const age = cacheAgeMinutes(ctx.cfg, ctx.now);
    if (age === null || age >= SYNC_STALE_MINUTES) {
      const sync = deps.syncSessions ?? syncSessions;
      try {
        ctx.sessions = sync(ctx.cfg, { now: ctx.now }).sessions;
        return;
      } catch (error) {
        const reason = error instanceof Error ? error.message : String(error);
        ctx.notes.push(`session sync failed (${reason}); using the cached sessions`);
      }
    }
  }

  ctx.sessions = loadSessions(ctx.cfg).sessions;
}

/** Joins rendered output and CLI-level notes into one block, dropping the empty parts. */
function compose(rendered: string, notes: string[]): string {
  return [rendered.replace(/\n+$/, ''), ...notes].filter((part) => part !== '').join('\n');
}

function jsonLine(payload: unknown): string {
  return `${JSON.stringify(payload)}\n`;
}

function humanUserError(error: UserError): string {
  const lines = [error.message];
  for (const candidate of error.candidates ?? []) lines.push(`  ${candidate}`);
  return `${lines.join('\n')}\n`;
}

const USAGE = `waid — what am I doing

Usage: waid <command> [options]

  waid sync [--full]                  Rebuild the derived session cache
  waid today [--date YYYY-MM-DD]      Sessions + item activity for a day
  waid week [--last]                  Rollup by project for this week (or last)
  waid loops [-p <project>] [-i]      Declared open/waiting items, then detected signals
  waid scan [-p <project>] [-i]       Detected signals only
  waid list [--status s] [--project p] [--tag t] [--all]
  waid add "<title>" [-p <project>] [--waiting-on <who>] [--tag <t>] [--session <id>]
  waid done <id>                      waid reopen <id>
  waid note <id> "<text>"
  waid show <id>                      Full item with notes and history
  waid promote <key>                  waid dismiss <key>
  waid doctor                         Validate config, log integrity, gh auth, cache freshness

Global flags:
  -i, --interactive                   Mark rows and apply in one keystroke (loops, scan)
  --json                              Print a single JSON document to stdout
  --no-sync                           Skip the implicit session sync
  --waid-home <path>                  Override $WAID_HOME
  -h, --help                          Show this help
`;

const defaultIo: Io = {
  out: (text) => process.stdout.write(text),
  err: (text) => process.stderr.write(text),
};
