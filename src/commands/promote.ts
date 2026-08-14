import { detectSignals } from '../detect.ts';
import { UserError } from '../errors.ts';
import { newId } from '../ids.ts';
import { appendEvent, loadState } from '../store.ts';
import type { CommandModule, Ctx } from '../types.ts';

export type PromoteResult = {
  id: string;
  key: string;
  title: string;
  project: string | null;
};

/** The tag every promoted item carries, so a detected loop stays distinguishable from a declared one. */
const PROMOTED_TAG = 'promoted';

/**
 * Turns a detected signal into a declared item, then dismisses the key so the loop is not reported
 * twice. The signal must still be detectable — an already-dismissed or vanished key is rejected
 * rather than guessed at, since the item's title and project can only come from the live signal.
 */
async function run(ctx: Ctx): Promise<PromoteResult> {
  const key = (ctx.args[0] ?? '').trim();
  if (key === '') throw new UserError('promote requires a signal key');

  const state = loadState(ctx.cfg);
  const { signals } = detectSignals(ctx.cfg, {
    sessions: ctx.sessions ?? [],
    state,
    now: ctx.now,
    git: ctx.git,
    gh: ctx.gh,
  });

  const signal = signals.find((candidate) => candidate.key === key);
  if (signal === undefined) throw new UserError(`unknown signal key: ${key}`);

  const taken = new Set(state.items.map((item) => item.id));
  const id = newId((candidate) => taken.has(candidate));

  appendEvent(ctx.cfg, {
    ev: 'add',
    id,
    title: signal.title,
    status: 'open',
    project: signal.project,
    session: null,
    tags: [PROMOTED_TAG],
    waitingOn: null,
  });
  appendEvent(ctx.cfg, { ev: 'dismiss', key });

  return { id, key, title: signal.title, project: signal.project };
}

function render(data: PromoteResult): string {
  return `promoted ${data.id}  ${data.title}`;
}

export const promote: CommandModule<PromoteResult> = { run, render, needsSessions: true };
