import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import { COMMANDS } from '../src/cli.ts';
import { UserError } from '../src/errors.ts';
import type { CommandModule } from '../src/types.ts';
import { makeHome, waid } from './helpers.ts';

/** Registers a command for the duration of `body`, then removes it again. */
async function withCommand(
  name: string,
  mod: CommandModule<unknown>,
  body: () => Promise<void>,
): Promise<void> {
  COMMANDS[name] = mod;
  try {
    await body();
  } finally {
    delete COMMANDS[name];
  }
}

test('bare invocation prints usage and exits 0', async () => {
  const home = makeHome();
  const result = await waid(home, []);

  assert.equal(result.code, 0);
  assert.equal(result.err, '');
  for (const command of ['sync', 'today', 'week', 'loops', 'scan', 'list', 'add', 'done', 'show']) {
    assert.ok(result.out.includes(`waid ${command}`), `usage is missing ${command}`);
  }
  assert.ok(result.out.includes('--json'), 'usage is missing the global flags');
});

test('--help prints the same usage and exits 0', async () => {
  const home = makeHome();
  const result = await waid(home, ['--help']);

  assert.equal(result.code, 0);
  assert.ok(result.out.includes('waid loops'));
});

test('an unknown command exits 1 with unknown command on stderr', async () => {
  const home = makeHome();
  const result = await waid(home, ['nope']);

  assert.equal(result.code, 1);
  assert.equal(result.out, '');
  assert.match(result.err, /unknown command: nope/);
});

test('an unknown command under --json writes a parseable JSON object to stderr', async () => {
  const home = makeHome();
  const result = await waid(home, ['nope', '--json']);

  assert.equal(result.code, 1);
  assert.equal(result.out, '');
  const parsed = result.json() as { error: string };
  assert.match(parsed.error, /unknown command: nope/);
});

test('running any command initialises WAID_HOME', async () => {
  const home = makeHome();
  await waid(home, ['nope']);

  assert.ok(fs.existsSync(path.join(home, 'config.json')));
  assert.ok(fs.existsSync(path.join(home, 'events.jsonl')));
  assert.ok(fs.existsSync(path.join(home, 'cache')));
});

test('a registered command renders human output by default and JSON under --json', async () => {
  const home = makeHome();
  await withCommand(
    'stub',
    {
      run: async (ctx) => ({ args: ctx.args, project: ctx.flags.project ?? null }),
      render: (data) => `stub ${JSON.stringify(data)}`,
    },
    async () => {
      const human = await waid(home, ['stub', 'one', '-p', 'C:\\dev\\waid']);
      assert.equal(human.code, 0);
      assert.equal(human.out, 'stub {"args":["one"],"project":"C:\\\\dev\\\\waid"}\n');

      const json = await waid(home, ['stub', 'one', '--json']);
      assert.equal(json.code, 0);
      assert.deepEqual(json.json(), { args: ['one'], project: null });
    },
  );
});

test('a command throwing UserError exits 1 with only the message', async () => {
  const home = makeHome();
  await withCommand(
    'boom',
    {
      run: async () => {
        throw new UserError('unknown item id: zzzz');
      },
      render: () => '',
    },
    async () => {
      const result = await waid(home, ['boom']);
      assert.equal(result.code, 1);
      assert.equal(result.err.trim(), 'unknown item id: zzzz');
      assert.ok(!result.err.includes('at '), 'a UserError must not print a stack');
    },
  );
});

test('a UserError with candidates lists them', async () => {
  const home = makeHome();
  await withCommand(
    'boom',
    {
      run: async () => {
        throw new UserError('ambiguous project: dev', { candidates: ['C:\\dev\\a', 'C:\\dev\\b'] });
      },
      render: () => '',
    },
    async () => {
      const result = await waid(home, ['boom']);
      assert.equal(result.code, 1);
      assert.match(result.err, /C:\\dev\\a/);
      assert.match(result.err, /C:\\dev\\b/);

      const json = await waid(home, ['boom', '--json']);
      assert.deepEqual((json.json() as { candidates: string[] }).candidates, [
        'C:\\dev\\a',
        'C:\\dev\\b',
      ]);
    },
  );
});

test('an internal error exits 2 with a stack on stderr', async () => {
  const home = makeHome();
  await withCommand(
    'crash',
    {
      run: async () => {
        throw new Error('kaboom');
      },
      render: () => '',
    },
    async () => {
      const result = await waid(home, ['crash']);
      assert.equal(result.code, 2);
      assert.match(result.err, /kaboom/);
      assert.match(result.err, /at /);
    },
  );
});

test('a flag parse failure exits 1 before any command runs', async () => {
  const home = makeHome();
  const result = await waid(home, ['list', '-p']);

  assert.equal(result.code, 1);
  assert.match(result.err, /requires a value/);
});
