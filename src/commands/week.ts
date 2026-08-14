import { localYmd, weekBounds } from '../format.ts';
import { loadState, readEvents } from '../store.ts';
import type { CachedSession, CommandModule, Ctx, Session } from '../types.ts';

/** One project's share of a week, carrying its session titles so the rollup reads as a work log. */
export type WeekProject = {
  project: string | null;
  sessions: number;
  prompts: number;
  /** Distinct session titles, oldest first; a resumed session is not listed twice. */
  titles: string[];
  itemsClosed: number;
};

export type WeekResult = {
  /** The window's Monday, as `YYYY-MM-DD`. */
  start: string;
  /** The window's Sunday — the last day covered, not the exclusive bound. */
  end: string;
  /** Projects touched by the week, busiest first. */
  projects: WeekProject[];
  totals: { sessions: number; prompts: number; closed: number };
};

async function run(ctx: Ctx): Promise<WeekResult> {
  const { start, end } = weekBounds(ctx.now, ctx.flags.last === true ? -1 : 0);

  const sessions = (ctx.sessions ?? [])
    .filter((session) => session.prompts > 0 && overlaps(session, start, end))
    .sort((a, b) => span(a).started - span(b).started);

  const projects = rollUp(sessions, closuresByProject(ctx, start, end));

  return {
    start: localYmd(start),
    end: localYmd(new Date(end.getTime() - 1)),
    projects,
    totals: {
      sessions: sum(projects, (group) => group.sessions),
      prompts: sum(projects, (group) => group.prompts),
      closed: sum(projects, (group) => group.itemsClosed),
    },
  };
}

/**
 * A session belongs to the week if any part of its span falls inside the half-open window. A
 * session with only one of the two timestamps is treated as an instant at that time.
 */
function overlaps(session: CachedSession, start: Date, end: Date): boolean {
  const { started, ended } = span(session);
  if (Number.isNaN(started) || Number.isNaN(ended)) return false;
  return started < end.getTime() && ended >= start.getTime();
}

function span(session: Session): { started: number; ended: number } {
  return {
    started: Date.parse(session.started ?? session.ended ?? ''),
    ended: Date.parse(session.ended ?? session.started ?? ''),
  };
}

/**
 * Sessions rolled up by project, busiest first; ties fall back to name. Projects that only closed
 * an item still earn a group, so a week of finishing work does not read as an empty one.
 */
function rollUp(
  sessions: readonly CachedSession[],
  closures: Map<string | null, number>,
): WeekProject[] {
  const groups = new Map<string | null, WeekProject>();
  const group = (project: string | null): WeekProject => {
    const existing = groups.get(project) ?? {
      project,
      sessions: 0,
      prompts: 0,
      titles: [],
      itemsClosed: 0,
    };
    groups.set(project, existing);
    return existing;
  };

  for (const session of sessions) {
    const entry = group(session.project);
    entry.sessions += 1;
    entry.prompts += session.prompts;
    if (!entry.titles.includes(session.title)) entry.titles.push(session.title);
  }

  for (const [project, closed] of closures) group(project).itemsClosed = closed;

  return [...groups.values()].sort(
    (a, b) => b.prompts - a.prompts || (a.project ?? '').localeCompare(b.project ?? ''),
  );
}

/**
 * How many items each project closed inside the window, read from the log so the count reflects
 * when the work finished rather than an item's current state.
 */
function closuresByProject(ctx: Ctx, start: Date, end: Date): Map<string | null, number> {
  const items = new Map(loadState(ctx.cfg).items.map((item) => [item.id, item]));
  const closures = new Map<string | null, number>();
  const counted = new Set<string>();

  for (const { ev } of readEvents(ctx.cfg)) {
    if (typeof ev !== 'object' || ev === null) continue;
    const record = ev as { ts?: unknown; ev?: unknown; id?: unknown };
    if (record.ev !== 'close' || typeof record.ts !== 'string' || typeof record.id !== 'string') {
      continue;
    }

    const ts = Date.parse(record.ts);
    if (Number.isNaN(ts) || ts < start.getTime() || ts >= end.getTime()) continue;

    const item = items.get(record.id);
    // An item closed, reopened and closed again in one week is still one closure.
    if (item === undefined || counted.has(item.id)) continue;
    counted.add(item.id);
    closures.set(item.project, (closures.get(item.project) ?? 0) + 1);
  }

  return closures;
}

function sum(groups: readonly WeekProject[], pick: (group: WeekProject) => number): number {
  return groups.reduce((total, group) => total + pick(group), 0);
}

function render(data: WeekResult): string {
  if (data.projects.length === 0) return `nothing recorded for the week of ${data.start}`;

  const lines = [`WEEK  ${data.start} → ${data.end}`];

  for (const group of data.projects) {
    const heading = summarize(group.sessions, group.prompts, group.itemsClosed);
    lines.push('', `  ${group.project ?? '(no project)'}  ${heading}`);
    for (const title of group.titles) lines.push(`    ${title}`);
  }

  const { sessions, prompts, closed } = data.totals;
  lines.push('', `  TOTAL  ${summarize(sessions, prompts, closed)}`);
  return lines.join('\n');
}

function summarize(sessions: number, prompts: number, closed: number): string {
  return `${plural(sessions, 'session')}, ${plural(prompts, 'prompt')}, ${closed} closed`;
}

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`;
}

export const week: CommandModule<WeekResult> = { run, render, needsSessions: true };
