import { test } from 'node:test';
import assert from 'node:assert/strict';

import { decode } from '../src/tui/keys.ts';

test('decodes the arrow keys the picker moves with', () => {
  assert.deepEqual(decode('\x1b[A'), { name: 'up' });
  assert.deepEqual(decode('\x1b[B'), { name: 'down' });
});

test('decodes arrows sent in application cursor mode', () => {
  assert.deepEqual(decode('\x1bOA'), { name: 'up' });
  assert.deepEqual(decode('\x1bOB'), { name: 'down' });
});

test('ignores the arrows the picker has no use for', () => {
  assert.equal(decode('\x1b[C'), null);
  assert.equal(decode('\x1b[D'), null);
});

test('decodes a bare escape as esc', () => {
  assert.deepEqual(decode('\x1b'), { name: 'esc' });
});

test('leaves an unrecognised escape sequence undecoded rather than typing it', () => {
  assert.equal(decode('\x1b[5~'), null);
  assert.equal(decode('\x1b[1;5A'), null);
});

test('decodes ctrl-c, which cancels with exit 130', () => {
  assert.deepEqual(decode('\x03'), { name: 'ctrl-c' });
});

test('decodes both line endings as enter', () => {
  assert.deepEqual(decode('\r'), { name: 'enter' });
  assert.deepEqual(decode('\n'), { name: 'enter' });
});

test('decodes both backspace conventions', () => {
  assert.deepEqual(decode('\x7f'), { name: 'backspace' });
  assert.deepEqual(decode('\x08'), { name: 'backspace' });
});

test('decodes a printable character as itself', () => {
  assert.deepEqual(decode('p'), { name: 'char', value: 'p' });
  assert.deepEqual(decode('G'), { name: 'char', value: 'G' });
  assert.deepEqual(decode('?'), { name: 'char', value: '?' });
  assert.deepEqual(decode(' '), { name: 'char', value: ' ' });
});

test('decodes a multi-byte character from the buffer the terminal delivers', () => {
  assert.deepEqual(decode(Buffer.from('é', 'utf8')), { name: 'char', value: 'é' });
  assert.deepEqual(decode(Buffer.from('🙂', 'utf8')), { name: 'char', value: '🙂' });
});

test('decodes buffers as well as strings', () => {
  assert.deepEqual(decode(Buffer.from('\x1b[A', 'binary')), { name: 'up' });
  assert.deepEqual(decode(Buffer.from('j', 'binary')), { name: 'char', value: 'j' });
});

test('ignores other control characters and empty reads', () => {
  assert.equal(decode('\x00'), null);
  assert.equal(decode('\x04'), null);
  assert.equal(decode('\t'), null);
  assert.equal(decode(''), null);
});

test('ignores a multi-character read rather than typing part of it', () => {
  assert.equal(decode('pasted'), null);
});
