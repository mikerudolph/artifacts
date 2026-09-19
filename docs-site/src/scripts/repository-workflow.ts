import { COMMIT_AT, WORKFLOW_DURATION, workflowState } from './repository-workflow-state';

type Point = { x: number; y: number };
const samples = (path: SVGPathElement): Point[] => {
  const length = path.getTotalLength();
  return Array.from({ length: 121 }, (_, index) => {
    const { x, y } = path.getPointAtLength(length * index / 120);
    return { x, y };
  });
};
const position = (points: Point[], progress: number) => {
  const value = Math.max(0, Math.min(1, progress)) * 120;
  const index = Math.min(119, Math.floor(value));
  const amount = value - index;
  return { x: points[index].x + (points[index + 1].x - points[index].x) * amount,
    y: points[index].y + (points[index + 1].y - points[index].y) * amount };
};
const text = (element: Element, value: string) => { if (element.textContent !== value) element.textContent = value; };
const stroke = (element: SVGElement, amount: number) => { element.style.strokeDashoffset = String(1 - amount); };

class RepositoryWorkflow extends HTMLElement {
  private elapsed = 0;
  private frame = 0;
  private lastTime: number | null = null;
  private visible = false;
  private narrow = false;
  private events?: AbortController;
  private observer?: IntersectionObserver;
  private resize?: ResizeObserver;
  private scenes: ReturnType<RepositoryWorkflow['prepare']>[] = [];
  private labels: HTMLElement[] = [];
  private captions: HTMLElement[] = [];

  private prepare(svg: SVGSVGElement) {
    const node = <T extends SVGElement = SVGElement>(selector: string) => svg.querySelector<T>(selector)!;
    return {
      svg, read: samples(node<SVGPathElement>('[data-read-route]')), write: samples(node<SVGPathElement>('[data-write-route]')),
      readTrace: node('[data-read-trace]'), writeTrace: node('[data-write-trace]'),
      packets: ['read', 'report', 'state'].map(name => node(`[data-packet="${name}"]`)),
      layers: [...svg.querySelectorAll<SVGElement>('[data-layer]')], next: node('[data-new-layer]'),
      inspect: node('[data-inspect]'), compose: node('[data-compose]'),
      inspectCheck: node('[data-inspect-check]'), composeCheck: node('[data-compose-check]'),
      head: node('[data-head]'), previous: node('[data-previous]'),
      hashes: ['plan', 'state', 'report'].map(name => node(`[data-file-hash="${name}"]`)),
      flashes: ['plan', 'state', 'report'].map(name => node(`[data-file-flash="${name}"]`)),
      spin: node('[data-agent-spin]'), orbit: node('[data-rotate]'), status: node('[data-agent-status]'),
    };
  }

  connectedCallback() {
    if (this.events) return;
    this.events = new AbortController();
    this.narrow = this.getBoundingClientRect().width <= 420;
    this.scenes = [...this.querySelectorAll<SVGSVGElement>('.workflow-scene')].map(svg => this.prepare(svg));
    this.labels = [...this.querySelectorAll<HTMLElement>('[data-workflow-step]')];
    this.captions = [...this.querySelectorAll<HTMLElement>('[data-workflow-caption]')];
    this.render();
    this.resize = new ResizeObserver(([entry]) => {
      this.narrow = entry.contentRect.width <= 420;
      this.render();
    });
    this.resize.observe(this);
    this.observer = new IntersectionObserver(([entry]) => {
      this.visible = entry.isIntersecting && entry.intersectionRatio >= 0.15;
      this.syncPlayback();
    }, { threshold: [0, 0.15] });
    this.observer.observe(this.querySelector('.workflow-stage')!);
    document.addEventListener('visibilitychange', () => this.syncPlayback(), { signal: this.events.signal });
  }

  disconnectedCallback() {
    this.suspend();
    this.visible = false;
    this.observer?.disconnect();
    this.resize?.disconnect();
    this.events?.abort();
    this.events = undefined;
    this.dataset.playing = 'false';
  }

  private suspend() {
    cancelAnimationFrame(this.frame);
    this.frame = 0;
    this.lastTime = null;
  }

  private syncPlayback() {
    this.suspend();
    const playing = this.visible && !document.hidden;
    this.dataset.playing = String(playing);
    if (playing) this.frame = requestAnimationFrame(this.tick);
  }

  private tick = (now: number) => {
    if (this.lastTime !== null) this.elapsed += Math.max(0, now - this.lastTime);
    this.lastTime = now;
    this.render();
    this.frame = requestAnimationFrame(this.tick);
  };

  private render() {
    const state = workflowState(this.elapsed);
    this.dataset.stage = String(state.stage);
    this.dataset.revision = String(state.revision);
    const starts = [0, 3800, 9200, COMMIT_AT, WORKFLOW_DURATION];
    this.labels.forEach((label, index) => {
      label.dataset.active = String(index === state.stage);
      label.dataset.past = String(index < state.stage);
      label.style.setProperty('--workflow-progress', String((state.time - starts[state.stage]) / (starts[state.stage + 1] - starts[state.stage])));
    });
    this.captions.forEach((caption, index) => { caption.hidden = index !== state.stage; });
    for (const scene of this.scenes) {
      if (scene.svg.classList.contains('workflow-narrow') !== this.narrow) continue;
      [state.read, state.report, state.state].forEach((flight, index) => {
        const point = position(index === 0 ? scene.read : scene.write, flight.progress);
        scene.packets[index].setAttribute('transform', `translate(${point.x} ${point.y})`);
        scene.packets[index].style.opacity = String(flight.opacity);
      });
      stroke(scene.readTrace, state.read.progress);
      scene.readTrace.style.opacity = state.time < 3800 ? '.7' : '.12';
      stroke(scene.writeTrace, Math.max(state.report.progress, state.state.progress));
      scene.writeTrace.style.opacity = state.stage === 2 ? '.65' : '.12';
      scene.layers.forEach(layer => {
        const index = Number(layer.dataset.layer);
        layer.setAttribute('transform', `translate(0 ${(index + state.stack) * 25})`);
        layer.style.opacity = String(index === 2 ? .54 * (1 - state.stack) : 1 - (index + state.stack) * .23);
      });
      scene.next.setAttribute('transform', `translate(0 ${-25 * (1 - state.stack)})`);
      scene.next.style.opacity = String(state.stack);
      stroke(scene.inspect, state.inspect);
      stroke(scene.compose, state.compose);
      stroke(scene.inspectCheck, Math.max(0, (state.inspect - .9) * 10));
      stroke(scene.composeCheck, Math.max(0, (state.compose - .9) * 10));
      scene.spin.setAttribute('transform', `rotate(${(state.inspect + state.compose) * 180})`);
      scene.orbit.setAttribute('transform', `rotate(${(this.elapsed / 100000 * 360) % 360})`);
      text(scene.head, state.head);
      text(scene.previous, state.previous);
      [state.planHash, state.stateHash, state.reportHash].forEach((hash, index) => text(scene.hashes[index], hash));
      scene.flashes[0].style.opacity = String(state.stage === 0 ? .05 + state.read.opacity * .07 : 0);
      scene.flashes[1].style.opacity = scene.flashes[2].style.opacity = String(state.committed ? .14 * (1 - state.saved) + .04 : 0);
      text(scene.status, state.stage === 0 ? 'Loading plan.md' : state.stage === 1 ? 'Working in your runtime' : state.stage === 2 ? 'Saving 2 changed files' : 'New snapshot published');
    }
  }
}

if (!customElements.get('repository-workflow')) customElements.define('repository-workflow', RepositoryWorkflow);
