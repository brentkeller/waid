import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { DEFAULTS, DEFAULT_HOME, ensureHome, loadConfig, resolveHome } from '../src/config.ts';
import { UserError } from '../src/errors.ts';

function tempDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'waid-config-'));
}

/** Runs `fn` with WAID_HOME set to `value` (or unset when null), restoring it afterwards. */
function withEnvHome(value: string | null, fn: () => void): void {
  const previous = process.env.WAID_HOME;
  if (value === null) delete process.env.WAID_HOME;
  else process.env.WAID_HOME = value;
  try {
    fn();
  } finally {
    if (previous === undefined) delete process.env.WAID_HOME;
    else process.env.WAID_HOME = previous;
  }
}

test('resolveHome prefers the explicit override', () => {
  withEnvHome('C:\\from\\env', () => {
    assert.equal(resolveHome('C:\\from\\override'), 'C:\\from\\override');
  });
});

test('resolveHome falls back to WAID_HOME', () => {
  withEnvHome('C:\\from\\env', () => {
    assert.equal(resolveHome(), 'C:\\from\\env');
    assert.equal(resolveHome(''), 'C:\\from\\env');
  });
});

test('resolveHome falls back to the default data directory', () => {
  withEnvHome(null, () => {
    assert.equal(resolveHome(), DEFAULT_HOME);
  });
  assert.equal(DEFAULT_HOME, 'C:\\data\\waid');
});

test('ensureHome initialises an empty home', () => {
  const home = tempDir();
  ensureHome(home, { detectUser: () => 'octocat' });

  assert.ok(fs.statSync(path.join(home, 'cache')).isDirectory());
  assert.equal(fs.readFileSync(path.join(home, 'events.jsonl'), 'utf8'), '');

  const ignore = fs.readFileSync(path.join(home, '.gitignore'), 'utf8');
  assert.ok(
    ignore.split(/\r?\n/).some((line) => line.trim() === 'cache/'),
    `expected cache/ in .gitignore, got ${JSON.stringify(ignore)}`,
  );

  const config = JSON.parse(fs.readFileSync(path.join(home, 'config.json'), 'utf8'));
  assert.equal(config.ghUser, 'octocat');
  assert.deepEqual(config.scanRoots, DEFAULTS.scanRoots);
  assert.equal(config.activeWindowDays, DEFAULTS.activeWindowDays);
  assert.equal(config.ghCacheTtlMinutes, DEFAULTS.ghCacheTtlMinutes);
  assert.equal(config.scanMaxDepth, DEFAULTS.scanMaxDepth);
});

test('ensureHome creates the home directory when it does not exist', () => {
  const home = path.join(tempDir(), 'nested', 'waid');
  ensureHome(home, { detectUser: () => null });
  assert.ok(fs.statSync(path.join(home, 'cache')).isDirectory());
});

test('ensureHome leaves an existing config.json untouched', () => {
  const home = tempDir();
  const configPath = path.join(home, 'config.json');
  const original = '{"ghUser":"mine","scanMaxDepth":9}';
  fs.writeFileSync(configPath, original);

  ensureHome(home, { detectUser: () => 'octocat' });

  assert.equal(fs.readFileSync(configPath, 'utf8'), original);
});

test('ensureHome records a null ghUser when detection finds nothing', () => {
  const home = tempDir();
  ensureHome(home, { detectUser: () => null });
  const config = JSON.parse(fs.readFileSync(path.join(home, 'config.json'), 'utf8'));
  assert.equal(config.ghUser, null);
});

test('ensureHome skips gh detection under WAID_SKIP_GH_DETECT', () => {
  const home = tempDir();
  const previous = process.env.WAID_SKIP_GH_DETECT;
  process.env.WAID_SKIP_GH_DETECT = '1';
  try {
    ensureHome(home);
  } finally {
    if (previous === undefined) delete process.env.WAID_SKIP_GH_DETECT;
    else process.env.WAID_SKIP_GH_DETECT = previous;
  }
  const config = JSON.parse(fs.readFileSync(path.join(home, 'config.json'), 'utf8'));
  assert.equal(config.ghUser, null);
});

