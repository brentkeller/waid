import { stringFlag } from '../args.ts';
import { detectSignals } from '../detect.ts';
import { pad } from '../format.ts';
import { knownProjects, resolveProject } from '../projects.ts';
import { loadState } from '../store.ts';
import type { CommandModule, Ctx, PickRow, Signal } from '../types.ts';

export type ScanResult = {
  signals: Signal[];
  /** Signals suppressed because their key was dismissed, counted before `-p` narrowing. */
  dismissedCount: number;
  /** Non-fatal degradations, today only an unavailable `gh`. */
  notes: string[];
};

const FOOTER = 'waid promote <key> to track · waid dismiss <key> to hide';

async function run(ctx: Ctx): Promise<ScanResult> {
  const state = loadState(ctx.cfg);
  const detected = detectSignals(ctx.cfg, {
    sessions: ctx.sessions ?? [],
    state,
    now: ctx.now,
    git: ctx.git,
    gh: ctx.gh,
  });

  const project = resolveProject(
    stringFlag(ctx.flags, 'project'),
    signalProjects(detected.signals, knownProjects(state, ctx.sessions)),
    ctx.cwd,
  );

  const signals =
    project === null
      ? detected.signals
      : detected.signals.filter((signal) => signal.project === project);

  return { signals, dismissedCount: detected.dismissedCount, notes: detected.notes };
}

/**
 * The known projects a `-p` partial may match, extended with the repos detection just found. A repo
 * waid has never recorded an item or session against is still a legitimate target for `-p`.
 */
export function signalProjects(signals: readonly Signal[], known: string[]): string[] {
  const projects = [...known];
  for (const signal of signals) {
    if (signal.project !== null && !projects.includes(signal.project)) projects.push(signal.project);
  }
  return projects;
}

/**
 * The `DETECTED` block, shared with `loops` so the two render signals identically. Returns no lines
 * at all when there is nothing detected and nothing dismissed, letting the caller decide what to say.
 */
export function detectedSection(signals: readonly Signal[], dismissedCount: number): string[] {
  if (signals.length === 0 && dismissedCount === 0) return [];

  const width = Math.max(0, ...signals.map((signal) => signal.key.length)) + 2;
  const lines = [`DETECTED  (${signals.length} shown, ${dismissedCount} dismissed)`, ''];
  for (const signal of signals) lines.push(`  ${pad(signal.key, width)}${signal.detail}`);
  lines.push('', `  ${FOOTER}`);
  return lines;
}

/** Lines `detectedSection` puts above the signals: the header and the blank beneath it. */
const DETECTED_HEADER_LINES = 2;

/** A line the cursor skips. */
export function heading(text: string): PickRow {
  return { kind: 'heading', text };
}

/**
 * The degradation notes as rows, in the shape both commands print them.
 */
export function noteRows(notes: readonly string[]): PickRow[] {
  return notes.flatMap((note) => [heading(''), heading(`  ${note}`)]);
}

/**
 * The `DETECTED` block as rows, shared with `loops`. The lines come from `detectedSection`, so a
 * signal row's text is exactly the line the non-interactive command prints for it.
 */
export function signalRows(signals: readonly Signal[], dismissedCount: number): PickRow[] {
  const lines = detectedSection(signals, dismissedCount);
  if (lines.length === 0) return [];

  return [
    ...lines.slice(0, DETECTED_HEADER_LINES).map(heading),
    ...signals.map((signal, index) => ({
      kind: 'signal' as const,
      id: signal.key,
      text: lines[DETECTED_HEADER_LINES + index] ?? '',
      signal,
    })),
    ...lines.slice(DETECTED_HEADER_LINES + signals.length).map(heading),
  ];
}

function rows(data: ScanResult): PickRow[] {
  const section = signalRows(data.signals, data.dismissedCount);
  const body = section.length > 0 ? section : [heading('nothing detected')];
  return [...body, ...noteRows(data.notes)];
}

// Derived from `rows` so the printed screen and the interactive one are the same screen.
function render(data: ScanResult): string {
  return rows(data)
    .map((row) => row.text)
    .join('\n');
}

export const scan: CommandModule<ScanResult> = { run, render, rows, needsSessions: true };
