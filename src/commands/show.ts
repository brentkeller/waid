import { UserError } from '../errors.ts';
import { localYmd, pad, relTime } from '../format.ts';
import { loadState, readEvents, requireItem } from '../store.ts';
import type { CommandModule, Ctx, Item } from '../types.ts';

/** One log line touching the item, located by its 1-based line number. */
export type HistoryEntry = {
  ts: string;
  ev: string;
  line: number;
};

export type ShowResult = {
  item: Item;
  /** Every log line naming the item's id, in file order. */
  history: HistoryEntry[];
};

const LABEL_WIDTH = 9;

async function run(ctx: Ctx): Promise<ShowResult> {
  const id = (ctx.args[0] ?? '').trim();
  if (id === '') throw new UserError('show requires an item id');

  const item = requireItem(loadState(ctx.cfg), id);

  const history: HistoryEntry[] = [];
  for (const { line, ev } of readEvents(ctx.cfg)) {
    if (!isRecord(ev) || ev['id'] !== item.id) continue;
    history.push({ ts: str(ev['ts']), ev: str(ev['ev']), line });
  }

  return { item, history };
}

function render(data: ShowResult, ctx: Ctx): string {
  const { item } = data;
  const status = item.waitingOn === null ? item.status : `${item.status} ← ${item.waitingOn}`;

  const lines = [
    `${item.id}  ${item.title}`,
    '',
    field('status', status),
    field('project', item.project ?? '-'),
    field('tags', item.tags.length > 0 ? item.tags.join(', ') : '-'),
    field('session', item.session ?? '-'),
    field('created', stamp(item.created, ctx.now)),
    field('updated', stamp(item.updated, ctx.now)),
  ];

  if (item.notes.length > 0) {
    lines.push('', '  notes');
    for (const note of item.notes) lines.push(`    ${localYmd(note.ts)}  ${note.text}`);
  }

  lines.push('', '  history');
  for (const entry of data.history) {
    lines.push(`    ${String(entry.line).padStart(4)}  ${localYmd(entry.ts)}  ${entry.ev}`);
  }

  return lines.join('\n');
}

function field(label: string, value: string): string {
  return `  ${pad(label, LABEL_WIDTH)}${value}`;
}

function stamp(ts: string, now: Date): string {
  return `${localYmd(ts)}  (${relTime(ts, now)})`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function str(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

export const show: CommandModule<ShowResult> = { run, render };
