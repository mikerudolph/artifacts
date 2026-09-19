import { readdir, readFile, stat } from 'node:fs/promises';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../dist/', import.meta.url));
const base = '/artifacts/';
const origin = 'https://mikerudolph.github.io';
async function walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = await Promise.all(entries.map(entry => entry.isDirectory()
    ? walk(join(dir, entry.name)) : [join(dir, entry.name)]));
  return files.flat();
}
const files = await walk(root);
const pages = files.filter(path => path.endsWith('.html'));
const documents = new Map(await Promise.all(pages.map(async path => [path, await readFile(path, 'utf8')])));
const failures = [];
let checked = 0;
const checkedAssets = new Set();
async function checkAsset(href, route, name) {
  if (href.startsWith('#')) return;
  const url = new URL(href.replaceAll('&amp;', '&'), `${origin}${route}`);
  if (url.origin !== origin || !['https:', 'http:'].includes(url.protocol)) return;
  if (!url.pathname.startsWith(base)) {
    failures.push(`${name}: asset escapes deployment base: ${href}`);
    return;
  }
  const target = join(root, decodeURIComponent(url.pathname.slice(base.length)));
  try {
    if (!(await stat(target)).isFile()) throw new Error('Expected a file');
    checkedAssets.add(target);
  } catch { failures.push(`${name}: missing asset ${href}`); }
}
for (const [path, html] of documents) {
  const name = relative(root, path);
  const route = base + name.replace(/index\.html$/, '');
  if (!/http-equiv="refresh"/i.test(html) && [...html.matchAll(/<h1\b/gi)].length !== 1) {
    failures.push(`${name}: expected one page title`);
  }
  for (const match of html.matchAll(/<a\b[^>]*\bhref=(['"])(.*?)\1/gi)) {
    const href = match[2].replaceAll('&amp;', '&');
    const url = new URL(href, `${origin}${route}`);
    if (url.origin !== origin || !['https:', 'http:'].includes(url.protocol)) continue;
    if (!url.pathname.startsWith(base)) {
      failures.push(`${name}: link escapes deployment base: ${href}`);
      continue;
    }
    let target = join(root, decodeURIComponent(url.pathname.slice(base.length)));
    try {
      if ((await stat(target)).isDirectory()) target = join(target, 'index.html');
      await stat(target);
    } catch {
      failures.push(`${name}: missing target ${href}`);
      continue;
    }
    if (url.hash && documents.has(target)) {
      const fragment = decodeURIComponent(url.hash.slice(1));
      const ids = [...documents.get(target).matchAll(/\bid=(['"])(.*?)\1/g)].map(match => match[2]);
      if (!ids.includes(fragment)) failures.push(`${name}: missing anchor ${href}`);
    }
    checked++;
  }
  // Content caching can retain an old generated code stylesheet even when the
  // page and its navigation links build successfully. Check resources as well.
  for (const tag of html.matchAll(/<(script|link|img|source)\b[^>]*>/gi)) {
    if (tag[1].toLowerCase() === 'link') {
      const rel = tag[0].match(/\brel=(['"])(.*?)\1/i)?.[2] ?? '';
      if (!/\b(stylesheet|icon|preload|modulepreload)\b/.test(rel)) continue;
    }
    const attribute = tag[1].toLowerCase() === 'link' ? 'href' : 'src';
    const href = tag[0].match(new RegExp(`\\b${attribute}=(['"])(.*?)\\1`, 'i'))?.[2];
    if (href) await checkAsset(href, route, name);
  }
}
for (const path of files.filter(path => path.endsWith('.css'))) {
  const name = relative(root, path);
  const css = await readFile(path, 'utf8');
  for (const match of css.matchAll(/url\(\s*(['"]?)(.*?)\1\s*\)/g)) {
    await checkAsset(match[2], base + name, name);
  }
}
if (failures.length) {
  console.error(failures.join('\n'));
  process.exitCode = 1;
} else {
  console.log(`Checked ${pages.length} HTML pages, ${checked} internal links, and ${checkedAssets.size} local assets; all targets, anchors, stylesheets, fonts, and page titles are valid.`);
}
