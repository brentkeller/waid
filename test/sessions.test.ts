import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { loadConfig } from '../src/config.ts';
import {
  cacheAgeMinutes,
  discoverTranscripts,
  loadSessions,
  readLines,
  syncSessions,
} from '../src/sessions.ts';
import type { CachedSession, Config } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

const FIXTURES = path.join(import.meta.dirname, 'fixtures', 'transcripts');

/** The transcript layout every test starts from: two project directories, four sessions. */
const LAYOUT = [
  { dir: 'C--dev-waid', id: 'a1b2c3d4', fixture: 'normal.jsonl' },
  { dir: 'C--dev-waid', id: 'b2c3d4e5', fixture: 'no-ai-title.jsonl' },
  { dir: 'C--dev-decoded', id: 'c3d4e5f6', fixture: 'sidechain-only.jsonl' },
  { dir: 'C--dev-decoded', id: 'empty-session', fixture: 'empty.jsonl' },
];

function makeClaudeDir(layout = LAYOUT): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'waid-claude-'));
  for (const entry of layout) {
    const dir = path.join(root, 'projects', entry.dir);
    fs.mkdirSync(dir, { recursive: true });
    fs.copyFileSync(path.join(FIXTURES, entry.fixture), path.join(dir, `${entry.id}.jsonl`));
  }
  return root;
}

/** A config against a fresh home whose `claudeDir` points at a fresh transcript tree. */
function setup(layout = LAYOUT): { cfg: Config; claudeDir: string } {
  const home = makeHome();
  fs.mkdirSync(path.join(home, 'cache'), { recursive: true });
  const claudeDir = makeClaudeDir(layout);
  return { cfg: { ...loadConfig(home), claudeDir }, claudeDir };
}

/** Wraps the real line reader so a test can assert which transcripts were opened. */
function countingReader(opened: string[]): (filePath: string) => Iterable<string> {
  return (filePath) => {
    opened.push(filePath);
    return readLines(filePath);
  };
}

function byId(sessions: CachedSession[]): Map<string, CachedSession> {
  return new Map(sessions.map((session) => [session.id, session]));
}

function transcriptPath(claudeDir: string, dir: string, id: string): string {
  return path.join(claudeDir, 'projects', dir, `${id}.jsonl`);
}

test('discoverTranscripts finds every transcript with its stats and directory slug', () => {
  const { cfg, claudeDir } = setup();

  const found = discoverTranscripts(cfg);

  assert.equal(found.length, 4);
  const normal = found.find((file) => file.id === 'a1b2c3d4');
  assert.ok(normal);
  assert.equal(normal.path, transcriptPath(claudeDir, 'C--dev-waid', 'a1b2c3d4'));
  assert.equal(normal.dir, 'C--dev-waid');
  assert.ok(normal.size > 0);
  assert.ok(normal.mtimeMs > 0);
});

test('a first sync parses every transcript and writes the cache', () => {
  const { cfg } = setup();
  const opened: string[] = [];

  const result = syncSessions(cfg, { readLines: countingReader(opened) });

  assert.deepEqual(result.stats, { scanned: 4, parsed: 4, reused: 0, removed: 0 });
  assert.equal(opened.length, 4);
  assert.equal(result.version, 1);

  const sessions = byId(result.sessions);
  const normal = sessions.get('a1b2c3d4');
  assert.ok(normal);
  assert.equal(normal.title, 'Fix budget chart legend overflow');
  assert.equal(normal.project, 'C:\\dev\\waid');
  assert.equal(normal.branch, 'fix/legend');
  assert.equal(normal.prompts, 2);
  assert.equal(normal._file.path, transcriptPath(cfg.claudeDir, 'C--dev-waid', 'a1b2c3d4'));

  // No `cwd` anywhere in a sidechain-only transcript, so the directory slug supplies the project.
  assert.equal(sessions.get('c3d4e5f6')?.project, 'C:\\dev\\decoded');

  // An empty transcript carries no `sessionId`, so the filename is the only id available.
  const empty = sessions.get('empty-session');
  assert.ok(empty);
  assert.equal(empty.title, '(untitled)');
  assert.equal(empty.prompts, 0);

  assert.ok(fs.existsSync(cfg.sessionsCachePath));
  assert.equal(loadSessions(cfg).sessions.length, 4);
});

test('a second sync with untouched files reuses every record and re-reads nothing', () => {
  const { cfg } = setup();
  syncSessions(cfg);

  const opened: string[] = [];
  const result = syncSessions(cfg, { readLines: countingReader(opened) });

  assert.deepEqual(result.stats, { scanned: 4, parsed: 0, reused: 4, removed: 0 });
  assert.deepEqual(opened, []);
  assert.equal(result.sessions.length, 4);
});

