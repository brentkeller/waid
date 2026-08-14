import { test } from 'node:test';
import assert from 'node:assert/strict';

import { parseArgv } from '../src/args.ts';
import { UserError } from '../src/errors.ts';

test('splits command, positionals and value flags', () => {
  const parsed = parseArgv(['add', 'fix', 'the', 'thing', '-p', 'C:\\dev\\waid', '--session', 's1']);

  assert.equal(parsed.command, 'add');
  assert.deepEqual(parsed.args, ['fix', 'the', 'thing']);
  assert.equal(parsed.flags.project, 'C:\\dev\\waid');
  assert.equal(parsed.flags.session, 's1');
});

test('a boolean flag does not swallow the next argument', () => {
  const parsed = parseArgv(['list', '--all', 'leftover']);

  assert.equal(parsed.command, 'list');
  assert.equal(parsed.flags.all, true);
  assert.deepEqual(parsed.args, ['leftover']);
});

test('every declared boolean flag parses without a value', () => {
  const parsed = parseArgv(['sync', '--json', '--no-sync', '--full', '--last', '--all', '-h']);

  assert.equal(parsed.flags.json, true);
  assert.equal(parsed.flags['no-sync'], true);
  assert.equal(parsed.flags.full, true);
  assert.equal(parsed.flags.last, true);
  assert.equal(parsed.flags.all, true);
  assert.equal(parsed.flags.help, true);
  assert.deepEqual(parsed.args, []);
});

test('--flag=value binds the inline value', () => {
  const parsed = parseArgv(['list', '--project=C:\\dev\\waid', '--status=waiting']);

  assert.equal(parsed.flags.project, 'C:\\dev\\waid');
  assert.equal(parsed.flags.status, 'waiting');
});

test('repeated -t and --tag accumulate into an array', () => {
  const parsed = parseArgv(['add', 'title', '-t', 'bug', '--tag', 'ui', '--tag=perf']);

  assert.deepEqual(parsed.flags.tag, ['bug', 'ui', 'perf']);
});

test('a single tag is still an array', () => {
  assert.deepEqual(parseArgv(['add', 'title', '-t', 'bug']).flags.tag, ['bug']);
});

test('-- ends flag parsing', () => {
  const parsed = parseArgv(['note', 'k3f9', '--', '--not-a-flag', '-p']);

  assert.equal(parsed.command, 'note');
  assert.deepEqual(parsed.args, ['k3f9', '--not-a-flag', '-p']);
  assert.deepEqual(parsed.flags, {});
});

test('a value flag with no value throws UserError', () => {
  assert.throws(() => parseArgv(['add', 'title', '-p']), UserError);
  assert.throws(() => parseArgv(['add', 'title', '-p', '--json']), UserError);
});

test('empty argv yields a null command', () => {
  const parsed = parseArgv([]);

  assert.equal(parsed.command, null);
  assert.deepEqual(parsed.args, []);
  assert.deepEqual(parsed.flags, {});
});

test('flags may precede the command', () => {
  const parsed = parseArgv(['--json', 'loops', '-p', '.']);

  assert.equal(parsed.command, 'loops');
  assert.equal(parsed.flags.json, true);
  assert.equal(parsed.flags.project, '.');
});
