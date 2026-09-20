export const STEP_DURATION = 4400;
export const FINAL_DURATION = STEP_DURATION + 2200;

export class DiagramTimeline {
  readonly stepCount: number;
  step = 0;
  elapsed = 0;
  paused = false;

  constructor(stepCount: number) {
    if (!Number.isInteger(stepCount) || stepCount < 2) throw new RangeError('An explainer needs at least two steps');
    this.stepCount = stepCount;
  }

  get duration() {
    return this.step === this.stepCount - 1 ? FINAL_DURATION : STEP_DURATION;
  }

  get progress() { return this.elapsed / this.duration; }

  advance(delta: number) {
    if (this.paused) return false;
    this.elapsed += Math.max(0, delta);
    if (this.elapsed < this.duration) return false;
    this.step = (this.step + 1) % this.stepCount;
    this.elapsed = 0;
    return true;
  }

  seek(step: number) {
    if (!Number.isInteger(step) || step < 0 || step >= this.stepCount) throw new RangeError('Invalid explainer step');
    this.step = step;
    this.elapsed = 0;
  }
}
