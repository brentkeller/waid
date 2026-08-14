import fs from 'node:fs';

import { DEFAULTS, detectGhUser } from '../config.ts';
import { pad, relTime } from '../format.ts';
import { loadState, readEventLines } from '../store.ts';
import type { CommandModule, Ctx, Problem } from '../types.ts';

export type DoctorResult = {
  home: string;
  config: {
    path: string;
    /** False only when `config.json` is unreadable or not a JSON object. */
    valid: boolean;
    /** Keys stored in `config.json` that waid does not recognise — usually typos. */
    unknownKeys: string[];
  };
  log: {
    path: string;
    lines: number;
    items: number;
    problems: Problem[];
  };
  cache: {
    sessions: {
      exists: boolean;
      syncedAt: string | null;
      ageMinutes: number | null;
    };
  };
  gh: {
    available: boolean;
    /** The detected login, falling back to the configured one when `gh` cannot be reached. */
    user: string | null;
  };
  /** False when something needs attention; the command still exits 0, since this is a report. */
  ok: boolean;
};

const LABEL_WIDTH = 10;
const KNOWN_CONFIG_KEYS = Object.keys(DEFAULTS);

async function run(ctx: Ctx): Promise<DoctorResult> {
  const { cfg } = ctx;
  const config = inspectConfig(cfg.configPath);
  const state = loadState(cfg);
  const detected = detectGhUser();

  return {
    home: cfg.home,
    config,
    log: {
      path: cfg.eventsPath,
      lines: readEventLines(cfg).length,
      items: state.items.length,
      problems: state.problems,
    },
    cache: { sessions: inspectSessionsCache(cfg.sessionsCachePath, ctx.now) },
    gh: { available: detected !== null, user: detected ?? cfg.ghUser },
    ok: config.valid && config.unknownKeys.length === 0 && state.problems.length === 0,
  };
}

/** Re-reads `config.json` for the keys `loadConfig` discards, without ever writing to it. */
function inspectConfig(configPath: string): DoctorResult['config'] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(fs.readFileSync(configPath, 'utf8'));
  } catch {
    return { path: configPath, valid: false, unknownKeys: [] };
  }

  if (!isRecord(parsed)) return { path: configPath, valid: false, unknownKeys: [] };

  const unknownKeys = Object.keys(parsed).filter((key) => !KNOWN_CONFIG_KEYS.includes(key));
  return { path: configPath, valid: true, unknownKeys };
}

/**
 * Reports the derived session cache without building it. Phase 1 has no writer, so a missing or
 * unreadable file is simply "not built" rather than a problem.
 */
function inspectSessionsCache(cachePath: string, now: Date): DoctorResult['cache']['sessions'] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(fs.readFileSync(cachePath, 'utf8'));
  } catch {
    return { exists: false, syncedAt: null, ageMinutes: null };
  }

  const syncedAt = isRecord(parsed) && typeof parsed['syncedAt'] === 'string' ? parsed['syncedAt'] : null;
  const parsedAt = syncedAt === null ? Number.NaN : Date.parse(syncedAt);
  const ageMinutes = Number.isNaN(parsedAt)
    ? null
    : Math.max(0, Math.floor((now.getTime() - parsedAt) / 60000));

  return { exists: true, syncedAt, ageMinutes };
}

function render(data: DoctorResult, ctx: Ctx): string {
  const lines = ['DOCTOR', ''];

  lines.push(field('home', data.home));
  lines.push(field('config', data.config.valid ? data.config.path : `${data.config.path}  UNREADABLE`));
  if (data.config.unknownKeys.length > 0) {
    lines.push(field('', `unknown keys: ${data.config.unknownKeys.join(', ')}`));
  }

  lines.push(field('log', data.log.path));
  lines.push(field('', `${count(data.log.lines, 'line')}, ${count(data.log.items, 'item')}, ${count(data.log.problems.length, 'problem')}`));
  lines.push(field('cache', sessionsSummary(data.cache.sessions, ctx.now)));
  lines.push(field('gh', ghSummary(data.gh)));

  if (data.log.problems.length > 0) {
    lines.push('', 'PROBLEMS', '');
    for (const problem of data.log.problems) lines.push(`  ${problemLine(problem)}`);
  }

  lines.push('', data.ok ? 'ok' : 'needs attention');
  return lines.join('\n');
}

function field(label: string, value: string): string {
  return `  ${pad(label, LABEL_WIDTH)}${value}`;
}

function count(value: number, noun: string): string {
  return `${value} ${noun}${value === 1 ? '' : 's'}`;
}

function sessionsSummary(sessions: DoctorResult['cache']['sessions'], now: Date): string {
  if (!sessions.exists) return 'sessions not built';
  if (sessions.syncedAt === null) return 'sessions built, sync time unknown';
  return `sessions synced ${relTime(sessions.syncedAt, now)}`;
}

function ghSummary(gh: DoctorResult['gh']): string {
  if (!gh.available) return gh.user === null ? 'unavailable' : `unavailable (configured as ${gh.user})`;
  return `available as ${gh.user}`;
}

function problemLine(problem: Problem): string {
  const detail = [problem.ev, problem.id === null ? null : `id=${problem.id}`].filter(
    (part): part is string => part !== null && part !== undefined,
  );
  const suffix = detail.length > 0 ? `  ${detail.join('  ')}` : '';
  return `${pad(`line ${problem.line}`, LABEL_WIDTH)}${problem.reason}${suffix}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export const doctor: CommandModule<DoctorResult> = { run, render };
