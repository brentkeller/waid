import { ensureHome, loadConfig, resolveHome } from './config.ts';
import type { CommandModule } from './types.ts';

export type Io = {
  out: (text: string) => void;
  err: (text: string) => void;
};

/** Dispatch table; each command module registers itself here. */
export const COMMANDS: Record<string, CommandModule<unknown>> = {};

/** Entry point: returns the process exit code. */
export async function run(argv: string[], io: Io = defaultIo): Promise<number> {
  const home = resolveHome();
  ensureHome(home);
  const cfg = loadConfig(home);
  io.out(`waid — home: ${cfg.home}\n`);
  void argv;
  return 0;
}

const defaultIo: Io = {
  out: (text) => process.stdout.write(text),
  err: (text) => process.stderr.write(text),
};
