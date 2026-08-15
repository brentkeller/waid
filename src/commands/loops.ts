import { stringFlag } from '../args.ts';
import { detectSignals } from '../detect.ts';
import { groupByProject, knownProjects, resolveProject } from '../projects.ts';
import { loadState } from '../store.ts';
import type { CommandModule, Ctx, Item, LoopsResult, PickRow, State } from '../types.ts';
import { itemLine } from './list.ts';
import { heading, noteRows, signalProjects, signalRows } from './scan.ts';

type Detection = Pick<LoopsResult, 'detected' | 'dismissedCount' | 'notes'>;

async function run(ctx: Ctx): Promise<LoopsResult> {
  const state = loadState(ctx.cfg);
  const detection = detect(ctx, state);

  // Resolved after detection so a `-p` partial can name a repo waid has only ever *detected*,
  // matching `scan`.
  const project = resolveProject(
    stringFlag(ctx.flags, 'project'),
    signalProjects(detection.detected, knownProjects(state, ctx.sessions)),
    ctx.cwd,
  );

  const items = state.items
    .filter((item) => isOpen(item) && (project === null || item.project === project))
    .sort((a, b) => a.updated.localeCompare(b.updated));

  const detected =
    project === null
      ? detection.detected
      : detection.detected.filter((signal) => signal.project === project);

  return { ...detection, groups: groupByProject(items), detected };
}

/**
 * Detection is a bonus on top of the declared items, so a failure costs a note rather than the
 * command: whatever waid was told about is still worth showing.
 */
function detect(ctx: Ctx, state: State): Detection {
  try {
    const result = detectSignals(ctx.cfg, {
      sessions: ctx.sessions ?? [],
      state,
      now: ctx.now,
      git: ctx.git,
      gh: ctx.gh,
    });
    return { detected: result.signals, dismissedCount: result.dismissedCount, notes: result.notes };
  } catch (error) {
    const reason = error instanceof Error ? error.message : String(error);
    return {
      detected: [],
      dismissedCount: 0,
      notes: [`detection failed (${reason}); showing declared items only`],
    };
  }
}

/** A loop is anything still owed: closing it is the only way out of the list. */
function isOpen(item: Item): boolean {
  return item.status !== 'done';
}

function rows(data: LoopsResult, ctx: Ctx): PickRow[] {
  const rows: PickRow[] = [];

  if (data.groups.length === 0) {
    rows.push(heading('no open loops'));
  } else {
    rows.push(heading('OPEN LOOPS'));
    for (const group of data.groups) {
      rows.push(heading(''), heading(`  ${group.project ?? '(no project)'}`));
      for (const item of group.items) {
        rows.push({ kind: 'item', id: item.id, text: itemLine(item, ctx.now), item });
      }
    }
  }

  // Detected signals stay a section of their own: they are guesses, and merging them into the
  // declared items would blur which is which.
  const detected = signalRows(data.detected, data.dismissedCount);
  if (detected.length > 0) rows.push(heading(''), ...detected);

  rows.push(...noteRows(data.notes));
  return rows;
}

// Derived from `rows` so the printed screen and the interactive one are the same screen.
function render(data: LoopsResult, ctx: Ctx): string {
  return rows(data, ctx)
    .map((row) => row.text)
    .join('\n');
}

export const loops: CommandModule<LoopsResult> = { run, render, rows, needsSessions: true };
