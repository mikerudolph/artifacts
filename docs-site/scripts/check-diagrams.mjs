import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { stories } from '../src/components/diagrams/stories.ts';

const routes = {
  objects: 'core/concepts',
  publication: 'storage',
  storage: 'storage',
  forks: 'core/forks-and-imports',
};
let steps = 0;
for (const [name, story] of Object.entries(stories)) {
  assert.ok(story.steps.length >= 2 && story.steps.length <= 5, `${name}: visual state selectors support two through five steps`);
  const html = await readFile(new URL(`../dist/${routes[name]}/index.html`, import.meta.url), 'utf8');
  const diagrams = [...html.matchAll(/<artifact-diagram\b([^>]*)>([\s\S]*?)<\/artifact-diagram>/g)];
  const matches = diagrams.filter(match => match[1].includes(`data-story="${name}"`));
  assert.equal(matches.length, 1, `${name}: mount exactly once on its documentation page`);
  const [, attributes, markup] = matches[0];
  assert.match(attributes, new RegExp(`data-step="${story.steps.length - 1}"`), `${name}: useful final-state fallback`);
  assert.match(markup, new RegExp(`aria-labelledby="diagram-${name}-heading"`));
  assert.match(markup, new RegExp(`id="diagram-${name}-summary"`));
  assert.match(markup, /<details\b[^>]*class="diagram-transcript"/);
  assert.equal([...markup.matchAll(/data-go="\d+"/g)].length, story.steps.length);
  assert.equal([...markup.matchAll(/data-caption="\d+"/g)].length, story.steps.length);
  assert.equal([...markup.matchAll(/aria-current="step"/g)].length, 1);
  for (let step = 0; step < story.steps.length; step++) {
    assert.match(markup, new RegExp(`data-go="${step}"`));
    const caption = markup.match(new RegExp(`<div data-caption="${step}"([^>]*)>`));
    assert.ok(caption, `${name}: caption ${step}`);
    assert.equal(/\bhidden\b/.test(caption[1]), step !== story.steps.length - 1, `${name}: one readable fallback caption`);
    assert.ok(story.steps[step].title && story.steps[step].description, `${name}: text alternative for step ${step}`);
    steps++;
  }
  for (const variant of ['desktop', 'mobile']) {
    assert.match(markup, new RegExp(`<svg class="scene-${variant}[^>]*aria-hidden="true"`), `${name}: dedicated ${variant} layout`);
  }
  for (const state of markup.matchAll(/data-show="([^"]*)"/g)) {
    for (const index of state[1].split(' ').filter(Boolean)) {
      assert.ok(Number.isInteger(Number(index)) && Number(index) >= 0 && Number(index) < story.steps.length, `${name}: invalid visual state ${index}`);
    }
  }
  assert.match(markup, /data-draw="/, `${name}: authored line construction, not only scene fades`);
  assert.match(markup, /data-(transit|shift|offset)="/, `${name}: authored spatial motion`);
  for (const track of markup.matchAll(/data-(draw|transit|shift)="([^"]*)"/g)) {
    const values = track[2].split(/\s+/).map(Number);
    assert.equal(values.length, track[1] === 'shift' ? 4 : 2, `${name}: valid ${track[1]} track`);
    assert.ok(values.every(Number.isFinite), `${name}: finite motion coordinates`);
    assert.ok(values[0] >= 0 && values[0] < values[1] && values[1] <= story.steps.length, `${name}: motion inside timeline`);
  }
  assert.equal([...markup.matchAll(/data-transit="/g)].length, [...markup.matchAll(/data-route="/g)].length, `${name}: every traveling object has a route`);
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
  assert.equal(new Set(ids).size, ids.length, `${routes[name]}: no duplicate IDs`);
  for (const reference of markup.matchAll(/url\(#([^)]*)\)/g)) {
    assert.ok(ids.includes(reference[1]), `${name}: missing SVG marker ${reference[1]}`);
  }
  assert.doesNotMatch(markup, /<(animate|animateMotion|animateTransform|iframe|video)\b/, `${name}: all animation obeys the shared motion controls`);
}
console.log(`Checked ${Object.keys(stories).length} SVG explainers and ${steps} steps: motion tracks, responsive scenes, controls, static fallbacks, and SVG references are valid.`);
