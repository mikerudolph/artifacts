import { DiagramTimeline } from './diagram-timeline';
import { DiagramMotion } from './diagram-motion';

// Automatically play through, hold the final state, and loop while visible.
// Only an explicit Pause stops playback; seeking keeps the sequence running.
class ArtifactDiagram extends HTMLElement {
  private timeline!: DiagramTimeline;
  private visible = false;
  private lastTime: number | null = null;
  private frame = 0;
  private observer?: IntersectionObserver;
  private events?: AbortController;
  private drawing!: DiagramMotion;
  private buttons: HTMLButtonElement[] = [];

  connectedCallback() {
    if (this.events) return;
    this.events = new AbortController();
    const { signal } = this.events;
    this.buttons = Array.from(this.querySelectorAll<HTMLButtonElement>('[data-go]'));
    this.timeline ??= new DiagramTimeline(this.buttons.length);
    this.drawing = new DiagramMotion(this);
    this.classList.add('is-ready');
    this.render();

    this.addEventListener('click', (event) => {
      const target = (event.target as Element).closest<HTMLButtonElement>('button');
      if (!target) return;
      if (target.dataset.action === 'play') {
        this.timeline.paused = !this.timeline.paused;
        this.syncPlayback();
        this.announce(this.timeline.paused
          ? 'Playback paused. Choose any step to explore.'
          : 'Playback enabled. The explainer plays through and loops while visible.');
      } else if (target.dataset.go !== undefined) {
        this.select(Number(target.dataset.go));
      }
    }, { signal });

    // Arrow keys supplement native Tab/Enter/Space behavior without introducing
    // tab semantics: these are navigation buttons, not separate tab panels.
    this.querySelector('.diagram-steps')?.addEventListener('keydown', (event) => {
      const key = (event as KeyboardEvent).key;
      const index = this.buttons.indexOf(document.activeElement as HTMLButtonElement);
      if (index < 0 || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(key)) return;
      event.preventDefault();
      const next = key === 'Home' ? 0 : key === 'End' ? this.buttons.length - 1
        : (index + (key === 'ArrowRight' ? 1 : -1) + this.buttons.length) % this.buttons.length;
      this.select(next);
      this.buttons[next].focus();
    }, { signal });

    document.addEventListener('visibilitychange', () => this.syncPlayback(), { signal });

    this.observer = new IntersectionObserver(([entry]) => {
      this.visible = entry.isIntersecting && entry.intersectionRatio >= 0.15;
      this.syncPlayback();
    }, { threshold: [0, 0.15], rootMargin: '-80px 0px -24px 0px' });
    // Measure the drawing, not the article/player/transcript around it.
    this.observer.observe(this.querySelector('.diagram-stage') ?? this);
  }

  disconnectedCallback() {
    this.suspend();
    this.visible = false;
    this.dataset.playing = 'false';
    this.observer?.disconnect();
    this.drawing.disconnect();
    this.events?.abort();
    this.events = undefined;
  }

  private get playing() {
    return !this.timeline.paused && this.visible && !document.hidden;
  }

  private select(step: number) {
    this.timeline.seek(step);
    this.lastTime = null;
    this.render();
    this.announce(this.buttons[step].getAttribute('aria-label') ?? '');
  }

  private syncPlayback() {
    this.suspend();
    this.render();
    if (this.playing) this.frame = requestAnimationFrame(this.tick);
  }

  private suspend() {
    cancelAnimationFrame(this.frame);
    this.frame = 0;
    this.lastTime = null;
  }

  private tick = (time: number) => {
    if (!this.playing) return;
    // Use elapsed time, not a per-frame cap: throttled rendering must not turn
    // a four-second scene into a minute. Suspension resets the clock.
    const delta = this.lastTime === null ? 0 : time - this.lastTime;
    this.lastTime = time;
    if (this.timeline.advance(delta)) this.render();
    this.style.setProperty('--step-progress', String(this.timeline.progress));
    this.drawing.render(this.timeline.step, this.timeline.progress, this.buttons.length);
    this.frame = requestAnimationFrame(this.tick);
  };

  private render() {
    const { step, paused, progress } = this.timeline;
    this.dataset.step = String(step);
    this.dataset.playing = String(this.playing);
    this.dataset.paused = String(paused);
    this.style.setProperty('--step-progress', String(progress));
    this.buttons.forEach((button, index) => {
      if (index === step) button.setAttribute('aria-current', 'step');
      else button.removeAttribute('aria-current');
      button.dataset.past = String(index < step);
    });
    this.querySelectorAll<HTMLElement>('[data-caption]').forEach(caption => {
      caption.hidden = Number(caption.dataset.caption) !== step;
    });
    const counter = this.querySelector('[data-counter]');
    if (counter) counter.textContent = String(step + 1).padStart(2, '0');
    const label = paused ? 'Play' : 'Pause';
    const play = this.querySelector('[data-action="play"]');
    play?.setAttribute('aria-label', `${label} ${this.dataset.story} explainer`);
    const playLabel = this.querySelector('[data-play-label]');
    if (playLabel) playLabel.textContent = label;
    const status = this.querySelector('[data-playback-status]');
    if (status) status.textContent = paused ? 'Paused' : !this.playing ? 'Plays in view'
      : step === this.buttons.length - 1 ? 'Looping shortly' : 'Autoplay';
    this.drawing.render(step, progress, this.buttons.length);
  }

  private announce(message: string) {
    const status = this.querySelector('[data-announcement]');
    if (status) status.textContent = message;
  }
}

if (!customElements.get('artifact-diagram')) customElements.define('artifact-diagram', ArtifactDiagram);
