import { stringFlag } from '../args.ts';
import { detectSignals } from '../detect.ts';
import { pad } from '../format.ts';
import { knownProjects, resolveProject } from '../projects.ts';
import { loadState } from '../store.ts';
import type { CommandModule, Ctx, Signal } from '../types.ts';

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
function signalProjects(signals: readonly Signal[], known: string[]): string[] {
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

function render(data: ScanResult): string {
  const section = detectedSection(data.signals, data.dismissedCount);
  const lines = section.length > 0 ? section : ['nothing detected'];

  for (const note of data.notes) lines.push('', `  ${note}`);
  return lines.join('\n');
}

export const scan: CommandModule<ScanResult> = { run, render, needsSessions: true };
