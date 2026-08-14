import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { UserError } from './errors.ts';
import type { Config, ConfigDefaults } from './types.ts';

/** Data directory used when neither an override nor `WAID_HOME` is given. */
export const DEFAULT_HOME = 'C:\\data\\waid';

export const DEFAULTS: ConfigDefaults = {
  scanRoots: ['C:\\dev'],
  claudeDir: path.join(os.homedir(), '.claude'),
  activeWindowDays: 30,
  ghUser: null,
  ghCacheTtlMinutes: 15,
  scanMaxDepth: 4,
};

const GITIGNORE_ENTRY = 'cache/';

/** Resolves the data directory: explicit override, then `WAID_HOME`, then the default. */
export function resolveHome(override?: string): string {
  const fromEnv = process.env.WAID_HOME;
  return override?.trim() || fromEnv?.trim() || DEFAULT_HOME;
}

/**
 * Reads the GitHub login from `gh`, returning null when `gh` is missing, unauthenticated or slow.
 * Skipped entirely when `WAID_SKIP_GH_DETECT` is set, so tests never shell out.
 */
export function detectGhUser(): string | null {
  if (process.env.WAID_SKIP_GH_DETECT) return null;
  try {
    const out = execFileSync('gh', ['api', 'user', '-q', '.login'], {
      encoding: 'utf8',
      timeout: 5000,
      stdio: ['ignore', 'pipe', 'ignore'],
    });
    return out.trim() || null;
  } catch {
    return null;
  }
}

/**
 * First-run init, idempotent against an already-populated home: it creates only what is missing
 * and never rewrites `config.json`, `events.jsonl` or existing `.gitignore` entries.
 */
export function ensureHome(home: string, opts?: { detectUser?: () => string | null }): void {
  fs.mkdirSync(home, { recursive: true });
  fs.mkdirSync(path.join(home, 'cache'), { recursive: true });

  const eventsPath = path.join(home, 'events.jsonl');
  if (!fs.existsSync(eventsPath)) fs.writeFileSync(eventsPath, '');

  ensureGitignoreEntry(path.join(home, '.gitignore'));

  const configPath = path.join(home, 'config.json');
  if (!fs.existsSync(configPath)) {
    const detectUser = opts?.detectUser ?? detectGhUser;
    const seeded: ConfigDefaults = { ...DEFAULTS, ghUser: detectUser() };
    fs.writeFileSync(configPath, `${JSON.stringify(seeded, null, 2)}\n`);
  }
}

function ensureGitignoreEntry(gitignorePath: string): void {
  if (!fs.existsSync(gitignorePath)) {
    fs.writeFileSync(gitignorePath, `${GITIGNORE_ENTRY}\n`);
    return;
  }

  const current = fs.readFileSync(gitignorePath, 'utf8');
  const ignored = current
    .split(/\r?\n/)
    .map((line) => line.trim())
    .some((line) => line === GITIGNORE_ENTRY || line === 'cache');
  if (ignored) return;

  const eol = current.includes('\r\n') ? '\r\n' : '\n';
  const separator = current.length === 0 || current.endsWith('\n') ? '' : eol;
  fs.appendFileSync(gitignorePath, `${separator}${GITIGNORE_ENTRY}${eol}`);
}

/** Loads `config.json`, filling missing keys from `DEFAULTS` and deriving every path. */
export function loadConfig(home: string): Config {
  const configPath = path.join(home, 'config.json');
  const stored = readConfigFile(configPath);
  const cacheDir = path.join(home, 'cache');

  return {
    scanRoots: pick(stored, 'scanRoots', DEFAULTS.scanRoots),
    claudeDir: pick(stored, 'claudeDir', DEFAULTS.claudeDir),
    activeWindowDays: pick(stored, 'activeWindowDays', DEFAULTS.activeWindowDays),
    ghUser: pick(stored, 'ghUser', DEFAULTS.ghUser),
    ghCacheTtlMinutes: pick(stored, 'ghCacheTtlMinutes', DEFAULTS.ghCacheTtlMinutes),
    scanMaxDepth: pick(stored, 'scanMaxDepth', DEFAULTS.scanMaxDepth),
    home,
    configPath,
    eventsPath: path.join(home, 'events.jsonl'),
    cacheDir,
    sessionsCachePath: path.join(cacheDir, 'sessions.json'),
    ghCachePath: path.join(cacheDir, 'gh.json'),
  };
}

function readConfigFile(configPath: string): Record<string, unknown> {
  let raw: string;
  try {
    raw = fs.readFileSync(configPath, 'utf8');
  } catch {
    return {};
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch (error) {
    const reason = error instanceof Error ? error.message : String(error);
    throw new UserError(`invalid JSON in config.json (${configPath}): ${reason}`);
  }

  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new UserError(`config.json must contain a JSON object (${configPath})`);
  }
  return parsed as Record<string, unknown>;
}

function pick<T>(stored: Record<string, unknown>, key: string, fallback: T): T {
  const value = stored[key];
  return value === undefined ? fallback : (value as T);
}
