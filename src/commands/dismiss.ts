import { UserError } from '../errors.ts';
import { appendEvent, loadState } from '../store.ts';
import type { CommandModule, Ctx } from '../types.ts';

export type DismissResult = {
  key: string;
  dismissed: true;
  /** Whether the key was already hidden, in which case nothing was written. */
  already: boolean;
};

/**
 * Hides a detected signal for good. The key is not validated against detection: a signal only has
 * to have existed once to be worth silencing, and detection is expensive enough that requiring a
 * live match would make dismissing an intermittent signal a matter of timing.
 */
async function run(ctx: Ctx): Promise<DismissResult> {
  const key = (ctx.args[0] ?? '').trim();
  if (key === '') throw new UserError('dismiss requires a signal key');

  const state = loadState(ctx.cfg);
  if (state.dismissed.includes(key)) return { key, dismissed: true, already: true };

  appendEvent(ctx.cfg, { ev: 'dismiss', key });
  return { key, dismissed: true, already: false };
}

function render(data: DismissResult): string {
  return data.already ? `already dismissed ${data.key}` : `dismissed ${data.key}`;
}

export const dismiss: CommandModule<DismissResult> = { run, render };