test('touching a transcript forces it, and only it, to be re-parsed', () => {
  const { cfg, claudeDir } = setup();
  syncSessions(cfg);

  const touched = transcriptPath(claudeDir, 'C--dev-waid', 'a1b2c3d4');
  const future = new Date(Date.now() + 60_000);
  fs.utimesSync(touched, future, future);

  const opened: string[] = [];
  const result = syncSessions(cfg, { readLines: countingReader(opened) });

  assert.deepEqual(result.stats, { scanned: 4, parsed: 1, reused: 3, removed: 0 });
  assert.deepEqual(opened, [touched]);
});

test('--full re-parses every transcript even when nothing changed', () => {
  const { cfg } = setup();
  syncSessions(cfg);

  const opened: string[] = [];
  const result = syncSessions(cfg, { full: true, readLines: countingReader(opened) });

  assert.deepEqual(result.stats, { scanned: 4, parsed: 4, reused: 0, removed: 0 });
  assert.equal(opened.length, 4);
});

test('a deleted transcript drops out of the cache on the next sync', () => {
  const { cfg, claudeDir } = setup();
  syncSessions(cfg);

  fs.rmSync(transcriptPath(claudeDir, 'C--dev-decoded', 'c3d4e5f6'));
  const result = syncSessions(cfg);

  assert.deepEqual(result.stats, { scanned: 3, parsed: 0, reused: 3, removed: 1 });
  assert.equal(byId(result.sessions).has('c3d4e5f6'), false);
  assert.equal(loadSessions(cfg).sessions.length, 3);
});

test('deleting the cache directory is safe and the next sync rebuilds it', () => {
  const { cfg } = setup();
  syncSessions(cfg);

  fs.rmSync(cfg.cacheDir, { recursive: true, force: true });
  assert.deepEqual(loadSessions(cfg), { syncedAt: null, sessions: [] });

  const result = syncSessions(cfg);

  assert.deepEqual(result.stats, { scanned: 4, parsed: 4, reused: 0, removed: 0 });
  assert.equal(loadSessions(cfg).sessions.length, 4);
});

test('a corrupt cache is a miss, not an error', () => {
  const { cfg } = setup();
  fs.writeFileSync(cfg.sessionsCachePath, '{not json');

  assert.deepEqual(loadSessions(cfg), { syncedAt: null, sessions: [] });
  assert.equal(cacheAgeMinutes(cfg, new Date()), null);
  assert.equal(syncSessions(cfg).stats.parsed, 4);
});

test('a cache written by a future version is ignored rather than trusted', () => {
  const { cfg } = setup();
  fs.writeFileSync(
    cfg.sessionsCachePath,
    JSON.stringify({ version: 99, syncedAt: '2026-08-14T00:00:00.000Z', sessions: [] }),
  );

  assert.deepEqual(loadSessions(cfg), { syncedAt: null, sessions: [] });
});

test('a missing claudeDir yields zero sessions and no error', () => {
  const { cfg } = setup();
  const missing = { ...cfg, claudeDir: path.join(cfg.claudeDir, 'does-not-exist') };

  assert.deepEqual(discoverTranscripts(missing), []);
  const result = syncSessions(missing);
  assert.deepEqual(result.stats, { scanned: 0, parsed: 0, reused: 0, removed: 0 });
  assert.deepEqual(result.sessions, []);
});

test('cacheAgeMinutes measures the recorded sync time', () => {
  const { cfg } = setup();
  const syncedAt = new Date('2026-08-14T12:00:00.000Z');
  syncSessions(cfg, { now: syncedAt });

  assert.equal(cacheAgeMinutes(cfg, syncedAt), 0);
  assert.equal(cacheAgeMinutes(cfg, new Date('2026-08-14T12:30:00.000Z')), 30);
});

test('waid sync reports its stats and writes the cache doctor reads', async () => {
  const home = makeHome();
  const claudeDir = makeClaudeDir();
  fs.mkdirSync(home, { recursive: true });
  fs.writeFileSync(path.join(home, 'config.json'), JSON.stringify({ claudeDir }, null, 2));

  const first = await waid(home, ['sync', '--json']);
  assert.equal(first.code, 0);
  assert.deepEqual(first.json(), {
    scanned: 4,
    parsed: 4,
    reused: 0,
    removed: 0,
    sessions: 4,
    syncedAt: (first.json() as { syncedAt: string }).syncedAt,
  });

  const second = await waid(home, ['sync']);
  assert.equal(second.code, 0);
  assert.match(second.out, /4 sessions/);
  assert.match(second.out, /reused 4/);

  const health = await waid(home, ['doctor', '--json']);
  assert.equal((health.json() as { cache: { sessions: { exists: boolean } } }).cache.sessions.exists, true);
});

test('readLines splits a transcript on newlines without buffering the whole file', () => {
  const { claudeDir } = setup();
  const lines = [...readLines(transcriptPath(claudeDir, 'C--dev-waid', 'a1b2c3d4'))];

  assert.ok(lines.length > 1);
  for (const line of lines) assert.equal(line.includes('\n'), false);
});
