import { copyFile, mkdir } from 'node:fs/promises';

const output = new URL('../public/licenses/', import.meta.url);
await mkdir(output, { recursive: true });
await Promise.all([
  copyFile(new URL('../../LICENSE', import.meta.url), new URL('artifacts-MIT.txt', output)),
  copyFile(new URL('../node_modules/@fontsource/barlow-semi-condensed/LICENSE', import.meta.url), new URL('barlow-semi-condensed-OFL.txt', output)),
]);
