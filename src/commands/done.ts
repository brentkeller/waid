import { UserError } from '../errors.ts';
import { appendEvent, loadState, requireItem } from '../store.ts';
import type { CommandModule, Ctx } from '../types.ts';

/** The closed item, echoed back so `--json` callers need no follow-up read. */
export type DoneResult = {
  id: string;
  title: string;
  status: 'done';
  ts: string;
};

async function run(ctx: Ctx): Promise<DoneResult> {
  const id = (ctx.args[0] ?? '').trim();
  if (id === '') throw new UserError('done requires an item id');

  const item = requireItem(loadState(ctx.cfg), id);
  const event = appendEvent(ctx.cfg, { ev: 'close', id: item.id });

  return { id: item.id, title: item.title, status: 'done', ts: event.ts };
}

function render(data: DoneResult): string {
  return `closed ${data.id}  ${data.title}`;
}

export const done: CommandModule<DoneResult> = { run, render };
