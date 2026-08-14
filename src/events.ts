import { STATUSES } from './types.ts';
import type { Item, Problem, ProblemReason, State, Status } from './types.ts';

/**
 * Folds raw log lines into items and dismissed keys.
 *
 * Pure: it touches no filesystem and never throws. The input is untrusted JSON read from disk, so
 * every field is narrowed before use and anything unusable becomes a `Problem` carrying the 1-based
 * line number. File order is authoritative — `ts` is display metadata only, since parallel agents
 * can append out-of-order timestamps.
 */
export function foldEvents(rawLines: string[]): State {
  const items = new Map<string, Item>();
  const dismissed: string[] = [];
  const problems: Problem[] = [];

  rawLines.forEach((rawLine, index) => {
    const line = index + 1;
    const report = (reason: ProblemReason, id?: string | null, ev?: string | null): void => {
      problems.push({ line, reason, id: id ?? null, ev: ev ?? null });
    };

    if (rawLine.trim() === '') return;

    let parsed: unknown;
    try {
      parsed = JSON.parse(rawLine);
    } catch {
      report('unparseable');
      return;
    }

    if (!isRecord(parsed)) {
      report('not-an-object');
      return;
    }

    const ev = str(parsed['ev']);
    const ts = str(parsed['ts']) ?? '';
    const id = str(parsed['id']);

    switch (ev) {
      case 'add': {
        const title = str(parsed['title']);
        if (id === null || title === null) {
          report('add-missing-fields', id, ev);
          return;
        }
        if (items.has(id)) {
          report('duplicate-id', id, ev);
          return;
        }
        const status = readStatus(parsed, () => report('bad-status', id, ev));
        items.set(id, {
          id,
          title,
          status: status ?? 'open',
          waitingOn: str(parsed['waitingOn']),
          project: str(parsed['project']),
          session: str(parsed['session']),
          tags: strArray(parsed['tags']),
          notes: [],
          created: ts,
          updated: ts,
        });
        return;
      }

      case 'update':
      case 'note':
      case 'close':
      case 'reopen': {
        const item = id === null ? undefined : items.get(id);
        if (item === undefined) {
          report('unknown-id', id, ev);
          return;
        }
        if (!applyToItem(item, ev, parsed, ts, report)) return;
        item.updated = ts;
        return;
      }

      case 'dismiss':
      case 'undismiss': {
        const key = str(parsed['key']);
        if (key === null) {
          report('missing-key', id, ev);
          return;
        }
        const at = dismissed.indexOf(key);
        if (ev === 'dismiss') {
          if (at === -1) dismissed.push(key);
        } else if (at !== -1) {
          dismissed.splice(at, 1);
        }
        return;
      }

      default:
        report('unknown-ev', id, ev);
    }
  });

  return { items: [...items.values()], dismissed, problems };
}

/**
 * Applies one event to an existing item, returning false when nothing was applied. A bad status is
 * reported and ignored; the event's other fields still land.
 */
function applyToItem(
  item: Item,
  ev: 'update' | 'note' | 'close' | 'reopen',
  parsed: Record<string, unknown>,
  ts: string,
  report: (reason: ProblemReason, id?: string | null, ev?: string | null) => void,
): boolean {
  if (ev === 'note') {
    const text = str(parsed['text']);
    if (text === null) {
      report('note-missing-text', item.id, ev);
      return false;
    }
    item.notes.push({ ts, text });
    return true;
  }

  if (ev === 'close') {
    item.status = 'done';
    return true;
  }

  if (ev === 'reopen') {
    item.status = 'open';
    item.waitingOn = null;
    return true;
  }

  const status = readStatus(parsed, () => report('bad-status', item.id, ev));
  if (status !== null) item.status = status;
  if ('title' in parsed) {
    const title = str(parsed['title']);
    if (title !== null) item.title = title;
  }
  if ('project' in parsed) item.project = str(parsed['project']);
  if ('waitingOn' in parsed) item.waitingOn = str(parsed['waitingOn']);
  if ('tags' in parsed) item.tags = strArray(parsed['tags']);
  return true;
}

/** Reads a `status` field, calling `onBad` for a present-but-invalid value. */
function readStatus(parsed: Record<string, unknown>, onBad: () => void): Status | null {
  const raw = parsed['status'];
  if (raw === undefined || raw === null) return null;
  if (isStatus(raw)) return raw;
  onBad();
  return null;
}

export function isStatus(value: unknown): value is Status {
  return typeof value === 'string' && (STATUSES as readonly string[]).includes(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function str(value: unknown): string | null {
  return typeof value === 'string' ? value : null;
}

function strArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === 'string') : [];
}
