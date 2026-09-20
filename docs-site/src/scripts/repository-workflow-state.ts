import { clamp, ease } from './diagram-motion.ts';

export const WORKFLOW_DURATION = 18000;
export const COMMIT_AT = 13100;
const phase = (time: number, from: number, to: number) => ease((time - from) / (to - from));

export function flight(time: number, from: number, to: number) {
  const progress = clamp((time - from) / (to - from));
  return {
    progress: ease(progress),
    opacity: phase(progress, 0, 0.1) * (1 - phase(progress, 0.9, 1)),
  };
}

export function exampleHash(revision: number, file = 0) {
  const seed = Math.imul(revision + 31, 0x45d9f3b) ^ Math.imul(file + 7, 0x119de1f3);
  return (seed >>> 0).toString(16).padStart(8, '0').slice(0, 7);
}

export function workflowState(elapsed: number) {
  const clock = Number.isFinite(elapsed) ? Math.max(0, elapsed) : 0;
  const cycle = Math.floor(clock / WORKFLOW_DURATION);
  const time = clock % WORKFLOW_DURATION;
  const committed = time >= COMMIT_AT;
  const revision = cycle + Number(committed);
  return {
    cycle, time, revision, committed,
    stage: time < 3800 ? 0 : time < 9200 ? 1 : time < COMMIT_AT ? 2 : 3,
    read: flight(time, 700, 3500),
    report: flight(time, 9300, 10800),
    state: flight(time, 10800, 12200),
    inspect: phase(time, 3900, 6100),
    compose: phase(time, 6200, 8600),
    stack: phase(time, 12200, COMMIT_AT),
    saved: phase(time, COMMIT_AT, COMMIT_AT + 500),
    head: exampleHash(revision),
    previous: exampleHash(revision - 1),
    planHash: exampleHash(0, 1),
    reportHash: exampleHash(revision, 2),
    stateHash: exampleHash(revision, 3),
  };
}
