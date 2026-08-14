import fs from 'node:fs';

import { UserError } from './errors.ts';
import { foldEvents } from './events.ts';
import type { Config, EventInput, Item, State, WaidEvent } from './types.ts';

/** Reads `events.jsonl` as lines, returning an empty list when the log does not exist yet. */
export function readEventLines(cfg: Config): string[] {
  let raw: string;
  try {
    raw = fs.readFileSync(cfg.eventsPath, 'utf8');
  } catch {
    return [];
  }

  const lines = raw.split(/\r?\n/);
  if (lines.at(-1) === '') lines.pop();
  return lines;
}

/**
 * Reads the log as parsed JSON values tagged with their 1-based line number. The values stay
 * `unknown` — callers narrow what they need. Blank and unparseable lines are skipped here and
 * reported by `foldEvents` instead, so line numbers stay comparable between the two.
 */
export function readEvents(cfg: Config): { line: number; ev: unknown }[] {
  const events: { line: number; ev: unknown }[] = [];
  readEventLines(cfg).forEach((rawLine, index) => {
    if (rawLine.trim() === '') return;
    try {
      events.push({ line: index + 1, ev: JSON.parse(rawLine) });
    } catch {
      // Reported by the fold; readEvents only yields what it could parse.
    }
  });
  return events;
}

/**
 * Appends one event, stamping `ts` unless the caller supplied one, and returns the event as
 * written. A single sub-4KB `appendFileSync` is atomic enough for the parallel agents that share
 * this log.
 */
export function appendEvent(cfg: Config, event: EventInput): WaidEvent {
  const { ts, ...rest } = event;
  const stamped = { ts: ts ?? new Date().toISOString(), ...rest } as WaidEvent;
  fs.appendFileSync(cfg.eventsPath, `${JSON.stringify(stamped)}\n`);
  return stamped;
}

/** Folds the whole log into the current state. */
export function loadState(cfg: Config): State {
  return foldEvents(readEventLines(cfg));
}

/** Looks an item up by id, rejecting the input when the id is unknown. */
export function requireItem(state: State, id: string): Item {
  const item = state.items.find((candidate) => candidate.id === id);
  if (item === undefined) throw new UserError(`unknown item id: ${id}`);
  return item;
}
