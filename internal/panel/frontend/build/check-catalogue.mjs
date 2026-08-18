/**
 * Loads every story in the built catalogue and fails on any that breaks.
 *
 * `storybook build` exits 0 on a bundle whose first module throws, and the static
 * check beside it only reads the output as text - so the catalogue could be
 * published as a spinner and nothing here would notice. This is what actually runs
 * it: the same page a reader opens, once per story.
 *
 * It catches the class of defect that everything else in this repository misses. A
 * story that renders nothing passes `eslint`, `svelte-check`, `prettier` and the
 * coverage ratchet alike - all four ask whether the file is right, and none of them
 * ask whether the component came out. Three separate bugs shipped that way: 31
 * stories throwing `toJSON was called from a story`, three rendering an empty box on
 * a duplicate key, and a whole set of decor stories drawing past the right edge of
 * the window.
 *
 * Three questions per story, and no more. Anything about how a component *looks*
 * belongs in `tests/browser/`, which drives the built app on its real routes and can
 * measure against it. This one only asks whether the story is there at all.
 */
import { createReadStream } from 'node:fs';
import { readFile, stat } from 'node:fs/promises';
import { createServer } from 'node:http';
import { extname, join, normalize } from 'node:path';

import { chromium } from 'playwright-core';

const root = process.argv[2];
if (root === undefined) {
  console.error('usage: check-catalogue.mjs <storybook-static>');
  process.exit(2);
}

const TYPES = new Map([
  ['.html', 'text/html'],
  ['.js', 'text/javascript'],
  ['.mjs', 'text/javascript'],
  ['.css', 'text/css'],
  ['.json', 'application/json'],
  ['.svg', 'image/svg+xml'],
  ['.png', 'image/png'],
  ['.woff2', 'font/woff2'],
]);

/* Served rather than opened from `file:`, because a module graph this size only
   loads over http - and because it is the shape the deploy publishes. */
const server = createServer((request, response) => {
  const path = decodeURIComponent(new URL(request.url ?? '/', 'http://localhost').pathname);
  const file = join(root, normalize(path).replace(/^(\.\.[/\\])+/u, ''));
  stat(file)
    .then((info) => {
      const target = info.isDirectory() ? join(file, 'index.html') : file;
      response.setHeader('Content-Type', TYPES.get(extname(target)) ?? 'application/octet-stream');
      createReadStream(target).pipe(response);
    })
    .catch(() => {
      response.statusCode = 404;
      response.end('not found');
    });
});

await new Promise((resolve) =>
  server.listen({ host: '127.0.0.1', port: 0 }, () => resolve(undefined)),
);
/* Port 0 asks the kernel for a free one, so the address is only knowable now - and
   it is a socket, never a pipe, which is the string half of this union. */
const address = server.address();
if (address === null || typeof address === 'string') {
  console.error('the catalogue server did not take a port');
  process.exit(1);
}
const base = `http://127.0.0.1:${address.port}`;

const index = JSON.parse(await readFile(join(root, 'index.json'), 'utf8'));
const ids = Object.keys(index.entries ?? {});
if (ids.length === 0) {
  console.error('the catalogue has no stories in it');
  process.exit(1);
}

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

let current = '';
/** @type {string[]} */
const broke = [];
/** @type {string[]} */
const wide = [];
page.on('pageerror', (error) => broke.push(`${current}: ${String(error).slice(0, 120)}`));

for (const id of ids) {
  current = id;
  await page.goto(`${base}/iframe.html?id=${encodeURIComponent(id)}&viewMode=story`, {
    waitUntil: 'load',
  });
  /* Long enough for a story's own effects to run and short enough to be worth
     running: a story that needs longer than this to draw anything is a story that
     needs longer than a reader will wait. */
  await page.waitForTimeout(140);
  const over = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  /* A pixel of slack for the rasteriser. A story that overflows does so by the width
     of whatever it laid out wrong, which is never one pixel. */
  if (over > 1) wide.push(`${id} runs ${over}px past the right of the window`);
}

await browser.close();
await new Promise((resolve) => server.close(resolve));

if (broke.length > 0 || wide.length > 0) {
  for (const line of [...broke, ...wide]) console.error(`  ${line}`);
  console.error(`\n${broke.length} of ${ids.length} stories threw, ${wide.length} overflowed.`);
  process.exit(1);
}

console.log(`catalogue checked: ${ids.length} stories, none throwing, none overflowing`);
