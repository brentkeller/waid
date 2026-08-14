import { parseArgv } from './args.ts';
import { add } from './commands/add.ts';
import { doctor } from './commands/doctor.ts';
import { done } from './commands/done.ts';
import { list } from './commands/list.ts';
import { loops } from './commands/loops.ts';
import { note } from './commands/note.ts';
import { reopen } from './commands/reopen.ts';
import { show } from './commands/show.ts';
import { ensureHome, loadConfig, resolveHome } from './config.ts';
import { UserError } from './errors.ts';
import type { CommandModule, Ctx } from './types.ts';

export type Io = {
  out: (text: string) => void;
  err: (text: string) => void;
};

/** Dispatch table; each command module registers itself here. */
export const COMMANDS: Record<string, CommandModule<unknown>> = {
  add,
  done,
  reopen,
  note,
  list,
  loops,
  show,
  doctor,
};

/** Entry point: returns the process exit code. */
export async function run(argv: string[], io: Io = defaultIo): Promise<number> {
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

    const ctx: Ctx = { cfg, flags, args, cwd: process.cwd(), now: new Date() };
    const data = await mod.run(ctx);

    if (json) {
      io.out(`${JSON.stringify(data, null, 2)}\n`);
    } else {
      const text = mod.render(data, ctx);
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
  waid loops [-p <project>]           Declared open/waiting items, then detected signals
  waid scan [-p <project>]            Detected signals only
  waid list [--status s] [--project p] [--tag t] [--all]
  waid add "<title>" [-p <project>] [--waiting-on <who>] [--tag <t>] [--session <id>]
  waid done <id>                      waid reopen <id>
  waid note <id> "<text>"
  waid show <id>                      Full item with notes and history
  waid promote <key>                  waid dismiss <key>
  waid doctor                         Validate config, log integrity, gh auth, cache freshness

Global flags:
  --json                              Print a single JSON document to stdout
  --no-sync                           Skip the implicit session sync
  --waid-home <path>                  Override $WAID_HOME
  -h, --help                          Show this help
`;

const defaultIo: Io = {
  out: (text) => process.stdout.write(text),
  err: (text) => process.stderr.write(text),
};
