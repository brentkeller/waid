import { listFlag, stringFlag } from '../args.ts';
import { UserError } from '../errors.ts';
import { isStatus } from '../events.ts';
import { pad, relTime } from '../format.ts';
import { groupByProject, knownProjects, resolveProject } from '../projects.ts';
import { loadState } from '../store.ts';
import { STATUSES } from '../types.ts';
import type { CommandModule, Ctx, Item, Status } from '../types.ts';

/** The filters that produced a list, echoed back so `--json` callers can see what was applied. */
export type ListFilters = {
  status: Status | null;
  project: string | null;
  tag: string[];
  all: boolean;
};

export type ListResult = {
  items: Item[];
  filters: ListFilters;
};

/** Column widths for the shared item row; titles beyond the width are ellipsized, not wrapped. */
const ID_WIDTH = 4;
const STATUS_WIDTH = 10;
const TITLE_WIDTH = 44;
const META_WIDTH = 10;

async function run(ctx: Ctx): Promise<ListResult> {
  const state = loadState(ctx.cfg);

  const filters: ListFilters = {
    status: readStatus(stringFlag(ctx.flags, 'status')),
    project: resolveProject(
      stringFlag(ctx.flags, 'project'),
      knownProjects(state, ctx.sessions),
      ctx.cwd,
    ),
    tag: listFlag(ctx.flags, 'tag'),
    all: ctx.flags['all'] === true,
  };

  const items = state.items
    .filter((item) => matches(item, filters))
    .sort((a, b) => a.updated.localeCompare(b.updated));

  return { items, filters };
}

/** Filters are conjunctive; an explicit `--status` speaks for itself, so it overrides the done gate. */
function matches(item: Item, filters: ListFilters): boolean {
  if (filters.status !== null) {
    if (item.status !== filters.status) return false;
  } else if (!filters.all && item.status === 'done') {
    return false;
  }
  if (filters.project !== null && item.project !== filters.project) return false;
  return filters.tag.every((tag) => item.tags.includes(tag));
}

function readStatus(value: string | undefined): Status | null {
  if (value === undefined) return null;
  if (!isStatus(value)) {
    throw new UserError(`unknown status: ${value} (expected ${STATUSES.join(', ')})`);
  }
  return value;
}

function render(data: ListResult, ctx: Ctx): string {
  if (data.items.length === 0) return 'no items';

  const lines: string[] = [];
  for (const group of groupByProject(data.items)) {
    if (lines.length > 0) lines.push('');
    lines.push(`  ${group.project ?? '(no project)'}`);
    for (const item of group.items) lines.push(itemLine(item, ctx.now));
  }
  return lines.join('\n');
}

/** One item as an aligned row, indented under its project heading. Shared with `waid loops`. */
export function itemLine(item: Item, now: Date): string {
  const meta = [
    item.tags.length > 0 ? `[${item.tags.join(',')}]` : '',
    item.waitingOn === null ? '' : `← ${item.waitingOn}`,
  ]
    .filter((part) => part !== '')
    .join(' ');

  const row = [
    pad(item.id, ID_WIDTH),
    '  ',
    pad(item.status, STATUS_WIDTH),
    pad(item.title, TITLE_WIDTH),
    '  ',
    pad(meta, META_WIDTH),
    relTime(item.updated, now),
  ].join('');

  return `    ${row}`.trimEnd();
}

export const list: CommandModule<ListResult> = { run, render };
