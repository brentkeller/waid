import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { realGitClient } from '../src/git.ts';

const git = realGitClient();

function run(repo: string, args: string[]): void {
  execFileSync('git', ['-C', repo, ...args], { stdio: 'ignore', timeout: 20000 });
}

function tempDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'waid-git-'));
}

/** A repo with identity configured and one commit on `main`. */
function makeRepo(): string {
  const dir = tempDir();
  run(dir, ['init', '-b', 'main']);
  configure(dir);
  commit(dir, 'a.txt', 'one\n', 'initial');
  return dir;
}

function configure(repo: string): void {
  run(repo, ['config', 'user.email', 'test@example.com']);
  run(repo, ['config', 'user.name', 'waid test']);
  run(repo, ['config', 'commit.gpgsign', 'false']);
}

function commit(repo: string, file: string, contents: string, message: string): void {
  fs.writeFileSync(path.join(repo, file), contents);
  run(repo, ['add', '.']);
  run(repo, ['commit', '-m', message]);
}

/** Clones `origin` locally, so the clone's branch has a real upstream to be ahead of. */
function makeClone(origin: string): string {
  const dir = path.join(tempDir(), 'clone');
  execFileSync('git', ['clone', origin, dir], { stdio: 'ignore', timeout: 20000 });
  configure(dir);
  return dir;
}

test('headCommitDate returns the date of HEAD', () => {
  const repo = makeRepo();
  const date = git.headCommitDate(repo);
  assert.ok(date instanceof Date);
  assert.ok(Number.isFinite(date.getTime()));
  assert.ok(Math.abs(Date.now() - date.getTime()) < 60_000);
});

test('currentBranch returns the checked-out branch', () => {
  const repo = makeRepo();
  assert.equal(git.currentBranch(repo), 'main');
  run(repo, ['checkout', '-b', 'feature/detect']);
  assert.equal(git.currentBranch(repo), 'feature/detect');
});

test('currentBranch returns null on a detached HEAD', () => {
  const repo = makeRepo();
  run(repo, ['checkout', '--detach', 'HEAD']);
  assert.equal(git.currentBranch(repo), null);
});

test('aheadCount is null when the branch has no upstream', () => {
  const repo = makeRepo();
  assert.equal(git.aheadCount(repo), null);
});

test('aheadCount counts commits made since the upstream', () => {
  const origin = makeRepo();
  const clone = makeClone(origin);

  assert.equal(git.aheadCount(clone), 0);
  commit(clone, 'b.txt', 'two\n', 'second');
  assert.equal(git.aheadCount(clone), 1);
  commit(clone, 'c.txt', 'three\n', 'third');
  assert.equal(git.aheadCount(clone), 2);
});

test('dirtyFileCount counts modified and untracked files', () => {
  const repo = makeRepo();
  assert.equal(git.dirtyFileCount(repo), 0);

  fs.writeFileSync(path.join(repo, 'a.txt'), 'changed\n');
  assert.equal(git.dirtyFileCount(repo), 1);

  fs.writeFileSync(path.join(repo, 'untracked.txt'), 'new\n');
  assert.equal(git.dirtyFileCount(repo), 2);
});

test('isRepo distinguishes a repo from a plain directory', () => {
  assert.equal(git.isRepo(makeRepo()), true);
  assert.equal(git.isRepo(tempDir()), false);
});

test('a non-repo directory returns falsy from every wrapper without throwing', () => {
  const dir = tempDir();
  assert.equal(git.isRepo(dir), false);
  assert.equal(git.headCommitDate(dir), null);
  assert.equal(git.currentBranch(dir), null);
  assert.equal(git.aheadCount(dir), null);
  assert.equal(git.dirtyFileCount(dir), 0);
});

test('a nonexistent path returns falsy from every wrapper without throwing', () => {
  const dir = path.join(tempDir(), 'does', 'not', 'exist');
  assert.equal(git.isRepo(dir), false);
  assert.equal(git.headCommitDate(dir), null);
  assert.equal(git.currentBranch(dir), null);
  assert.equal(git.aheadCount(dir), null);
  assert.equal(git.dirtyFileCount(dir), 0);
});

test('a repo with no commits yet has no HEAD date but is still a repo', () => {
  const dir = tempDir();
  run(dir, ['init', '-b', 'main']);
  assert.equal(git.isRepo(dir), true);
  assert.equal(git.headCommitDate(dir), null);
  assert.equal(git.aheadCount(dir), null);
});
