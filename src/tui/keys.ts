import type { Key } from '../types.ts';

/** Escape sequences the picker acts on, in both normal and application cursor mode. */
const SEQUENCES: Record<string, Key> = {
  '\x1b[A': { name: 'up' },
  '\x1b[B': { name: 'down' },
  '\x1bOA': { name: 'up' },
  '\x1bOB': { name: 'down' },
};

/** Single control bytes the picker acts on. */
const CONTROLS: Record<string, Key> = {
  '\x1b': { name: 'esc' },
  '\x03': { name: 'ctrl-c' },
  '\r': { name: 'enter' },
  '\n': { name: 'enter' },
  '\x7f': { name: 'backspace' },
  '\x08': { name: 'backspace' },
};

/**
 * One read from the terminal as a keystroke, or null when it is a key the picker has no meaning
 * for. Returning null rather than a `char` keeps unrecognised escape sequences and pasted runs out
 * of the line editor, where they would otherwise arrive as text.
 */
export function decode(bytes: Buffer | string): Key | null {
  const input = typeof bytes === 'string' ? bytes : bytes.toString('utf8');

  const sequence = SEQUENCES[input];
  if (sequence !== undefined) return sequence;

  const control = CONTROLS[input];
  if (control !== undefined) return control;

  // A grapheme can span several code units, so count code points rather than string length.
  const points = Array.from(input);
  const value = points.length === 1 ? points[0] : undefined;
  if (value === undefined || !printable(value)) return null;

  return { name: 'char', value };
}

/** Everything outside the C0 range and `DEL`, which the terminal sends as bare control bytes. */
function printable(value: string): boolean {
  const code = value.codePointAt(0);
  return code !== undefined && code >= 0x20 && code !== 0x7f;
}
