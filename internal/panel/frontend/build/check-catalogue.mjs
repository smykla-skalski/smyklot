/**
 * Loads every story in the built catalogue and fails on any that breaks.
 *
 * `storybook build` exits 0 on a bundle whose first module throws, and the static
 * check beside it only reads the output as text - so the catalogue could be published
 * as a spinner and nothing here would notice. This is what actually runs it: the same
 * page a reader opens, once per story.
 *
 * It catches the class of defect that everything else in this repository misses. A
 * story that renders nothing passes `eslint`, `svelte-check`, `prettier` and the
 * coverage ratchet alike - all four ask whether the file is right, and none of them
 * ask whether the component came out. Three separate bugs shipped that way: 31
 * stories throwing `toJSON was called from a story`, three rendering an empty box on
 * a duplicate key, and a set of decor stories drawing past the right of the window.
 *
 * Three questions per story, and no more: did it throw, did it draw anything, does it
 * fit its window. Anything about how a component *looks* belongs in `tests/browser/`,
 * which drives the built app on its real routes and can measure against it.
 *
 * Accessibility is deliberately not here, and that is measured rather than assumed.
 * The a11y addon does not run in a bare `iframe.html` - it runs when the manager's
 * panel asks it to - so there is no result to read, and driving axe from here instead
 * put a second CPU-bound pass on every story: the run went from 20 seconds to eight
 * minutes and most of the passes timed out waiting for each other. Auditing the panel
 * for accessibility is a thing to do deliberately, not on the way to publishing.
 */
import { createReadStream } from 'node:fs';
import { readFile, stat } from 'node:fs/promises';
import { createServer } from 'node:http';
import { cpus } from 'node:os';
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

/* Served rather than opened from `file:`, because a module graph this size only loads
   over http - and because it is the shape the deploy publishes. */
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
/* Port 0 asks the kernel for a free one, so the address is only knowable now - and it
   is a socket, never a pipe, which is the string half of this union. */
const address = server.address();
if (address === null || typeof address === 'string') {
  console.error('the catalogue server did not take a port');
  process.exit(1);
}
const base = `http://127.0.0.1:${address.port}`;

const index = JSON.parse(await readFile(join(root, 'index.json'), 'utf8'));
const ids = Object.keys(index.entries ?? {});
/**
 * The stories whose whole point is that nothing is drawn - a receipt not showing, a
 * form with no error, a footer with nothing to report. Declared on the story with
 * `tags={['blank']}` rather than listed here, so the claim sits beside the state it
 * describes, and checked both ways below: a blank story that starts drawing is as
 * much a surprise as a drawing one that stops.
 */
const blank = new Set(ids.filter((id) => (index.entries[id]?.tags ?? []).includes('blank')));
if (ids.length === 0) {
  console.error('the catalogue has no stories in it');
  process.exit(1);
}

const browser = await chromium.launch({
  channel: 'chrome',
  /* Chrome throttles timers in pages it believes nobody is looking at, and every lane
     but one is exactly that. These are the standard three flags for saying otherwise;
     without them a lane's waits ran at a tick a second. */
  args: [
    '--disable-background-timer-throttling',
    '--disable-backgrounding-occluded-windows',
    '--disable-renderer-backgrounding',
  ],
});

/** @type {string[]} */
const broke = [];
/** @type {string[]} */
const bare = [];
/** @type {string[]} */
const filled = [];
/** @type {string[]} */
const wide = [];

/* Lanes take stories off one list. Four rather than one per core: measured from 2 to
   12 on a 14-core machine the run sits around forty seconds either way, because what
   it costs is booting the preview bundle, and past four they simply queue. */
const LANES = Number(process.env.CATALOGUE_LANES ?? '') || Math.min(4, cpus().length);
let next = 0;

