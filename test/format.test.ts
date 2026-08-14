import { test } from 'node:test';
import assert from 'node:assert/strict';

import { dayBounds, localYmd, pad, relTime, weekBounds } from '../src/format.ts';

const NOW = new Date('2026-08-14T12:00:00Z');

/** An ISO timestamp `ms` milliseconds before `NOW`. */
function ago(ms: number): string {
  return new Date(NOW.getTime() - ms).toISOString();
}

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;

test('relTime buckets at every boundary', () => {
  assert.equal(relTime(ago(0), NOW), 'just now');
  assert.equal(relTime(ago(59 * SECOND), NOW), 'just now');
  assert.equal(relTime(ago(MINUTE), NOW), '1m');
  assert.equal(relTime(ago(59 * MINUTE), NOW), '59m');
  assert.equal(relTime(ago(HOUR), NOW), '1h');
  assert.equal(relTime(ago(23 * HOUR), NOW), '23h');
  assert.equal(relTime(ago(DAY), NOW), '1d');
  assert.equal(relTime(ago(6 * DAY), NOW), '6d');
  assert.equal(relTime(ago(WEEK), NOW), '1w');
  assert.equal(relTime(ago(51 * WEEK), NOW), '51w');
  assert.equal(relTime(ago(52 * WEEK), NOW), '1y');
  assert.equal(relTime(ago(120 * WEEK), NOW), '2y');
});

test('relTime treats future timestamps as now', () => {
  assert.equal(relTime(new Date(NOW.getTime() + DAY).toISOString(), NOW), 'just now');
});

test('relTime returns a placeholder for an unparseable timestamp', () => {
  assert.equal(relTime('', NOW), '?');
  assert.equal(relTime('not a date', NOW), '?');
});

test('pad right-pads to the requested width', () => {
  assert.equal(pad('ab', 5), 'ab   ');
  assert.equal(pad('', 3), '   ');
  assert.equal(pad('abcde', 5), 'abcde');
});

test('pad truncates with an ellipsis when the text is too long', () => {
  assert.equal(pad('abcdef', 5), 'abcd…');
  assert.equal(pad('abc', 1), 'a');
  assert.equal(pad('abc', 0), '');
});

test('localYmd formats a Date in local time', () => {
  assert.equal(localYmd(new Date(2026, 7, 14, 23, 30)), '2026-08-14');
  assert.equal(localYmd(new Date(2026, 0, 2, 0, 0)), '2026-01-02');
});

test('localYmd accepts an ISO string', () => {
  const local = new Date(2026, 7, 14, 9, 15);
  assert.equal(localYmd(local.toISOString()), '2026-08-14');
});

test('dayBounds spans exactly one local day', () => {
  const { start, end } = dayBounds('2026-08-14');

  assert.equal(localYmd(start), '2026-08-14');
  assert.equal(start.getHours(), 0);
  assert.equal(start.getMinutes(), 0);
  assert.equal(start.getSeconds(), 0);
  assert.equal(start.getMilliseconds(), 0);
  assert.equal(localYmd(end), '2026-08-15');
  assert.equal(end.getHours(), 0);
});

test('weekBounds starts on the Monday of a mid-week date', () => {
  // 2026-08-12 is a Wednesday.
  const { start, end } = weekBounds(new Date(2026, 7, 12, 16, 45));

  assert.equal(localYmd(start), '2026-08-10');
  assert.equal(start.getDay(), 1);
  assert.equal(start.getHours(), 0);
  assert.equal(localYmd(end), '2026-08-17');
});

test('weekBounds puts a Sunday in the week that began the Monday before', () => {
  // 2026-08-16 is a Sunday.
  const { start, end } = weekBounds(new Date(2026, 7, 16, 23, 0));

  assert.equal(localYmd(start), '2026-08-10');
  assert.equal(localYmd(end), '2026-08-17');
});

test('weekBounds shifts back a whole week for offsetWeeks -1', () => {
  const { start, end } = weekBounds(new Date(2026, 7, 12, 16, 45), -1);

  assert.equal(localYmd(start), '2026-08-03');
  assert.equal(localYmd(end), '2026-08-10');
});
