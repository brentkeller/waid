import { stringFlag } from '../args.ts';
import { UserError } from '../errors.ts';
import { dayBounds, localYmd, pad } from '../format.ts';
import { loadState, readEvents } from '../store.ts';
import type { CachedSession, CommandModule, Ctx, Item, Session } from '../types.ts';

/** One project's share of a day: how many sessions ran in it and how many prompts they took. */
export type DayProject = {
  project: string | null;
  sessions: number;
  prompts: number;
};

/** The items a day touched, split by the kind of event that touched them. */
export type DayItems = {
  added: Item[];
  closed: Item[];
  noted: Item[];
};

export type TodayResult = {
  /** The local calendar day the rest of the record describes, as `YYYY-MM-DD`. */
  date: string;
  /** Sessions overlapping the day, oldest first; zero-prompt sessions are left out. */
  sessions: Session[];
  /** The same sessions rolled up by project, busiest first. */
  projects: DayProject[];
  items: DayItems;
};

const YMD = /^\d{4}-\d{2}-\d{2}$/;

/** Column width for a session title, matching the item rows `list` renders. */
const TITLE_WIDTH = 44;

/** Item activity is labelled by event; the widest label sets the column. */
const LABEL_WIDTH = 8;

async function run(ctx: Ctx): Promise<TodayResult> {
  const date = readDate(stringFlag(ctx.flags, 'date'), ctx.now);
  const { start, end } = dayBounds(date);

  const sessions = (ctx.sessions ?? [])
    .filter((session) => session.prompts > 0 && overlaps(session, start, end))
    .sort((a, b) => span(a).started - span(b).started)
    .map(stripFileStat);

  return { date, sessions, projects: rollUp(sessions), items: itemActivity(ctx, start, end) };
}

/** `--date` when given, otherwise the day the command is being run on. */
function readDate(input: string | undefined, now: Date): string {
  if (input === undefined) return localYmd(now);

  const value = input.trim();
  // A syntactically valid date can still be impossible (`2026-02-31`); the round trip catches it.
  if (!YMD.test(value) || localYmd(dayBounds(value).start) !== value) {
    throw new UserError(`invalid --date: ${input} (expected YYYY-MM-DD)`);
  }
  return value;
}

/**
 * A session belongs to a day if any part of its span falls inside the day's half-open window. A
 * session with only one of the two timestamps is treated as an instant at that time.
 */
function overlaps(session: CachedSession, start: Date, end: Date): boolean {
  const { started, ended } = span(session);
  if (Number.isNaN(started) || Number.isNaN(ended)) return false;
  return started < end.getTime() && ended >= start.getTime();
}

function span(session: CachedSession): { started: number; ended: number } {
  return {
    started: Date.parse(session.started ?? session.ended ?? ''),
    ended: Date.parse(session.ended ?? session.started ?? ''),
  };
}

/** The cache's stat fields are an implementation detail of syncing, not part of a day's report. */
function stripFileStat(session: CachedSession): Session {
  const { _file: _, ...rest } = session;
  return rest;
}

/** Busiest project first, so the day reads as "where the time went"; ties fall back to name. */
function rollUp(sessions: readonly Session[]): DayProject[] {
  const groups = new Map<string | null, DayProject>();
  for (const session of sessions) {
    const group = groups.get(session.project) ?? {
      project: session.project,
      sessions: 0,
      prompts: 0,
    };
    group.sessions += 1;
    group.prompts += session.prompts;
    groups.set(session.project, group);
  }

  return [...groups.values()].sort(
    (a, b) => b.prompts - a.prompts || (a.project ?? '').localeCompare(b.project ?? ''),
  );
}

/**
 * Every item the day's events touched, read straight from the log so the day reflects when work
 * happened rather than an item's current state. An item touched twice is still listed once.
 */
function itemActivity(ctx: Ctx, start: Date, end: Date): DayItems {
  const items = new Map(loadState(ctx.cfg).items.map((item) => [item.id, item]));
  const activity: DayItems = { added: [], closed: [], noted: [] };

  for (const { ev } of readEvents(ctx.cfg)) {
    if (typeof ev !== 'object' || ev === null) continue;
    const record = ev as { ts?: unknown; ev?: unknown; id?: unknown };
    if (typeof record.ts !== 'string' || typeof record.id !== 'string') continue;

    const ts = Date.parse(record.ts);
    if (Number.isNaN(ts) || ts < start.getTime() || ts >= end.getTime()) continue;

    const item = items.get(record.id);
    const bucket = bucketFor(activity, record.ev);
    if (item === undefined || bucket === null || bucket.includes(item)) continue;
    bucket.push(item);
  }

  return activity;
}

function bucketFor(activity: DayItems, ev: unknown): Item[] | null {
  if (ev === 'add') return activity.added;
  if (ev === 'close') return activity.closed;
  if (ev === 'note') return activity.noted;
  return null;
}

function render(data: TodayResult): string {
  const activity = [
    ...data.items.added.map((item) => ['added', item] as const),
    ...data.items.closed.map((item) => ['closed', item] as const),
    ...data.items.noted.map((item) => ['noted', item] as const),
  ];

  if (data.sessions.length === 0 && activity.length === 0) {
    return `nothing recorded for ${data.date}`;
  }

  const lines = [`TODAY  ${data.date}`];

  for (const group of data.projects) {
    lines.push('', `  ${group.project ?? '(no project)'}  ${summarize(group)}`);
    for (const session of data.sessions.filter((s) => s.project === group.project)) {
      lines.push(`    ${pad(session.title, TITLE_WIDTH)}  ${plural(session.prompts, 'prompt')}`);
    }
  }

  if (activity.length > 0) {
    lines.push('', '  ITEMS');
    for (const [label, item] of activity) {
      lines.push(`    ${pad(label, LABEL_WIDTH)}${pad(item.id, 4)}  ${item.title}`);
    }
  }

  return lines.join('\n');
}

function summarize(group: DayProject): string {
  return `${plural(group.sessions, 'session')}, ${plural(group.prompts, 'prompt')}`;
}

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`;
}

export const today: CommandModule<TodayResult> = { run, render, needsSessions: true };