await Promise.all(
  Array.from({ length: LANES }, async () => {
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    let current = '';
    page.on('pageerror', (error) => broke.push(`${current}: ${String(error).slice(0, 120)}`));

    /* One load per story. Switching the running preview from story to story instead is
       about twice as quick - twenty seconds against forty - and it is not worth it:
       measured over five runs it called four to six stories unrendered, a different
       four to six each time, because a preview asked to change story does not reliably
       say when it has. A check that cries wolf on a pull request is worse than a check
       that takes another twenty seconds. */
    while (next < ids.length) {
      const id = ids[next];
      next += 1;
      current = id;

      /* A Docs page is an entry like any other and can throw like any other, so it is
         opened too - but it is a different page. It answers to `viewMode=docs` and
         renders into `#storybook-docs`; asked for `viewMode=story` it draws nothing
         into a `#storybook-root` that never fills, so every one of them sat out the
         ten-second cap and then reported as unrendered. */
      const docs = index.entries[id]?.type === 'docs';
      const root = docs ? '#storybook-docs' : '#storybook-root';

      /* `domcontentloaded`, not `load`: the fonts and images a story pulls in do not
         decide whether it rendered, and waiting on them was most of the run. */
      await page.goto(
        `${base}/iframe.html?id=${encodeURIComponent(id)}&viewMode=${docs ? 'docs' : 'story'}`,
        { waitUntil: 'domcontentloaded' },
      );
      /* Waits for the story rather than for a guess at how long a story takes. A fixed
         settle was 140ms whether the component needed 8 or 300, which across this many
         stories was 46 seconds of sleeping. This returns the moment the root has
         something in it, and only waits out the cap for a story that never draws -
         which is the one case worth waiting for. */
      await page
        .waitForFunction(
          (selector) => (document.querySelector(selector)?.childElementCount ?? 0) > 0,
          root,
          { timeout: 10_000 },
        )
        .catch(() => undefined);

      /* One round trip for both measurements. Each `evaluate` is a crossing, and at
         this many stories that is not nothing. */
      const seen = await page.evaluate((docsPage) => {
        if (docsPage) {
          /* A Docs page carries no `.app-shell` of its own - the decorator runs inside
             each embedded story - so the question is only whether the page came out at
             all: a contract, a props table and the stories under them. */
          const page = document.querySelector('#storybook-docs');
          return {
            painted:
              page !== null &&
              [...page.querySelectorAll('*')].some(
                (node) => node.getBoundingClientRect().height > 1,
              ),
            over: document.documentElement.scrollWidth - window.innerWidth,
          };
        }
        const story = document.querySelector('#storybook-root');
        /* Below the decorator rather than at it. `PanelShell` is 48px tall with
           nothing inside it, so a root measured whole is painted whether or not the
           component came out - which is how a story passing `view="access"`, a value
           `InstallationView` has no branch for, sat in the catalogue drawing an empty
           frame and reporting as healthy.

           Descendants only, never the container's own box, which is what the shell
           and a backdrop have in common: a backdrop is positioned out of flow, so its
           parent measures zero while the sky it drew is 448px - a check reading the
           parent alone calls eight healthy stories empty. */
        const shell = story?.querySelector('.app-shell') ?? story;
        /* The shell's own two boxes are not the story. Everything else under it is,
           portalled overlays included: `Modal` is `<Dialog.Portal to=".app-shell">`,
           so a dialog's content is a sibling of the column rather than a child of
           it, and a check reading the column alone calls every dialog empty. */
        const chrome = new Set(
          [
            shell?.querySelector(':scope > .workspace'),
            shell?.querySelector(':scope > .workspace > .workspace-content'),
          ].filter((node) => node !== null && node !== undefined),
        );
        const painted =
          shell !== null &&
          [...shell.querySelectorAll('*')].some(
            (node) => !chrome.has(node) && node.getBoundingClientRect().height > 1,
          );
        return { painted, over: document.documentElement.scrollWidth - window.innerWidth };
      }, docs);

      if (!seen.painted && !blank.has(id)) bare.push(`${id} rendered nothing`);
      /* A Docs page collects every story of its component, blank ones included, so it
         draws whatever they draw plus its own prose. The tag belongs to the story. */
      if (seen.painted && blank.has(id) && !docs) {
        filled.push(`${id} is tagged blank and drew something; drop the tag`);
      }
      /* A pixel of slack for the rasteriser. A story that overflows does so by the
         width of whatever it laid out wrong, which is never one pixel. */
      if (seen.over > 1) wide.push(`${id} runs ${seen.over}px past the right of the window`);
    }

    await page.close();
  }),
);

await browser.close();
await new Promise((resolve) => server.close(() => resolve(undefined)));

const wrong = [...broke, ...bare, ...wide, ...filled];
if (wrong.length > 0) {
  for (const line of wrong) console.error(`  ${line}`);
  console.error(
    `\n${ids.length} entries: ${broke.length} threw, ${bare.length} drew nothing, ` +
      `${wide.length} overflowed, ${filled.length} drew while tagged blank.`,
  );
  process.exit(1);
}

const pages = ids.filter((id) => index.entries[id]?.type === 'docs').length;
console.log(
  `catalogue checked: ${ids.length - pages} stories and ${pages} docs pages, none throwing, ` +
    `${ids.length - pages - blank.size} drawing and ${blank.size} blank by declaration, ` +
    'none overflowing',
);
