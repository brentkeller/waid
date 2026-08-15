import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import { COMMANDS } from '../src/cli.ts';
import { UserError } from '../src/errors.ts';
import type { CommandModule, PickRow, Plan, Signal } from '../src/types.ts';
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

const SIGNAL: Signal = {
  key: 'dirty:C:\\dev\\waid',
  kind: 'dirty',
  title: '6 uncommitted files in waid',
  detail: '6 files · 20m',
  project: 'C:\\dev\\waid',
  age: '20m',
};

/** A `-i`-capable command whose rows are fixed, so the CLI wiring is what is under test. */
function pickable(rows: PickRow[]): CommandModule<unknown> {
  return {
    run: async () => ({ ok: true }),
    render: () => 'plain output',
    rows: () => rows,
  };
}

test('-i with --json exits 1 without running the command', async () => {
  const home = makeHome();
  let ran = false;
  const mod: CommandModule<unknown> = {
    ...pickable([]),
    run: async () => {
      ran = true;
      return {};
    },
  };

  await withCommand('stub', mod, async () => {
    const result = await waid(home, ['stub', '-i', '--json'], { isTty: () => true });
    assert.equal(result.code, 1);
    assert.equal(result.out, '');
    assert.match((result.json() as { error: string }).error, /-i cannot be combined with --json/);
    assert.equal(ran, false, 'the guard must fire before the command runs');
  });
});

test('-i without a TTY on both ends exits 1', async () => {
  const home = makeHome();
  await withCommand('stub', pickable([]), async () => {
    const result = await waid(home, ['stub', '-i'], { isTty: () => false });
    assert.equal(result.code, 1);
    assert.equal(result.out, '');
    assert.equal(result.err.trim(), '-i requires an interactive terminal');
  });
});

test('-i on a command with no rows() exits 1 naming the command', async () => {
  const home = makeHome();
  const result = await waid(home, ['list', '-i'], { isTty: () => true });

  assert.equal(result.code, 1);
  assert.equal(result.out, '');
  assert.equal(result.err.trim(), 'list does not support -i');
});

test('-i with nothing selectable prints the plain output and never opens the picker', async () => {
  const home = makeHome();
  const headings: PickRow[] = [{ kind: 'heading', text: 'nothing detected' }];
  await withCommand('stub', pickable(headings), async () => {
    let opened = false;
    const pick = async (): Promise<Plan | null> => {
      opened = true;
      return null;
    };

    const plain = await waid(home, ['stub'], { isTty: () => true });
    const result = await waid(home, ['stub', '-i'], { isTty: () => true, pick });

    assert.equal(result.code, 0);
    assert.equal(result.err, '');
    assert.equal(result.out, plain.out);
    assert.equal(opened, false, '-i must not enter raw mode with nothing to select');
  });
});

test('-i applies the confirmed plan and prints its receipts', async () => {
  const home = makeHome();
  const row: PickRow = { kind: 'signal', id: SIGNAL.key, text: '  dirty', signal: SIGNAL };
  await withCommand('stub', pickable([row]), async () => {
    const pick = async (rows: PickRow[]): Promise<Plan> => [
      { row: rows[0] as PickRow, mark: { action: 'dismiss' } },
    ];

    const result = await waid(home, ['stub', '-i'], { isTty: () => true, pick });

    assert.equal(result.code, 0);
    assert.equal(result.out, `dismissed ${SIGNAL.key}\n`);
    const log = fs.readFileSync(path.join(home, 'events.jsonl'), 'utf8').trim();
    assert.equal((JSON.parse(log) as { ev: string; key: string }).key, SIGNAL.key);
  });
});

test('-i cancelled writes nothing and exits 0', async () => {
  const home = makeHome();
  const row: PickRow = { kind: 'signal', id: SIGNAL.key, text: '  dirty', signal: SIGNAL };
  await withCommand('stub', pickable([row]), async () => {
    const result = await waid(home, ['stub', '-i'], { isTty: () => true, pick: async () => null });

    assert.equal(result.code, 0);
    assert.equal(result.out, '');
    assert.equal(fs.readFileSync(path.join(home, 'events.jsonl'), 'utf8'), '');
  });
});
