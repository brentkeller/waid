import fs from 'node:fs';
import path from 'node:path';
import { StringDecoder } from 'node:string_decoder';

import { decodeProjectSlug, feedLine, finalize, newAccumulator } from './harvest.ts';
import type { CachedSession, Config } from './types.ts';

/** Bumped whenever a cache written by an older waid can no longer be reused. */
export const CACHE_VERSION = 1;

const CHUNK_BYTES = 64 * 1024;

/** One transcript file on disk, with the stats a sync compares against the cache. */
export type TranscriptFile = {
  path: string;
  /** Name of the Claude Code project directory holding the file — a lossy encoding of the cwd. */
  dir: string;
  /** Session id taken from the filename; only a fallback, since the transcript carries its own. */
  id: string;
  mtimeMs: number;
  size: number;
};

/** `cache/sessions.json` as written. Disposable: deleting it costs one re-sync, nothing more. */
export type SessionCache = {
  version: typeof CACHE_VERSION;
  syncedAt: string;
  sessions: CachedSession[];
};

export type SyncStats = {
  scanned: number;
  parsed: number;
  reused: number;
  removed: number;
};

export type SyncOptions = {
  /** Re-parse every transcript, ignoring the cached stats. */
  full?: boolean;
  now?: Date;
  /** Seam for tests: swap in a reader to observe or fake which transcripts are opened. */
  readLines?: (filePath: string) => Iterable<string>;
};

/**
 * Yields a file's lines without holding more than a chunk in memory, so a multi-megabyte
 * transcript costs a buffer rather than its own size. Line endings are left on the caller —
 * `feedLine` trims, so a CRLF file needs no special handling here.
 */
export function* readLines(filePath: string): Generator<string> {
  const fd = fs.openSync(filePath, 'r');
  try {
    const buffer = Buffer.allocUnsafe(CHUNK_BYTES);
    // A multi-byte character can straddle a chunk boundary; the decoder holds the partial bytes.
    const decoder = new StringDecoder('utf8');
    let rest = '';

    for (;;) {
      const bytes = fs.readSync(fd, buffer, 0, CHUNK_BYTES, null);
      if (bytes === 0) break;

      rest += decoder.write(buffer.subarray(0, bytes));
      const lines = rest.split('\n');
      rest = lines.pop() ?? '';
      yield* lines;
    }

    rest += decoder.end();
    if (rest !== '') yield rest;
  } finally {
    fs.closeSync(fd);
  }
}

/**
 * Lists every `<claudeDir>/projects/<project-dir>/<session-id>.jsonl`. A missing or unreadable
 * Claude Code directory is not an error — plenty of machines have none — so it yields nothing.
 */
export function discoverTranscripts(cfg: Config): TranscriptFile[] {
  const root = path.join(cfg.claudeDir, 'projects');
  const files: TranscriptFile[] = [];

  for (const dir of readDirNames(root)) {
    const dirPath = path.join(root, dir);
    for (const name of readFileNames(dirPath)) {
      if (!name.endsWith('.jsonl')) continue;

      const filePath = path.join(dirPath, name);
      const stats = statOrNull(filePath);
      if (stats === null) continue;

      files.push({
        path: filePath,
        dir,
        id: name.slice(0, -'.jsonl'.length),
        mtimeMs: stats.mtimeMs,
        size: stats.size,
      });
    }
  }

  return files;
}

/**
 * Rebuilds `cache/sessions.json`, re-reading only the transcripts whose `mtimeMs` or `size` moved
 * since the last sync. Sessions whose transcript has disappeared are dropped.
 */
export function syncSessions(cfg: Config, opts: SyncOptions = {}): SessionCache & { stats: SyncStats } {
  const read = opts.readLines ?? readLines;
  const files = discoverTranscripts(cfg);
  const cached = opts.full === true ? new Map<string, CachedSession>() : cachedByPath(cfg);

  const sessions: CachedSession[] = [];
  const stats: SyncStats = { scanned: files.length, parsed: 0, reused: 0, removed: 0 };

  for (const file of files) {
    const previous = cached.get(file.path);
    if (previous !== undefined && previous._file.mtimeMs === file.mtimeMs && previous._file.size === file.size) {
      sessions.push(previous);
      stats.reused += 1;
      continue;
    }

    // A transcript deleted between discovery and read is a race, not a failure: it stays counted
    // as scanned but contributes no record, so `parsed + reused` can fall short of `scanned`.
    const parsed = parseTranscript(file, read);
    if (parsed === null) continue;

    sessions.push(parsed);
    stats.parsed += 1;
  }

  const present = new Set(files.map((file) => file.path));
  for (const key of cached.keys()) if (!present.has(key)) stats.removed += 1;

  const cache: SessionCache = {
    version: CACHE_VERSION,
    syncedAt: (opts.now ?? new Date()).toISOString(),
    sessions,
  };
  writeCache(cfg, cache);

  return { ...cache, stats };
}

