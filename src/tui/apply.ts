import { promoteSignal } from '../commands/promote.ts';
import { appendEvent, loadState } from '../store.ts';
import type { Config, Mark, PickRow, Plan, State } from '../types.ts';

/**
 * Writes a confirmed batch, in row order, and returns one receipt line per mark — the same lines
 * the individual commands print, so an interactive session leaves the trace in the scrollback that
 * doing it by hand would. The caller prints them after the terminal has been restored.
 *
 * Nothing is re-detected. A signal that vanished while the user was deciding still promotes: the
 * decision was made against what was shown, and re-detecting would turn a race into a spurious
 * failure. Everything the writes need was captured on the rows when the screen opened.
 *
 * A mark that does not match its row's kind cannot be staged by `reduce`, so one here is a caller
 * bug rather than user input; it is skipped rather than written.
 */
export function applyPlan(cfg: Config, plan: Plan): string[] {
  const receipts: string[] = [];
  // Folded at most once, and only for a promote, which is the one action needing the taken ids.
  let state: State | null = null;

  for (const { row, mark } of plan) {
    if (mark.action === 'promote') {
      if (row.kind !== 'signal') continue;
      state ??= loadState(cfg);
      const promoted = promoteSignal(cfg, state, row.signal, mark.title);
      // Keeps the id out of the pool for the rest of the batch, which folds no further logs.
      state.items.push({
        id: promoted.id,
        title: promoted.title,
        status: 'open',
        waitingOn: null,
        project: promoted.project,
        session: null,
        tags: ['promoted'],
        notes: [],
        created: '',
        updated: '',
      });
      receipts.push(`promoted ${promoted.id}  ${promoted.title}`);
      continue;
    }

    const receipt = write(cfg, row, mark);
    if (receipt !== null) receipts.push(receipt);
  }

  return receipts;
}

/** The single-event marks. Returns the receipt, or null when the mark does not fit the row. */
function write(cfg: Config, row: PickRow, mark: Exclude<Mark, { action: 'promote' }>): string | null {
  if (mark.action === 'dismiss') {
    if (row.kind !== 'signal') return null;
    appendEvent(cfg, { ev: 'dismiss', key: row.signal.key });
    return `dismissed ${row.signal.key}`;
  }

  if (row.kind !== 'item') return null;
  const { item } = row;

  if (mark.action === 'done') {
    appendEvent(cfg, { ev: 'close', id: item.id });
    return `closed ${item.id}  ${item.title}`;
  }

  // An editor dismissed without typing leaves the field blank, which is a waiting item with nobody
  // named rather than an item waiting on the empty string.
  const waitingOn = mark.waitingOn.trim() || null;
  appendEvent(cfg, { ev: 'update', id: item.id, status: 'waiting', waitingOn });
  return `waiting ${item.id}  ${item.title}${waitingOn === null ? '' : ` ← ${waitingOn}`}`;
}
