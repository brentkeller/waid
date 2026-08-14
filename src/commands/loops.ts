import { stringFlag } from '../args.ts';
import { groupByProject, knownProjects, resolveProject } from '../projects.ts';
import { loadState } from '../store.ts';
import type { CommandModule, Ctx, Item, LoopsResult } from '../types.ts';
import { itemLine } from './list.ts';

async function run(ctx: Ctx): Promise<LoopsResult> {
  const state = loadState(ctx.cfg);
  const project = resolveProject(
    stringFlag(ctx.flags, 'project'),
    knownProjects(state, ctx.sessions),
    ctx.cwd,
  );

  const items = state.items
    .filter((item) => isOpen(item) && (project === null || item.project === project))
    .sort((a, b) => a.updated.localeCompare(b.updated));

  return { groups: groupByProject(items), detected: [], dismissedCount: 0, notes: [] };
}

/** A loop is anything still owed: closing it is the only way out of the list. */
function isOpen(item: Item): boolean {
  return item.status !== 'done';
}

function render(data: LoopsResult, ctx: Ctx): string {
  const lines: string[] = [];

  if (data.groups.length === 0) {
    lines.push('no open loops');
  } else {
    lines.push('OPEN LOOPS');
    for (const group of data.groups) {
      lines.push('', `  ${group.project ?? '(no project)'}`);
      for (const item of group.items) lines.push(itemLine(item, ctx.now));
    }
  }

  for (const note of data.notes) lines.push('', `  ${note}`);
  return lines.join('\n');
}

export const loops: CommandModule<LoopsResult> = { run, render };