/** Reads the cache, treating a missing, corrupt or foreign-version file as an empty one. */
export function loadSessions(cfg: Config): { syncedAt: string | null; sessions: CachedSession[] } {
  let parsed: unknown;
  try {
    parsed = JSON.parse(fs.readFileSync(cfg.sessionsCachePath, 'utf8'));
  } catch {
    return { syncedAt: null, sessions: [] };
  }

  if (!isRecord(parsed) || parsed['version'] !== CACHE_VERSION) return { syncedAt: null, sessions: [] };

  const raw = parsed['sessions'];
  const sessions = Array.isArray(raw)
    ? raw.map(narrowSession).filter((session): session is CachedSession => session !== null)
    : [];

  return { syncedAt: str(parsed['syncedAt']), sessions };
}

/** Minutes since the last successful sync, or null when there has never been one. */
export function cacheAgeMinutes(cfg: Config, now: Date): number | null {
  const { syncedAt } = loadSessions(cfg);
  if (syncedAt === null) return null;

  const at = Date.parse(syncedAt);
  if (Number.isNaN(at)) return null;
  return Math.max(0, Math.floor((now.getTime() - at) / 60_000));
}

function parseTranscript(
  file: TranscriptFile,
  read: (filePath: string) => Iterable<string>,
): CachedSession | null {
  const acc = newAccumulator();
  try {
    for (const line of read(file.path)) feedLine(acc, line);
  } catch {
    return null;
  }

  const session = finalize(acc, { fallbackProject: decodeProjectSlug(file.dir) });
  return {
    ...session,
    // An empty transcript carries no `sessionId`; the filename is the only id it has.
    id: session.id || file.id,
    _file: { path: file.path, mtimeMs: file.mtimeMs, size: file.size },
  };
}

function cachedByPath(cfg: Config): Map<string, CachedSession> {
  return new Map(loadSessions(cfg).sessions.map((session) => [session._file.path, session]));
}

function writeCache(cfg: Config, cache: SessionCache): void {
  fs.mkdirSync(cfg.cacheDir, { recursive: true });
  fs.writeFileSync(cfg.sessionsCachePath, `${JSON.stringify(cache, null, 2)}\n`);
}

/** Cache entries are our own writes, but a hand-edited file must not crash a read command. */
function narrowSession(value: unknown): CachedSession | null {
  if (!isRecord(value)) return null;
  const file = value['_file'];
  if (!isRecord(file)) return null;

  const filePath = str(file['path']);
  const mtimeMs = num(file['mtimeMs']);
  const size = num(file['size']);
  const id = str(value['id']);
  const title = str(value['title']);
  if (filePath === null || mtimeMs === null || size === null || id === null || title === null) return null;

  return {
    id,
    title,
    project: str(value['project']),
    branch: str(value['branch']),
    started: str(value['started']),
    ended: str(value['ended']),
    prompts: num(value['prompts']) ?? 0,
    _file: { path: filePath, mtimeMs, size },
  };
}

function readDirNames(dirPath: string): string[] {
  try {
    return fs
      .readdirSync(dirPath, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .sort();
  } catch {
    return [];
  }
}

function readFileNames(dirPath: string): string[] {
  try {
    return fs
      .readdirSync(dirPath, { withFileTypes: true })
      .filter((entry) => entry.isFile())
      .map((entry) => entry.name)
      .sort();
  } catch {
    return [];
  }
}

function statOrNull(filePath: string): fs.Stats | null {
  try {
    return fs.statSync(filePath);
  } catch {
    return null;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function str(value: unknown): string | null {
  return typeof value === 'string' ? value : null;
}

function num(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}
