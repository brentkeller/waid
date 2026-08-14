import { UserError } from './errors.ts';
import type { Flags } from './types.ts';

/** Flags that never take a value, so they cannot swallow the argument that follows them. */
const BOOLEAN_FLAGS = new Set(['json', 'no-sync', 'full', 'last', 'all', 'help', 'version']);

/** Short forms, expanded to their long name before anything else looks at them. */
const ALIASES: Record<string, string> = { p: 'project', t: 'tag', h: 'help' };

/** Flags that collect every occurrence into an array instead of overwriting. */
const REPEATABLE = new Set(['tag']);

export type ParsedArgv = {
  /** The first positional, or null when argv carries no command. */
  command: string | null;
  /** Positionals after the command, in order. */
  args: string[];
  flags: Flags;
};

/**
 * Splits argv into a command, its positionals and its flags. Unrecognised long flags are treated
 * as value flags, so new commands can take options without touching this table.
 */
export function parseArgv(argv: string[]): ParsedArgv {
  const flags: Flags = {};
  const positionals: string[] = [];
  let endOfFlags = false;

  for (let i = 0; i < argv.length; i++) {
    const token = argv[i] ?? '';

    if (endOfFlags || !isFlag(token)) {
      positionals.push(token);
      continue;
    }
    if (token === '--') {
      endOfFlags = true;
      continue;
    }

    const body = token.startsWith('--') ? token.slice(2) : token.slice(1);
    const eq = body.indexOf('=');
    const name = expand(eq === -1 ? body : body.slice(0, eq));
    const inline = eq === -1 ? undefined : body.slice(eq + 1);

    if (BOOLEAN_FLAGS.has(name)) {
      flags[name] = inline === undefined ? true : inline !== 'false';
      continue;
    }

    let value = inline;
    if (value === undefined) {
      const next = argv[i + 1];
      if (next === undefined || isFlag(next)) {
        throw new UserError(`flag --${name} requires a value`);
      }
      value = next;
      i++;
    }

    if (REPEATABLE.has(name)) {
      const existing = flags[name];
      flags[name] = Array.isArray(existing) ? [...existing, value] : [value];
    } else {
      flags[name] = value;
    }
  }

  const [command = null, ...args] = positionals;
  return { command, args, flags };
}

/** Reads a flag that carries a single value, ignoring the boolean and repeated forms. */
export function stringFlag(flags: Flags, name: string): string | undefined {
  const value = flags[name];
  return typeof value === 'string' ? value : undefined;
}

/** Reads a repeatable flag as a list, tolerating the single-value form. */
export function listFlag(flags: Flags, name: string): string[] {
  const value = flags[name];
  if (Array.isArray(value)) return value;
  return typeof value === 'string' ? [value] : [];
}

function expand(name: string): string {
  return ALIASES[name] ?? name;
}

/** A lone `-` is a conventional stdin placeholder, not a flag. */
function isFlag(token: string): boolean {
  return token.startsWith('-') && token !== '-';
}
