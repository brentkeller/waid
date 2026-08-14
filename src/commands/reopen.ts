import { UserError } from '../errors.ts';
import { appendEvent, loadState, requireItem } from '../store.ts';
import type { CommandModule, Ctx } from '../types.ts';

/** The reopened item. `reopen` also clears `waitingOn`, so a waiting item comes back plain open. */
export type ReopenResult = {
  id: string;
  title: string;
  status: 'open';
  ts: string;
};

async function run(ctx: Ctx): Promise<ReopenResult> {
  const id = (ctx.args[0] ?? '').trim();
  if (id === '') throw new UserError('reopen requires an item id');

  const item = requireItem(loadState(ctx.cfg), id);
  const event = appendEvent(ctx.cfg, { ev: 'reopen', id: item.id });

  return { id: item.id, title: item.title, status: 'open', ts: event.ts };
}

function render(data: ReopenResult): string {
  return `reopened ${data.id}  ${data.title}`;
}

export const reopen: CommandModule<ReopenResult> = { run, render };
