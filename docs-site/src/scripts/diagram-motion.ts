import { STEP_DURATION, FINAL_DURATION } from './diagram-timeline.ts';

export const clamp = (value: number) => Math.max(0, Math.min(1, value));
export const ease = (value: number) => {
  const t = clamp(value);
  return t * t * t * (t * (t * 6 - 15) + 10);
};
const phase = (time: number, start: number, end: number) => ease((time - start) / (end - start));
const numbers = (value?: string) => (value ?? '').split(/[ ,]+/).filter(Boolean).map(Number);

export function intervals(value: string) {
  const steps = numbers(value);
  const ranges: [number, number][] = [];
  for (const step of steps) {
    const last = ranges.at(-1);
    if (last && last[1] === step) last[1]++;
    else ranges.push([step, step + 1]);
  }
  return ranges;
}

export function presence(time: number, ranges: [number, number][], delay = 0.03, span = 0.16, leave = 0) {
  return Math.max(0, ...ranges.map(([start, end]) => {
    const incoming = start === 0 ? 1 : phase(time, start + delay, start + delay + span);
    const outgoing = phase(time, end + leave, end + leave + 0.14);
    return incoming * (1 - outgoing);
  }));
}

export function choreographyTime(step: number, progress: number, count: number) {
  return step + Math.min(0.999, progress * (step === count - 1 ? FINAL_DURATION / STEP_DURATION : 1));
}

interface Reveal {
  element: SVGElement;
  ranges: [number, number][];
  delay: number;
  span: number;
  leave: number;
  offset: number[];
  pivot: number[];
  scale: number;
  solid: boolean;
  base: string;
}
interface Draw {
  element: SVGElement;
  start: number;
  end: number;
  reverse: boolean;
  marker: string;
}
interface Shift {
  element: SVGElement;
  start: number;
  end: number;
  offset: number[];
  base: string;
}
interface Transit {
  element: SVGElement;
  start: number;
  end: number;
  points: { x: number; y: number }[];
}
interface Scene {
  svg: SVGSVGElement;
  reveals: Reveal[];
  draws: Draw[];
  shifts: Shift[];
  transits: Transit[];
  packets: SVGElement[];
}

export class DiagramMotion {
  private scenes: Scene[];
  private narrow = false;
  private resize: ResizeObserver;
  private current?: [number, number, number];

  constructor(root: HTMLElement) {
    this.narrow = root.getBoundingClientRect().width <= 540;
    this.resize = new ResizeObserver(([entry]) => {
      const narrow = entry.contentRect.width <= 540;
      if (narrow === this.narrow) return;
      this.narrow = narrow;
      if (this.current) this.render(...this.current);
    });
    this.resize.observe(root);
    this.scenes = Array.from(root.querySelectorAll<SVGSVGElement>('.diagram-stage > svg')).map(svg => ({
      svg,
      reveals: Array.from(svg.querySelectorAll<SVGElement>('[data-show]')).map(element => ({
        element,
        ranges: intervals(element.dataset.show ?? ''),
        delay: Number(element.dataset.enter ?? 0.03),
        span: Number(element.dataset.span ?? 0.16),
        leave: Number(element.dataset.leave ?? 0),
        offset: numbers(element.dataset.offset),
        pivot: numbers(element.dataset.pivot),
        scale: Number(element.dataset.scale ?? 1),
        solid: element.hasAttribute('data-solid'),
        base: element.getAttribute('transform') ?? '',
      })),
      draws: Array.from(svg.querySelectorAll<SVGElement>('[data-draw]')).map(element => {
        const [start, end] = numbers(element.dataset.draw);
        element.setAttribute('pathLength', '1');
        return { element, start, end, reverse: element.hasAttribute('data-reverse'), marker: element.getAttribute('marker-end') ?? '' };
      }),
      shifts: Array.from(svg.querySelectorAll<SVGElement>('[data-shift]')).map(element => {
        const [start, end, x, y] = numbers(element.dataset.shift);
        return { element, start, end, offset: [x, y], base: element.getAttribute('transform') ?? '' };
      }),
      transits: Array.from(svg.querySelectorAll<SVGElement>('[data-transit]')).map(element => {
        const [start, end] = numbers(element.dataset.transit);
        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', element.dataset.route ?? 'M0 0');
        const length = path.getTotalLength();
        const points = Array.from({ length: 121 }, (_, index) => {
          const { x, y } = path.getPointAtLength(length * index / 120);
          return { x, y };
        });
        return { element, start, end, points };
      }),
      packets: Array.from(svg.querySelectorAll<SVGElement>('.packet')),
    }));
  }

  render(step: number, progress: number, count: number) {
    this.current = [step, progress, count];
    const position = step + progress;
    const time = choreographyTime(step, progress, count);
    for (const scene of this.scenes) {
      if (scene.svg.classList.contains('scene-mobile') !== this.narrow) continue;
      scene.svg.style.opacity = String(phase(position, 0, 0.09) * (1 - phase(position, count - 0.07, count)));
      for (const track of scene.reveals) {
        const amount = presence(time, track.ranges, track.delay, track.span, track.leave);
        track.element.style.opacity = String(track.solid
          ? presence(time, track.ranges, track.delay, Math.min(track.span, 0.06), track.leave)
          : amount);
        if (track.offset.length || track.scale !== 1) {
          const x = (track.offset[0] ?? 0) * (1 - amount);
          const y = (track.offset[1] ?? 0) * (1 - amount);
          const scale = track.scale + (1 - track.scale) * amount;
          const [px = 0, py = 0] = track.pivot;
          track.element.setAttribute('transform', `${track.base} translate(${x} ${y}) translate(${px} ${py}) scale(${scale}) translate(${-px} ${-py})`);
        }
      }
      for (const track of scene.draws) {
        const amount = phase(time, track.start, track.end);
        track.element.style.strokeDasharray = '1';
        track.element.style.strokeDashoffset = String((1 - amount) * (track.reverse ? -1 : 1));
        if (track.marker) track.element.setAttribute('marker-end', amount > 0.98 ? track.marker : 'none');
      }
      for (const track of scene.shifts) {
        const amount = phase(time, track.start, track.end);
        track.element.setAttribute('transform', `${track.base} translate(${track.offset[0] * amount} ${track.offset[1] * amount})`);
      }
      for (const track of scene.transits) {
        const t = (time - track.start) / (track.end - track.start);
        const amount = ease(t) * 120;
        const index = Math.min(119, Math.floor(amount));
        const fraction = amount - index;
        const a = track.points[index], b = track.points[index + 1];
        track.element.setAttribute('transform', `translate(${a.x + (b.x - a.x) * fraction} ${a.y + (b.y - a.y) * fraction})`);
        track.element.style.opacity = String(phase(t, 0, 0.09) * (1 - phase(t, 0.87, 1)));
      }
      scene.packets.forEach((packet, index) => {
        packet.style.strokeDashoffset = String(100 - ((position * 2.3 + index * 0.19) % 1) * 100);
      });
    }
  }

  disconnect() {
    this.resize.disconnect();
    for (const scene of this.scenes) {
      for (const track of [...scene.reveals, ...scene.shifts]) {
        if (track.base) track.element.setAttribute('transform', track.base);
        else track.element.removeAttribute('transform');
      }
      for (const track of scene.draws) {
        if (track.marker) track.element.setAttribute('marker-end', track.marker);
      }
    }
  }
}
