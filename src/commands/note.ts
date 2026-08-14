import { UserError } from '../errors.ts';
import { appendEvent, loadState, requireItem } from '../store.ts';
import type { CommandModule, Ctx } from '../types.ts';

/** The appended note, echoed back with the item it landed on. */
export type NoteResult = {
  id: string;
  title: string;
  text: string;
  ts: string;
};

async function run(ctx: Ctx): Promise<NoteResult> {
  const [rawId, ...rest] = ctx.args;
  const id = (rawId ?? '').trim();
  if (id === '') throw new UserError('note requires an item id');

  const text = rest.join(' ').trim();
  if (text === '') throw new UserError('note requires text');

  const item = requireItem(loadState(ctx.cfg), id);
  const event = appendEvent(ctx.cfg, { ev: 'note', id: item.id, text });

  return { id: item.id, title: item.title, text, ts: event.ts };
}

function render(data: NoteResult): string {
  return `noted ${data.id}  ${data.text}`;
}

export const note: CommandModule<NoteResult> = { run, render };