test('ensureHome mutates nothing in a populated home but ensures cache/ is ignored', () => {
  const home = tempDir();
  const configPath = path.join(home, 'config.json');
  const eventsPath = path.join(home, 'events.jsonl');
  const ignorePath = path.join(home, '.gitignore');
  const gitDir = path.join(home, '.git');

  fs.writeFileSync(configPath, '{"ghUser":"brentkeller"}\n');
  fs.writeFileSync(eventsPath, '{"ev":"add","id":"abcd","title":"existing"}\n');
  fs.writeFileSync(ignorePath, '*.tmp\r\nscratch/\r\n');
  fs.mkdirSync(gitDir);
  fs.writeFileSync(path.join(gitDir, 'HEAD'), 'ref: refs/heads/main\n');

  const configBefore = fs.readFileSync(configPath);
  const eventsBefore = fs.readFileSync(eventsPath);
  const headBefore = fs.readFileSync(path.join(gitDir, 'HEAD'));

  ensureHome(home, { detectUser: () => 'octocat' });

  assert.deepEqual(fs.readFileSync(configPath), configBefore);
  assert.deepEqual(fs.readFileSync(eventsPath), eventsBefore);
  assert.deepEqual(fs.readFileSync(path.join(gitDir, 'HEAD')), headBefore);

  const ignore = fs.readFileSync(ignorePath, 'utf8');
  const lines = ignore.split(/\r?\n/).map((line) => line.trim());
  assert.ok(lines.includes('*.tmp'));
  assert.ok(lines.includes('scratch/'));
  assert.ok(lines.includes('cache/'));

  // A second pass must not append a duplicate entry.
  const ignoreAfterFirst = fs.readFileSync(ignorePath);
  ensureHome(home, { detectUser: () => 'octocat' });
  assert.deepEqual(fs.readFileSync(ignorePath), ignoreAfterFirst);
});

test('ensureHome leaves a .gitignore that already ignores cache/ byte-identical', () => {
  const home = tempDir();
  const ignorePath = path.join(home, '.gitignore');
  fs.writeFileSync(ignorePath, 'cache/\n');
  const before = fs.readFileSync(ignorePath);

  ensureHome(home, { detectUser: () => null });

  assert.deepEqual(fs.readFileSync(ignorePath), before);
});

test('loadConfig fills missing keys from defaults and derives every path', () => {
  const home = tempDir();
  fs.writeFileSync(path.join(home, 'config.json'), JSON.stringify({ ghUser: 'brentkeller' }));

  const cfg = loadConfig(home);

  assert.equal(cfg.ghUser, 'brentkeller');
  assert.deepEqual(cfg.scanRoots, DEFAULTS.scanRoots);
  assert.equal(cfg.claudeDir, DEFAULTS.claudeDir);
  assert.equal(cfg.activeWindowDays, DEFAULTS.activeWindowDays);
  assert.equal(cfg.ghCacheTtlMinutes, DEFAULTS.ghCacheTtlMinutes);
  assert.equal(cfg.scanMaxDepth, DEFAULTS.scanMaxDepth);

  assert.equal(cfg.home, home);
  assert.equal(cfg.configPath, path.join(home, 'config.json'));
  assert.equal(cfg.eventsPath, path.join(home, 'events.jsonl'));
  assert.equal(cfg.cacheDir, path.join(home, 'cache'));
  assert.equal(cfg.sessionsCachePath, path.join(home, 'cache', 'sessions.json'));
  assert.equal(cfg.ghCachePath, path.join(home, 'cache', 'gh.json'));
});

test('loadConfig uses defaults when config.json is absent', () => {
  const home = tempDir();
  const cfg = loadConfig(home);
  assert.equal(cfg.ghUser, null);
  assert.deepEqual(cfg.scanRoots, DEFAULTS.scanRoots);
  assert.equal(cfg.home, home);
});

test('loadConfig throws a UserError naming config.json on invalid JSON', () => {
  const home = tempDir();
  fs.writeFileSync(path.join(home, 'config.json'), '{ not json');

  assert.throws(
    () => loadConfig(home),
    (error: unknown) => {
      assert.ok(error instanceof UserError);
      assert.match(error.message, /config\.json/);
      return true;
    },
  );
});
