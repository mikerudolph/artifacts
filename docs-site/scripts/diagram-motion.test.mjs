import test from 'node:test';
import assert from 'node:assert/strict';
import { clamp, ease, intervals, presence, choreographyTime } from '../src/scripts/diagram-motion.ts';
import { STEP_DURATION, FINAL_DURATION } from '../src/scripts/diagram-timeline.ts';

test('easing has continuous intermediate positions and gentle endpoints', () => {
  assert.equal(clamp(-1), 0);
  assert.equal(clamp(2), 1);
  assert.equal(ease(0), 0);
  assert.equal(ease(0.5), 0.5);
  assert.equal(ease(1), 1);
  const samples = Array.from({ length: 121 }, (_, index) => ease(index / 120));
  assert.equal(new Set(samples).size, 121);
  assert.ok(samples.every((value, index) => index === 0 || value > samples[index - 1]));
  assert.ok(samples[1] < 0.00001);
  assert.ok(1 - samples[119] < 0.00001);
});

test('consecutive chapters preserve objects instead of fading them out and back in', () => {
  assert.deepEqual(intervals('0 1 2 3 4'), [[0, 5]]);
  assert.deepEqual(intervals('0 2 3 4'), [[0, 1], [2, 5]]);
  assert.deepEqual(intervals(''), []);
  const range = intervals('1 2 3 4');
  for (const time of [1.9, 2, 2.1, 3, 4, 4.999]) assert.equal(presence(time, range), 1);
});

test('staggered arrivals and delayed departures interpolate without snapping', () => {
  const range = intervals('2 3');
  assert.equal(presence(2.2, range, 0.2, 0.4), 0);
  assert.ok(Math.abs(presence(2.4, range, 0.2, 0.4) - 0.5) < 1e-10);
  assert.equal(presence(2.7, range, 0.2, 0.4), 1);
  assert.equal(presence(4.3, range, 0.2, 0.4, 0.4), 1);
  assert.ok(Math.abs(presence(4.47, range, 0.2, 0.4, 0.4) - 0.5) < 1e-10);
  assert.equal(presence(4.6, range, 0.2, 0.4, 0.4), 0);
});

test('eviction removes only the missing interval and replay restores the same object', () => {
  const range = intervals('0 2 3 4');
  assert.equal(presence(0.8, range, 0.6, 0.15), 1);
  assert.equal(presence(1.5, range, 0.6, 0.15), 0);
  assert.equal(presence(2.5, range, 0.6, 0.15), 0);
  assert.equal(presence(2.8, range, 0.6, 0.15), 1);
  assert.equal(presence(4.99, range, 0.6, 0.15), 1);
});

test('last chapter keeps normal motion speed, then holds the completed mechanism', () => {
  assert.equal(choreographyTime(2, 0.5, 5), 2.5);
  assert.equal(choreographyTime(4, (STEP_DURATION / 2) / FINAL_DURATION, 5), 4.5);
  assert.equal(choreographyTime(4, STEP_DURATION / FINAL_DURATION, 5), 4.999);
  assert.equal(choreographyTime(4, 0.9, 5), 4.999);
  assert.ok(FINAL_DURATION - STEP_DURATION >= 2000);
});
