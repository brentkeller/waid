import type { Mark, PickRow, PickState } from '../types.ts';

/** The letter the gutter shows for each mark, so a marked row is legible without colour. */
const LETTERS: Record<Mark['action'], string> = {
  promote: 'P',
  dismiss: 'D',
  done: 'X',
  waiting: 'W',
};

/** What each mark contributes to the count line, in the order the counts are listed. */
const COUNTS: [Mark['action'], string][] = [
  ['promote', 'to promote'],
  ['dismiss', 'to dismiss'],
  ['done', 'to mark done'],
  ['waiting', 'to mark waiting'],
];

/** The label the editor puts in front of the field it has open. */
const FIELDS: Partial<Record<Mark['action'], string>> = {
  promote: 'title',
  waiting: 'waiting on',
};

const NOTHING_MARKED = 'nothing marked';

const LEGEND =
  'p promote · d dismiss · x done · w waiting · e edit · u unmark · enter apply · q cancel · ? keys';

const EDITOR_LEGEND = 'enter save · esc cancel';

/** The table `?` opens, as key / meaning pairs. */
const KEY_TABLE: [string, string][] = [
  ['j k ↑ ↓', 'move the cursor'],
  ['g G', 'first / last row'],
  ['p', 'promote a detected signal'],
  ['d', 'dismiss a detected signal'],
  ['x', 'mark an item done'],
  ['w', 'mark an item waiting on someone'],
  ['e', "edit the mark's text"],
  ['u', 'unmark the row'],
  ['enter', 'apply every mark and exit'],
  ['q esc', 'cancel, write nothing'],
  ['ctrl-c', 'cancel, exit 130'],
  ['?', 'hide this table'],
];

const KEY_COLUMN = Math.max(...KEY_TABLE.map(([keys]) => keys.length)) + 2;

/** Where the caret sits in the line editor. */
const CARET = '▌';

const INDENT = '  ';

/**
 * The whole screen as lines: the viewport slice of `rows`, padded to the viewport height so the
 * footer holds still while the list scrolls, then the footer itself. Pure — the caller writes it.
 *
 * Colour is additive: stripping every escape leaves exactly the `color: false` screen, so the
 * layout never depends on a terminal that renders SGR.
 */
export function paint(state: PickState, color = true): string[] {
  const { top, height } = state.viewport;

  const lines: string[] = [];
  for (let i = top; i < top + height; i += 1) {
    const row = state.rows[i];
    lines.push(row === undefined ? '' : paintRow(row, i === state.cursor, state.marks, color));
  }

  return [...lines, '', ...footer(state, color)];
}

/**
 * Lines `paint` adds beneath the viewport slice. The caller subtracts this from the terminal
 * height to size the viewport, so the footer growing — `?` opening the key table, the editor
 * opening — shrinks the list rather than pushing the footer off the screen.
 */
export function chromeHeight(state: PickState): number {
  return 1 + footer(state, false).length;
}

function paintRow(
  row: PickRow,
  isCursor: boolean,
  marks: Record<string, Mark>,
  color: boolean,
): string {
  if (row.kind === 'heading') return row.text;

  const mark = marks[row.id];
  // Row text already carries the printed indent, so the gutter is the two columns in front of it.
  const gutter = `${isCursor ? '>' : ' '} ${mark === undefined ? ' ' : LETTERS[mark.action]}`;

  if (isCursor) return bold(gutter + row.text, color);
  return mark === undefined ? gutter + row.text : bold(gutter, color) + row.text;
}

/** The status line and the legend beneath it — two blocks whose height the caller need not know. */
function footer(state: PickState, color: boolean): string[] {
  if (state.editing !== null) {
    const label = FIELDS[state.marks[state.editing.rowId]?.action ?? 'promote'] ?? 'value';
    return [
      `${INDENT}${label}: ${state.editing.value}${CARET}`,
      dim(INDENT + EDITOR_LEGEND, color),
    ];
  }

  const status = `${INDENT}${state.hint ?? counts(state.marks)}`;
  return [status, ...(state.legend ? keyTable(color) : [dim(INDENT + LEGEND, color)])];
}

/** `2 to promote, 1 to dismiss` — what pressing `Enter` would do, in one line. */
function counts(marks: Record<string, Mark>): string {
  const staged = Object.values(marks);
  const parts: string[] = [];
  for (const [action, phrase] of COUNTS) {
    const count = staged.filter((mark) => mark.action === action).length;
    if (count > 0) parts.push(`${count} ${phrase}`);
  }
  return parts.length === 0 ? NOTHING_MARKED : parts.join(', ');
}

function keyTable(color: boolean): string[] {
  return KEY_TABLE.map(([keys, meaning]) =>
    dim(`${INDENT}${keys.padEnd(KEY_COLUMN)}${meaning}`, color),
  );
}

function bold(text: string, color: boolean): string {
  return color ? `\x1b[1m${text}\x1b[0m` : text;
}

function dim(text: string, color: boolean): string {
  return color ? `\x1b[2m${text}\x1b[0m` : text;
}
