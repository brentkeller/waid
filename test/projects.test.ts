import { test } from 'node:test';
import assert from 'node:assert/strict';

import { UserError } from '../src/errors.ts';
import { knownProjects, normalizePath, resolveProject } from '../src/projects.ts';
import type { Item } from '../src/types.ts';

function item(overrides: Partial<Item>): Item {
  return {
    id: 'aaaa',
    title: 'title',
    status: 'open',
    waitingOn: null,
    project: null,
    session: null,
    tags: [],
    notes: [],
    created: '2026-08-14T00:00:00.000Z',
    updated: '2026-08-14T00:00:00.000Z',
    ...overrides,
  };
}

const KNOWN = ['C:\\dev\\waid', 'C:\\dev\\dr\\devresults', '/home/brent/scratch'];

test('normalizePath trims and strips trailing separators', () => {
  assert.equal(normalizePath('  C:\\dev\\waid  '), 'C:\\dev\\waid');
  assert.equal(normalizePath('C:\\dev\\waid\\'), 'C:\\dev\\waid');
  assert.equal(normalizePath('C:\\dev\\waid\\\\'), 'C:\\dev\\waid');
  assert.equal(normalizePath('/home/brent/scratch/'), '/home/brent/scratch');
});

test('normalizePath leaves bare roots alone', () => {
  assert.equal(normalizePath('C:\\'), 'C:\\');
  assert.equal(normalizePath('C:/'), 'C:/');
  assert.equal(normalizePath('/'), '/');
  assert.equal(normalizePath('  /  '), '/');
});

test('knownProjects unions item and session projects, dropping nulls', () => {
  const state = {
    items: [
      item({ id: '0001', project: 'C:\\dev\\waid' }),
      item({ id: '0002', project: null }),
      item({ id: '0003', project: 'C:\\dev\\dr' }),
    ],
  };
  const sessions = [{ project: 'C:\\dev\\other' }, { project: null }, {}];

  assert.deepEqual(knownProjects(state, sessions), [
    'C:\\dev\\waid',
    'C:\\dev\\dr',
    'C:\\dev\\other',
  ]);
});

test('knownProjects de-duplicates while preserving first-seen order', () => {
  const state = {
    items: [
      item({ id: '0001', project: 'C:\\dev\\waid' }),
      item({ id: '0002', project: 'C:\\dev\\dr' }),
      item({ id: '0003', project: 'C:\\dev\\waid' }),
    ],
  };

  assert.deepEqual(knownProjects(state, [{ project: 'C:\\dev\\dr' }]), [
    'C:\\dev\\waid',
    'C:\\dev\\dr',
  ]);
});

test('knownProjects works without sessions', () => {
  const state = { items: [item({ project: 'C:\\dev\\waid' })] };
  assert.deepEqual(knownProjects(state), ['C:\\dev\\waid']);
});

test('resolveProject maps empty input to null', () => {
  assert.equal(resolveProject(undefined, KNOWN, 'C:\\cwd'), null);
  assert.equal(resolveProject('', KNOWN, 'C:\\cwd'), null);
  assert.equal(resolveProject('   ', KNOWN, 'C:\\cwd'), null);
});

test('resolveProject maps . to the normalized cwd', () => {
  assert.equal(resolveProject('.', KNOWN, 'C:\\dev\\elsewhere\\'), 'C:\\dev\\elsewhere');
  assert.equal(resolveProject(' . ', KNOWN, '/home/brent/'), '/home/brent');
});

test('resolveProject takes absolute paths as given', () => {
  assert.equal(resolveProject('C:\\dev\\brand-new\\', KNOWN, 'C:\\cwd'), 'C:\\dev\\brand-new');
  assert.equal(resolveProject('/var/tmp/x/', KNOWN, 'C:\\cwd'), '/var/tmp/x');
  assert.equal(resolveProject('D:/data/repo', KNOWN, 'C:\\cwd'), 'D:/data/repo');
});

test('resolveProject matches a partial case-insensitively against known projects', () => {
  assert.equal(resolveProject('waid', KNOWN, 'C:\\cwd'), 'C:\\dev\\waid');
  assert.equal(resolveProject('DEVRESULTS', KNOWN, 'C:\\cwd'), 'C:\\dev\\dr\\devresults');
  assert.equal(resolveProject('scratch', KNOWN, 'C:\\cwd'), '/home/brent/scratch');
});

test('resolveProject rejects a partial matching nothing', () => {
  assert.throws(
    () => resolveProject('nope', KNOWN, 'C:\\cwd'),
    (error: unknown) => {
      assert.ok(error instanceof UserError);
      assert.match(error.message, /nope/);
      return true;
    },
  );
});

test('resolveProject rejects an ambiguous partial and carries the candidates', () => {
  assert.throws(
    () => resolveProject('dev', KNOWN, 'C:\\cwd'),
    (error: unknown) => {
      assert.ok(error instanceof UserError);
      assert.deepEqual(error.candidates, ['C:\\dev\\waid', 'C:\\dev\\dr\\devresults']);
      return true;
    },
  );
});

test('resolveProject rejects any partial when nothing is known yet', () => {
  assert.throws(() => resolveProject('waid', [], 'C:\\cwd'), UserError);
});
