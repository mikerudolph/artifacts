import test from 'node:test';
import assert from 'node:assert/strict';
import { DiagramTimeline, STEP_DURATION, FINAL_DURATION } from '../src/scripts/diagram-timeline.ts';

test('autoplays every step, holds the final state, and loops without input', () => {
  for (const count of [4, 5]) {
    const timeline = new DiagramTimeline(count);
    assert.equal(timeline.paused, false);
    for (let cycle = 0; cycle < 3; cycle++) {
      for (let step = 0; step < count; step++) {
        assert.equal(timeline.step, step);
        const duration = step === count - 1 ? FINAL_DURATION : STEP_DURATION;
        assert.equal(timeline.advance(duration - 1), false);
        assert.equal(timeline.advance(1), true);
      }
      assert.equal(timeline.step, 0);
    }
  }
});

test('slow frame rates do not slow the timeline', () => {
  const timeline = new DiagramTimeline(5);
  for (let i = 0; i < 4; i++) timeline.advance(1000);
  timeline.advance(STEP_DURATION - 4000);
  assert.equal(timeline.step, 1);
});

test('a long frame does not skip multiple concepts', () => {
  const timeline = new DiagramTimeline(5);
  timeline.advance(60000);
  assert.equal(timeline.step, 1);
  assert.equal(timeline.elapsed, 0);
});

test('seeking keeps playback running unless the reader explicitly paused', () => {
  const timeline = new DiagramTimeline(5);
  timeline.seek(2);
  timeline.advance(STEP_DURATION);
  assert.equal(timeline.step, 3);
  timeline.paused = true;
  timeline.seek(1);
  timeline.advance(60000);
  assert.equal(timeline.step, 1);
  assert.equal(timeline.elapsed, 0);
  timeline.paused = false;
  timeline.advance(STEP_DURATION);
  assert.equal(timeline.step, 2);
});

test('pause preserves elapsed time and instances remain independent', () => {
  const first = new DiagramTimeline(5), second = new DiagramTimeline(4);
  first.advance(1200);
  first.paused = true;
  first.advance(60000);
  second.advance(STEP_DURATION);
  assert.equal(first.elapsed, 1200);
  assert.equal(first.step, 0);
  assert.equal(second.step, 1);
  first.paused = false;
  first.advance(STEP_DURATION - 1200);
  assert.equal(first.step, 1);
});

test('invalid scenes are rejected', () => {
  assert.throws(() => new DiagramTimeline(1), RangeError);
  const timeline = new DiagramTimeline(5);
  for (const step of [-1, 5, 1.5, NaN]) assert.throws(() => timeline.seek(step), RangeError);
});
