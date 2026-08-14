const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;
const YEAR = 52 * WEEK;

/**
 * A compact age for a column: `just now` under a minute, then `45m` / `3h` / `5d` / `2w` / `1y`.
 * Unparseable timestamps render as `?` rather than throwing — the log is untrusted input.
 * Future timestamps read as `just now`, since clock skew between agents is not worth a `-2m`.
 */
export function relTime(iso: string, now: Date = new Date()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return '?';

  const elapsed = now.getTime() - then;
  if (elapsed < MINUTE) return 'just now';
  if (elapsed < HOUR) return `${Math.floor(elapsed / MINUTE)}m`;
  if (elapsed < DAY) return `${Math.floor(elapsed / HOUR)}h`;
  if (elapsed < WEEK) return `${Math.floor(elapsed / DAY)}d`;
  if (elapsed < YEAR) return `${Math.floor(elapsed / WEEK)}w`;
  return `${Math.floor(elapsed / YEAR)}y`;
}

/** Fits text to exactly `width` columns, padding with spaces or truncating with an ellipsis. */
export function pad(text: string, width: number): string {
  if (width <= 0) return '';
  if (text.length === width) return text;
  if (text.length < width) return text + ' '.repeat(width - text.length);
  return width === 1 ? text.slice(0, 1) : `${text.slice(0, width - 1)}…`;
}

/** The local calendar date of a timestamp, as `YYYY-MM-DD`. */
export function localYmd(value: Date | string): string {
  const date = typeof value === 'string' ? new Date(value) : value;
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${date.getFullYear()}-${month}-${day}`;
}

/** The half-open local-time window `[start, end)` covering one calendar day. */
export function dayBounds(ymd: string): { start: Date; end: Date } {
  const [year = 0, month = 1, day = 1] = ymd.split('-').map(Number);
  const start = new Date(year, month - 1, day, 0, 0, 0, 0);
  const end = new Date(year, month - 1, day + 1, 0, 0, 0, 0);
  return { start, end };
}

/**
 * The half-open local-time window `[start, end)` for the Monday-start week containing `date`,
 * shifted by `offsetWeeks` (`-1` for last week).
 */
export function weekBounds(date: Date, offsetWeeks = 0): { start: Date; end: Date } {
  // getDay() is Sunday-based; Monday-start means Sunday belongs to the week that began six days ago.
  const daysSinceMonday = (date.getDay() + 6) % 7;
  const firstDay = date.getDate() - daysSinceMonday + offsetWeeks * 7;
  const start = new Date(date.getFullYear(), date.getMonth(), firstDay, 0, 0, 0, 0);
  const end = new Date(date.getFullYear(), date.getMonth(), firstDay + 7, 0, 0, 0, 0);
  return { start, end };
}
