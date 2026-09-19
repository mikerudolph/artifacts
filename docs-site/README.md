# Artifacts documentation

The documentation site is built with Astro Starlight. Content remains in the repository-level `docs/` directory; this project contains only the site configuration and presentation layer.

```bash
npm install
npm run dev
```

The local site is available at `http://127.0.0.1:4321/artifacts/`.

Create a production build with:

```bash
npm run build
```

Run `npm run check` to build and validate generated internal links, heading anchors, local assets (including stylesheets and fonts), and SVG explainer contracts. Builds refresh Astro's content cache so a highlighting-theme change cannot leave Markdown pointing to obsolete generated code stylesheets. Use `npm run preview` to inspect production search; Pagefind is not available in the development server.

## Content and navigation

`src/navigation.mjs` defines the three sections and the order of the reading path. It supplies both Starlight's pagination and the custom section-aware navigation. Keep the greatest depth in Core: concepts, data modeling, integration, API behavior, and operations. Examples demonstrate application patterns; Developer tools covers local inspection and development.

Use one title in front matter and start the body at heading level two. The page header renders the title and description. Use `/artifacts/.../` links for published routes, and check links after adding or moving pages. The legacy `/onboarding/` route redirects to the agent sessions example. The existing `/getting-started/` and `/storage/` routes remain stable.

Describe the current implementation. Check request/response types, mounted routes, and tests before documenting behavior. Distinguish application schemas from Artifacts resources, raw file responses from JSON envelopes, and REST snapshot replacement from incremental Git edits. Include prerequisites and observable outcomes in runnable tutorials.

## Presentation

The **Orbital** visual system draws on the angular interfaces, condensed typography, and atmospheric depth of modern space games, with [EVE Online](https://www.eveonline.com/) as the visual reference. Artifacts keeps its own stack mark and Tokyo Night blue, cyan, and lilac palette. The field-manual styling extends across the navigation, reading pages, code panels, animation players, 404, and social preview. Both dark and light themes remain supported.

`src/styles/custom.css` owns the base reading layout; `src/styles/orbital.css`, loaded afterward, owns the brand tokens and styling. Starlight component overrides live in `src/components/`. Display typography is Barlow Semi Condensed, self-hosted through `@fontsource/barlow-semi-condensed` under the SIL Open Font License. Only the Latin 500/600 weights are shipped; there are no remote font requests. Body copy and code retain dedicated reading fonts.

`AgentWorkflow.astro` is an original, code-native SVG illustration: an application reads `plan.md` from the repository, performs work in its own runtime, and returns revised `report.md` and `state.json` files. A new immutable snapshot joins the Artifacts stack; the branch and changed-file hashes advance together while the unchanged plan and earlier snapshot remain available. Example hashes are illustrative, not live service telemetry. The next 18-second run starts from the snapshot just published, with no history reset. Separate wide/narrow compositions share one animation-frame clock in `repository-workflow.ts`, backed by the pure choreography in `repository-workflow-state.ts`. Playback starts in view and suspends offscreen or in hidden tabs. There is no alternate motion mode, graphics library, stock art, or copied game asset. `repository-workflow.css` owns this illustration's styling. Keep the long-form text backgrounds quiet; navigation, search, mobile menu, code copying, and page anchors must continue working when changing the shell.

The mark is a native SVG in `BrandMark.astro`, with a matching favicon. `public/social-card.svg` is the editable source for the 1200 × 630 social preview. Regenerate its PNG after changing the brand:

```bash
npm run brand:render
```

See [CONTENT-REVIEW.md](CONTENT-REVIEW.md) for the onboarding evaluation and known product boundaries reflected in the docs.

## Animated explainers

Four native SVG explainers live in `src/components/diagrams/`: Git object graphs, snapshot forks, publication ordering, and cache reconstruction. The three articles embedding them use MDX, with unchanged public routes. Each diagram has a dedicated narrow layout, not a shrunken desktop canvas.

`stories.ts` owns chapter labels and captions. `Diagram.astro` provides the shared player and transcript; `src/scripts/diagrams.ts` connects visibility and playback to the pure clock in `diagram-timeline.ts`. `diagram-motion.ts` renders native SVG transforms, path construction, staggered arrivals, and traveling objects from that same clock on every animation frame. Geometry is sampled once, not measured repeatedly during playback. Styling uses existing Tokyo Night tokens in `src/styles/diagrams.css`. There are no client-side animation dependencies, external assets, or raster frames.

Each explainer automatically plays through while its drawing is visible. Chapters take 4.4 seconds; the final chapter adds 2.2 seconds to let the result settle, including a brief fade into the next loop. Full spatial motion is always enabled, with no alternate motion mode or media-query gate. Offscreen drawings and hidden tabs suspend work. Pause freezes every object, connector, and progress indicator at the same instant; seeking preserves the play/pause choice. Responsive layout changes retain the current pose even while paused. Without JavaScript, the final diagram and full transcript remain available.

Author motion directly on SVG elements: `data-show` lists persistent chapters, `data-enter`/`data-span` stagger arrivals, `data-offset` and `data-scale` describe entry poses, `data-draw` progressively draws a stroke, `data-shift` moves a persistent object, and `data-transit`/`data-route` send an object along a path. Times are chapter coordinates: `1.2` means 20% into chapter two. `Transit.astro` is the shared traveling file tile. Keep captions readable throughout movement, don't overlap labels with flight paths, and retain objects across adjacent chapters instead of repeatedly fading the whole scene.

Use each story once per page so its labels and SVG marker IDs remain unique. After edits, run `npm run check`, then inspect intermediate poses as well as completed chapters at desktop and phone widths in both themes. Verify a complete automatic loop without clicks, actual frame-by-frame movement, pause/resume, seeking, responsive switching, viewport suspension, and JavaScript-disabled rendering. The timeline and motion tests cover looping, pause, seeking, easing continuity, staggered arrivals, persistent objects, and the final hold; browser checks are still needed for rendering and visual quality.
