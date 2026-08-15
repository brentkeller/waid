import { chromeHeight, paint } from './paint.ts';
import { createPickState, plan, reduce, resize } from './pick.ts';
import { openTerminal, type Terminal } from './term.ts';
import type { Key, PickRow, PickState, Plan } from '../types.ts';

/** The shell's convention for a process killed by `SIGINT`, which raw mode never delivers. */
const INTERRUPT_EXIT_CODE = 130;

/** Rows the list is never squeezed below, however short the terminal or tall the footer. */
const MIN_VIEWPORT = 1;

/** How the loop ended: the plan to apply, or null when the user cancelled. */
type Ended = { done: true; plan: Plan | null; interrupted?: boolean };

type Outcome = { done: false } | Ended;

/**
 * The picker loop: `term.read` → `keys.decode` (in `term`) → `reduce` → `paint` → `term.write`.
 * Returns the plan on `Enter` and null when the user cancelled, having written nothing either way
 * — `applyPlan` is the only writer, and the caller runs it once the terminal is back.
 *
 * The terminal is restored in a `finally`, so a throw mid-render leaves the shell usable. `Ctrl-C`
 * exits 130 rather than returning, but only after that restore has run.
 */
export async function pick(rows: PickRow[], term: Terminal = openTerminal()): Promise<Plan | null> {
  let result: Ended = { done: true, plan: null };
  try {
    result = await loop(rows, term);
  } finally {
    term.restore();
  }

  if (result.interrupted === true) process.exit(INTERRUPT_EXIT_CODE);
  return result.plan;
}

async function loop(rows: PickRow[], term: Terminal): Promise<Ended> {
  let state = fit(createPickState(rows, MIN_VIEWPORT), term.rows());
  term.write(paint(state));

  for (;;) {
    const input = await term.read();
    // stdin closing under the picker is a cancel: there is nobody left to confirm the batch.
    if (input.kind === 'end') return { done: true, plan: null };

    if (input.kind === 'key') {
      const outcome = act(state, input.key);
      if (outcome.done) return outcome;
      state = reduce(state, input.key);
    }

    // Re-read the height on every paint, so `SIGWINCH` needs no state of its own and a footer that
    // just grew is accounted for in the same pass.
    state = fit(state, term.rows());
    term.write(paint(state));
  }
}

/**
 * The keys the loop owns rather than `reduce`. While the editor is open only `Ctrl-C` is one of
 * them: `Enter` and `Esc` belong to the line editor, so a batch cannot be applied by trying to
 * finish a title.
 */
function act(state: PickState, key: Key): Outcome {
  if (key.name === 'ctrl-c') return { done: true, plan: null, interrupted: true };
  if (state.editing !== null) return { done: false };

  if (key.name === 'enter') return { done: true, plan: plan(state) };
  if (key.name === 'esc') return { done: true, plan: null };
  if (key.name === 'char' && key.value === 'q') return { done: true, plan: null };
  return { done: false };
}

/** Sizes the viewport to the terminal, leaving the footer its room. */
function fit(state: PickState, rows: number): PickState {
  const height = Math.max(MIN_VIEWPORT, rows - chromeHeight(state));
  return height === state.viewport.height ? state : resize(state, height);
}
