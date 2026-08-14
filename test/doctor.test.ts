import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import type { DoctorResult } from '../src/commands/doctor.ts';
import { makeHome, waid } from './helpers.ts';

/** Writes the log directly so line numbers in the report are deterministic. */
function seedLog(home: string, lines: string[]): void {
  fs.writeFileSync(path.join(home, 'events.jsonl'), `${lines.join('\n')}\n`);
}

function seedConfig(home: string, config: Record<string, unknown>): void {
  fs.writeFileSync(path.join(home, 'config.json'), `${JSON.stringify(config, null, 2)}\n`);
}

async function doctor(home: string): Promise<DoctorResult> {
  const result = await waid(home, ['doctor', '--json']);
  assert.equal(result.code, 0, result.err);
  return result.json() as DoctorResult;
}

test('a clean home reports ok with zero problems', async () => {
  const home = makeHome();

  const data = await doctor(home);
  assert.equal(data.ok, true);
  assert.equal(data.home, home);
  assert.equal(data.config.valid, true);
  assert.deepEqual(data.config.unknownKeys, []);
  assert.equal(data.config.path, path.join(home, 'config.json'));
  assert.equal(data.log.path, path.join(home, 'events.jsonl'));
  assert.equal(data.log.lines, 0);
  assert.equal(data.log.items, 0);
  assert.deepEqual(data.log.problems, []);
});

test('a home with items counts lines and items without reporting problems', async () => {
  const home = makeHome();
  seedLog(home, [
    JSON.stringify({ ts: '2026-08-10T09:00:00.000Z', ev: 'add', id: 'k3f9', title: 'One' }),
    JSON.stringify({ ts: '2026-08-11T09:00:00.000Z', ev: 'add', id: 'm7qz', title: 'Two' }),
    JSON.stringify({ ts: '2026-08-12T09:00:00.000Z', ev: 'close', id: 'k3f9' }),
  ]);

  const data = await doctor(home);
  assert.equal(data.log.lines, 3);
  assert.equal(data.log.items, 2);
  assert.deepEqual(data.log.problems, []);
  assert.equal(data.ok, true);
});

test('an unparseable line and an unknown-id event are reported with line numbers, exit 0', async () => {
  const home = makeHome();
  seedLog(home, [
    JSON.stringify({ ts: '2026-08-10T09:00:00.000Z', ev: 'add', id: 'k3f9', title: 'One' }),
    '{ not json at all',
    JSON.stringify({ ts: '2026-08-12T09:00:00.000Z', ev: 'close', id: 'nope' }),
  ]);

  const result = await waid(home, ['doctor', '--json']);
  assert.equal(result.code, 0, result.err);

  const data = result.json() as DoctorResult;
  assert.equal(data.ok, false);
  assert.equal(data.log.lines, 3);
  assert.equal(data.log.items, 1);
  assert.deepEqual(data.log.problems, [
    { line: 2, reason: 'unparseable', id: null, ev: null },
    { line: 3, reason: 'unknown-id', id: 'nope', ev: 'close' },
  ]);
});

test('unknown config keys are listed and drop ok', async () => {
  const home = makeHome();
  seedConfig(home, { scanRoots: ['C:\\dev'], scanDepth: 4, ghUsr: 'someone' });

  const data = await doctor(home);
  assert.deepEqual(data.config.unknownKeys, ['scanDepth', 'ghUsr']);
  assert.equal(data.config.valid, true);
  assert.equal(data.ok, false);
});

test('gh reports unavailable when detection is skipped', async () => {
  const home = makeHome();
  seedConfig(home, { ghUser: 'brentkeller' });

  const data = await doctor(home);
  assert.equal(data.gh.available, false);
  assert.equal(data.gh.user, 'brentkeller');
});

test('the session cache degrades to not-built before any sync', async () => {
  const home = makeHome();

  const data = await doctor(home);
  assert.equal(data.cache.sessions.exists, false);
  assert.equal(data.cache.sessions.syncedAt, null);
  assert.equal(data.cache.sessions.ageMinutes, null);
});

test('doctor mutates nothing', async () => {
  const home = makeHome();
  const log = [
    JSON.stringify({ ts: '2026-08-10T09:00:00.000Z', ev: 'add', id: 'k3f9', title: 'One' }),
    '{ not json at all',
  ];
  seedLog(home, log);

  const eventsPath = path.join(home, 'events.jsonl');
  const configPath = path.join(home, 'config.json');
  await waid(home, ['doctor']);
  const before = { events: fs.readFileSync(eventsPath, 'utf8'), config: '' };
  before.config = fs.readFileSync(configPath, 'utf8');

  await doctor(home);
  assert.equal(fs.readFileSync(eventsPath, 'utf8'), before.events);
  assert.equal(fs.readFileSync(configPath, 'utf8'), before.config);
});

test('human doctor reports every section and lists problems', async () => {
  const home = makeHome();
  seedLog(home, [
    JSON.stringify({ ts: '2026-08-10T09:00:00.000Z', ev: 'add', id: 'k3f9', title: 'One' }),
    '{ not json at all',
  ]);

  const result = await waid(home, ['doctor']);
  assert.equal(result.code, 0, result.err);

  const lines = result.out.split('\n');
  assert.equal(lines[0], 'DOCTOR');
  assert.match(result.out, /\n {2}home {2,}/);
  assert.match(result.out, /\n {2}config {2,}/);
  assert.match(result.out, /\n {2}log {2,}/);
  assert.match(result.out, /\n {2}cache {2,}/);
  assert.match(result.out, /\n {2}gh {2,}/);
  assert.match(result.out, /PROBLEMS/);
  assert.match(result.out, /line 2 {2,}unparseable/);
});

test('human doctor on a clean home says it is ok', async () => {
  const home = makeHome();

  const result = await waid(home, ['doctor']);
  assert.equal(result.code, 0, result.err);
  assert.doesNotMatch(result.out, /PROBLEMS/);
  assert.match(result.out, /\bok\b/);
});
