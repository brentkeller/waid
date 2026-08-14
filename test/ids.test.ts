import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ALPHABET, ID_LENGTH, newId } from '../src/ids.ts';

test('ALPHABET is 32 distinct Crockford base32 characters', () => {
  assert.equal(ALPHABET.length, 32);
  assert.equal(new Set(ALPHABET).size, 32);
});

test('ALPHABET omits the ambiguous letters i, l, o and u', () => {
  for (const banned of ['i', 'l', 'o', 'u']) {
    assert.equal(ALPHABET.includes(banned), false, `alphabet contains ${banned}`);
    assert.equal(ALPHABET.includes(banned.toUpperCase()), false, `alphabet contains ${banned.toUpperCase()}`);
  }
});

test('generated ids are four characters drawn only from the alphabet', () => {
  for (let i = 0; i < 200; i += 1) {
    const id = newId();
    assert.equal(id.length, ID_LENGTH);
    for (const char of id) {
      assert.ok(ALPHABET.includes(char), `id ${id} contains ${char}, which is outside the alphabet`);
    }
  }
});

test('newId retries exactly as long as exists returns true', () => {
  let calls = 0;
  const seen: string[] = [];
  const id = newId((candidate) => {
    calls += 1;
    seen.push(candidate);
    return calls <= 3;
  });
  assert.equal(calls, 4);
  assert.equal(seen[3], id);
});

test('newId does not consult exists more than once when the first id is free', () => {
  let calls = 0;
  newId(() => {
    calls += 1;
    return false;
  });
  assert.equal(calls, 1);
});

test('newId throws rather than looping forever when every id is taken', () => {
  let calls = 0;
  assert.throws(
    () =>
      newId(() => {
        calls += 1;
        return true;
      }),
    /unique id/i,
  );
  assert.ok(calls > 1, 'expected more than one attempt before giving up');
  assert.ok(calls < 1000, `expected a bounded number of attempts, got ${calls}`);
});
