import { decode } from './keys.ts';
import type { Key } from '../types.ts';

/**
 * The only file that touches `process.stdin` / `process.stdout`. It is deliberately small: it is
 * the part the tests cannot reach without a pty, so it has to be reviewable by eye.
 */

const ALT_SCREEN_ON = '\x1b[?1049h';
const ALT_SCREEN_OFF = '\x1b[?1049l';
const HIDE_CURSOR = '\x1b[?25l';
const SHOW_CURSOR = '\x1b[?25h';
/** Park the cursor at the top left without clearing, so a repaint overwrites in place. */
const HOME = '\x1b[H';
/** Erase from the cursor to the end of the line, and to the end of the screen. */
const CLEAR_LINE = '\x1b[K';
const CLEAR_BELOW = '\x1b[J';

/** Rows assumed when the terminal reports none, matching the classic default. */
const FALLBACK_ROWS = 24;

/** One thing the loop can wake up for: a keystroke, a resize, or stdin closing under it. */
export type Input = { kind: 'key'; key: Key } | { kind: 'resize' } | { kind: 'end' };

export type Terminal = {
  /** Terminal height in rows, re-read on every paint so a resize needs no bookkeeping. */
  rows(): number;
  /** The next input, awaited. Keystrokes arriving between reads are queued, never dropped. */
  read(): Promise<Input>;
  /** Paints the screen in place; `lines` must fit the terminal, since nothing here wraps. */
  write(lines: string[]): void;
  /** Leaves raw mode and the alt screen, and shows the cursor. Idempotent. */
  restore(): void;
};

/**
 * Enters raw mode and the alt screen. The caller must call `restore` from a `finally`; an
 * `exit` handler re-emits the escapes as a backstop, so a throw mid-render cannot strand the
 * user in raw mode with a hidden cursor.
 */
export function openTerminal(): Terminal {
  const { stdin, stdout } = process;

  const queue: Input[] = [];
  let waiting: ((input: Input) => void) | null = null;

  const push = (input: Input): void => {
    const resolve = waiting;
    waiting = null;
    if (resolve === null) queue.push(input);
    else resolve(input);
  };

  const onData = (chunk: Buffer): void => {
    const key = decode(chunk);
    if (key !== null) push({ kind: 'key', key });
  };
  const onEnd = (): void => push({ kind: 'end' });
  const onResize = (): void => push({ kind: 'resize' });

  let restored = false;
  const restore = (): void => {
    if (restored) return;
    restored = true;
    stdin.off('data', onData);
    stdin.off('end', onEnd);
    process.off('SIGWINCH', onResize);
    if (stdin.isTTY) stdin.setRawMode(false);
    stdin.pause();
    stdout.write(SHOW_CURSOR + ALT_SCREEN_OFF);
  };

  // Belt and braces: unconditional, so it also covers a path that never reached the `finally`.
  const onExit = (): void => {
    restore();
    stdout.write(SHOW_CURSOR + ALT_SCREEN_OFF);
  };

  stdin.setRawMode(true);
  stdin.resume();
  stdin.on('data', onData);
  stdin.on('end', onEnd);
  process.on('SIGWINCH', onResize);
  process.on('exit', onExit);
  stdout.write(ALT_SCREEN_ON + HIDE_CURSOR);

  return {
    rows: () => stdout.rows || FALLBACK_ROWS,

    read: async () => {
      const queued = queue.shift();
      if (queued !== undefined) return queued;
      return new Promise<Input>((resolve) => {
        waiting = resolve;
      });
    },

    write: (lines) => {
      // Each line is cleared as it is written and the rest of the screen after them, so a shorter
      // screen leaves no remnant of a taller one. No trailing newline: on a full screen it scrolls.
      const body = lines.map((line) => line + CLEAR_LINE).join('\r\n');
      stdout.write(HOME + body + CLEAR_BELOW);
    },

    restore: () => {
      restore();
      process.off('exit', onExit);
    },
  };
}
