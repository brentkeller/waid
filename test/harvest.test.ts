import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

import { decodeProjectSlug, feedLine, finalize, newAccumulator } from '../src/harvest.ts';
import type { Session } from '../src/types.ts';

const FIXTURES = path.join(import.meta.dirname, 'fixtures', 'transcripts');

/**
 * Streams a fixture through the accumulator exactly as `sessions.ts` will: split on `\n` only, so a
 * CRLF file hands each line a trailing `\r`.
 */
function harvest(name: string, fallbackProject: string | null = null): Session {
  const raw = readFileSync(path.join(FIXTURES, name), 'utf8');
  const acc = newAccumulator();
  for (const line of raw.split('\n')) feedLine(acc, line);
  return finalize(acc, { fallbackProject });
}

test('a normal session finalizes with the last ai-title, its cwd, branch, span and prompt count', () => {
  assert.deepEqual(harvest('normal.jsonl', 'C:\\fallback'), {
    id: 'a1b2c3d4',
    title: 'Fix budget chart legend overflow',
    project: 'C:\\dev\\waid',
    branch: 'fix/legend',
    started: '2026-08-14T09:00:00.000Z',
    ended: '2026-08-14T09:06:30.000Z',
    prompts: 2,
  });
});

test('the last ai-title wins over earlier ones', () => {
  const acc = newAccumulator();
  feedLine(acc, JSON.stringify({ type: 'ai-title', aiTitle: 'first', sessionId: 'z9' }));
  feedLine(acc, JSON.stringify({ type: 'ai-title', aiTitle: 'second', sessionId: 'z9' }));
  feedLine(acc, JSON.stringify({ type: 'ai-title', aiTitle: 'third', sessionId: 'z9' }));

  assert.equal(finalize(acc, { fallbackProject: null }).title, 'third');
});

test('sidechain records raise ended but not prompts', () => {
  const session = harvest('normal.jsonl');
  // The 09:02 sidechain prompt sits between the two counted prompts and is not one of them.
  assert.equal(session.prompts, 2);
  assert.equal(session.ended, '2026-08-14T09:06:30.000Z');
});

test('a session with no ai-title titles itself from the first non-meta user message', () => {
  const session = harvest('no-ai-title.jsonl');

  assert.equal(
    session.title,
    'Investigate why the nightly export job silently drops rows whenever the source t',
  );
  assert.equal(session.title.length, 80);
  assert.deepEqual(session, {
    id: 'b2c3d4e5',
    title: session.title,
    project: 'C:\\dev\\demo',
    branch: 'export/fix',
    started: '2026-08-14T10:00:00.000Z',
    ended: '2026-08-14T10:04:00.000Z',
    prompts: 2,
  });
});

test('a sidechain-only session counts zero prompts and falls back for its project', () => {
  assert.deepEqual(harvest('sidechain-only.jsonl', 'C:\\dev\\decoded'), {
    id: 'c3d4e5f6',
    title: 'Grep for every caller of renderLegend.',
    project: 'C:\\dev\\decoded',
    branch: 'agent-work',
    started: '2026-08-14T11:00:00.000Z',
    ended: '2026-08-14T11:00:30.000Z',
    prompts: 0,
  });
});

test('a truncated final line is skipped silently and the rest of the session still parses', () => {
  assert.deepEqual(harvest('truncated.jsonl'), {
    id: 'd4e5f6a7',
    title: 'Run the migration',
    project: 'C:\\dev\\waid',
    branch: 'main',
    started: '2026-08-14T12:00:00.000Z',
    ended: '2026-08-14T12:01:00.000Z',
    prompts: 2,
  });
});

test('CRLF lines parse identically to LF', () => {
  assert.deepEqual(harvest('crlf.jsonl', 'C:\\fallback'), harvest('normal.jsonl', 'C:\\fallback'));
});

test('an empty file finalizes to (untitled) with zero prompts', () => {
  assert.deepEqual(harvest('empty.jsonl', 'C:\\dev\\decoded'), {
    id: '',
    title: '(untitled)',
    project: 'C:\\dev\\decoded',
    branch: null,
    started: null,
    ended: null,
    prompts: 0,
  });
});

test('records that are not JSON objects are ignored without throwing', () => {
  const acc = newAccumulator();
  for (const line of ['', '   ', 'null', '[1,2,3]', '"a string"', '{oops']) feedLine(acc, line);

  assert.deepEqual(finalize(acc, { fallbackProject: null }), {
    id: '',
    title: '(untitled)',
    project: null,
    branch: null,
    started: null,
    ended: null,
    prompts: 0,
  });
});

test('decodeProjectSlug reverses a Windows slug and leaves unrecognisable input alone', () => {
  assert.equal(decodeProjectSlug('C--dev-waid'), 'C:\\dev\\waid');
  assert.equal(decodeProjectSlug('c--dev-dr-devresults'), 'C:\\dev\\dr\\devresults');
  assert.equal(decodeProjectSlug('-home-brent-dev'), '/home/brent/dev');
  assert.equal(decodeProjectSlug('C--'), 'C:\\');
  assert.equal(decodeProjectSlug('not-a-slug'), 'not-a-slug');
  assert.equal(decodeProjectSlug(''), '');
});
