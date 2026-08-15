import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { DEFAULTS } from '../src/config.ts';

const BIN = path.join(import.meta.dirname, '..', 'bin', 'waid.ts');

type SpawnResult = { code: number; out: string; err: string };

/**
 * A temp home with `config.json` written up front, so the spawned CLI never walks the real
 * `C:\dev` or `~/.claude`: discovery has no roots and the transcript directory does not exist.
 */
function makeHome(): string {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'waid-smoke-'));
  const config = {
    ...DEFAULTS,
    scanRoots: [],
    claudeDir: path.join(home, 'no-such-claude-dir'),
  };
  fs.writeFileSync(path.join(home, 'config.json'), `${JSON.stringify(config, null, 2)}\n`);
  return home;
}

/** Runs `bin/waid.ts` as a real child process — no in-process shortcuts. */
function waid(home: string, argv: string[]): SpawnResult {
  const result = spawnSync(process.execPath, [BIN, '--waid-home', home, ...argv], {
    encoding: 'utf8',
    env: { ...process.env, WAID_HOME: '', WAID_SKIP_GH_DETECT: '1' },
    timeout: 30_000,
  });
  if (result.error) throw result.error;
  return { code: result.status ?? -1, out: result.stdout, err: result.stderr };
}

test('the installed entry point runs --help with no build step', () => {
  const home = makeHome();
  const result = waid(home, ['--help']);

  assert.equal(result.code, 0, `--help failed: ${result.err}`);
  assert.equal(result.err, '');
  assert.ok(result.out.includes('waid loops'), 'usage is missing the command table');
});

test('an item added through the real binary comes back from loops --json', () => {
  const home = makeHome();

  const added = waid(home, ['add', 'ship the README', '-p', home, '--json']);
  assert.equal(added.code, 0, `add failed: ${added.err}`);
  const item = JSON.parse(added.out) as { id: string; title: string };
  assert.match(item.id, /^[0-9a-z]{4}$/);

  const loops = waid(home, ['loops', '--json']);
  assert.equal(loops.code, 0, `loops failed: ${loops.err}`);
  const parsed = JSON.parse(loops.out) as { groups: { items: { id: string; title: string }[] }[] };
  const titles = parsed.groups.flatMap((group) => group.items).map((entry) => entry.title);
  assert.deepEqual(titles, ['ship the README']);
});

test('-i refuses to run when the real process has piped stdio', () => {
  const home = makeHome();

  // spawnSync pipes stdin and stdout, so the child sees exactly what a script or a pipeline gives
  // it: the guard has to fire off the real `process.stdin.isTTY`, not off an injected seam.
  const result = waid(home, ['scan', '-i']);

  assert.equal(result.code, 1);
  assert.equal(result.out, '');
  assert.equal(result.err, '-i requires an interactive terminal\n');
});

test('--json puts a single parseable document on stdout and nothing else', () => {
  const home = makeHome();
  waid(home, ['add', 'only item']);

  const result = waid(home, ['list', '--json']);

  assert.equal(result.code, 0, `list failed: ${result.err}`);
  assert.equal(result.err, '');
  // Parsing the whole stream, unmodified, is the assertion: any banner or trailing line breaks it.
  const parsed = JSON.parse(result.out) as { items: unknown[] };
  assert.equal(parsed.items.length, 1);
});
