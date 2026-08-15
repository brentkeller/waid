import { detectSignals } from '../detect.ts';
import { UserError } from '../errors.ts';
import { newId } from '../ids.ts';
import { appendEvent, loadState } from '../store.ts';
import type { CommandModule, Config, Ctx, Signal, State } from '../types.ts';

export type PromoteResult = {
  id: string;
  key: string;
  title: string;
  project: string | null;
};

/** The tag every promoted item carries, so a detected loop stays distinguishable from a declared one. */
const PROMOTED_TAG = 'promoted';

/**
 * Writes the two events a promotion is made of: an `add` carrying the signal's project and the
 * `promoted` tag, then a `dismiss` of the signal's key so the loop is not reported twice. `title`
 * is passed separately so a caller that let the user edit it can use the edited text; `state` only
 * supplies the ids already taken.
 */
export function promoteSignal(
  cfg: Config,
  state: State,
  signal: Signal,
  title: string,
): PromoteResult {
  const taken = new Set(state.items.map((item) => item.id));
  const id = newId((candidate) => taken.has(candidate));

  appendEvent(cfg, {
    ev: 'add',
    id,
    title,
    status: 'open',
    project: signal.project,
    session: null,
    tags: [PROMOTED_TAG],
    waitingOn: null,
  });
  appendEvent(cfg, { ev: 'dismiss', key: signal.key });

  return { id, key: signal.key, title, project: signal.project };
}

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

  return promoteSignal(ctx.cfg, state, signal, signal.title);
}

function render(data: PromoteResult): string {
  return `promoted ${data.id}  ${data.title}`;
}

export const promote: CommandModule<PromoteResult> = { run, render, needsSessions: true };
