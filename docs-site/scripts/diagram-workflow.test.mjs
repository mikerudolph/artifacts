import test from 'node:test';
import assert from 'node:assert/strict';
import { COMMIT_AT, WORKFLOW_DURATION, exampleHash, flight, workflowState } from '../src/scripts/repository-workflow-state.ts';

test('the agent reads before working, then returns both files before publishing', () => {
  const working = workflowState(3800);
  assert.equal(working.read.progress, 1);
  assert.equal(working.inspect, 0);
  const writing = workflowState(9300);
  assert.equal(writing.inspect, 1);
  assert.equal(writing.compose, 1);
  assert.equal(writing.committed, false);
  const publishing = workflowState(12200);
  assert.equal(publishing.report.progress, 1);
  assert.equal(publishing.state.progress, 1);
  assert.equal(publishing.stack, 0);
  assert.equal(publishing.committed, false);
  assert.equal(workflowState(COMMIT_AT).stack, 1);
});

test('publication changes the head and changed files together, preserving plan and history', () => {
  const before = workflowState(COMMIT_AT - 1);
  const after = workflowState(COMMIT_AT);
  assert.equal(before.head, exampleHash(0));
  assert.notEqual(after.head, before.head);
  assert.equal(after.previous, before.head);
  assert.notEqual(after.reportHash, before.reportHash);
  assert.notEqual(after.stateHash, before.stateHash);
  assert.equal(after.planHash, before.planHash);
});

test('the next run continues from the published snapshot without rewinding history', () => {
  for (let cycle = 0; cycle < 20; cycle++) {
    const ending = workflowState((cycle + 1) * WORKFLOW_DURATION - 1);
    const next = workflowState((cycle + 1) * WORKFLOW_DURATION);
    for (const field of ['revision', 'head', 'previous', 'planHash', 'reportHash', 'stateHash']) assert.equal(next[field], ending[field]);
    assert.equal(next.stage, 0);
    assert.equal(ending.stage, 3);
  }
});

test('file motion and snapshot stacking contain continuous intermediate positions', () => {
  const points = Array.from({ length: 121 }, (_, i) => flight(700 + i * 2800 / 120, 700, 3500).progress);
  assert.equal(new Set(points).size, 121);
  assert.ok(points.every((p, i) => i === 0 || p > points[i - 1]));
  assert.equal(flight(700, 700, 3500).opacity, 0);
  assert.equal(flight(3500, 700, 3500).opacity, 0);
  assert.equal(flight(2100, 700, 3500).opacity, 1);
  const stack = workflowState(12650).stack;
  assert.ok(stack > .49 && stack < .51);
});

test('the two output tiles have separate flight windows', () => {
  for (let time = 9000; time <= 12400; time += 10) {
    const state = workflowState(time);
    assert.ok(state.report.opacity === 0 || state.state.opacity === 0);
  }
});

test('invalid clocks start safely and all example hashes are short hexadecimal', () => {
  for (const time of [-1, NaN, Infinity]) assert.deepEqual(workflowState(time), workflowState(0));
  for (let revision = -1; revision < 100; revision++) assert.match(exampleHash(revision), /^[a-f0-9]{7}$/);
});
