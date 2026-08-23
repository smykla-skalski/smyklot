/**
 * What the panel is allowed to ask the server for, counted in a real browser.
 *
 * `tests/effect-cycles` keeps one shape out of the source and `src/lib/request-rate`
 * writes a warning into a console nobody is reading. Neither of them fails a build.
 * This does: it runs the panel against the dev mock, walks every address, and
 * counts. A reactive loop reaches the budget below inside a tenth of a second,
 * which is why the two that shipped were invisible - the page rendered, nothing
 * flashed, and the only sign was a request log going past too fast to read.
 *
 * It is a browser rather than a mounted component on purpose. Both loops closed
 * through the network, and both needed a route, a layout and a mounted account
 * menu to exist at all - the notification inbox was asking from every page of the
 * panel because it hung off a menu that was on every page.
 */
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { SETTLE_MS, settle, startPanel, type Panel } from './harness';

/**
 * How many times one address may be asked for while a page settles.
 *
 * The panel asks twice at most, and only because a reload resolves the session
 * and then the workspace. Six leaves room for a page that legitimately loads a
 * list and then a page of it, and is still four times under the 25 that
 * `request-rate` reports at runtime - which is itself far under a loop.
 */
const BUDGET = 6;

/** Nothing in the panel polls over HTTP, so an idle panel asks for nothing. */
const IDLE_MS = 2000;

/** `{account}` is whichever workspace the mock seeded, read rather than named. */
const ADDRESSES = [
  '/i/{account}/defaults',
  '/i/{account}/repositories',
  '/i/{account}/access/users',
  '/i/{account}/history/audit',
  '/inbox',
  '/root',
  '/root/installations',
  '/root/access/users',
  '/root/history/audit',
  '/root/runtime/service',
  '/root/runtime/database',
  '/root/runtime/settings',
] as const;

interface Measurement {
  path: string;
  /** The address asked for most often, and how often. */
  busiest: [string, number] | null;
  addresses: number;
  floods: string[];
  crashes: string[];
}

let panel: Panel;
const measured = new Map<string, Measurement>();
let walked: Measurement;

beforeAll(async () => {
  panel = await startPanel();

  // Measured together rather than one after another, and the walk alongside
  // them. A count does not change under load - only how long a page takes to
  // reach it does, which is what the "asked for something" checks are for.
  await Promise.all([
    ...ADDRESSES.map(async (template) => {
      measured.set(template, await measure(template.replace('{account}', panel.account)));
    }),
    walk().then((result) => {
      walked = result;
    }),
  ]);
});

afterAll(async () => {
  await panel?.close();
});

async function measure(path: string): Promise<Measurement> {
  const page = await panel.browser.newPage();
  const watcher = watch(page);
  try {
    const mounted = page.waitForRequest((request) => request.url().includes('/api/'), {
      timeout: 30_000,
    });
    await page.goto(`${panel.origin}${path}`, { waitUntil: 'domcontentloaded' });
    // `DOMContentLoaded` only says Vite served the shell. On a cold run its client
    // chunks can still be compiling, especially while all routes start together.
    // Begin the settle window when the mounted panel first reaches its API.
    await mounted;
    await page.waitForTimeout(SETTLE_MS);

    return {
      path,
      busiest: busiest(watcher.counts),
      addresses: watcher.counts.size,
      floods: watcher.floods,
      crashes: watcher.crashes,
    };
  } finally {
    await page.close();
  }
}

/**
 * Walks the panel client-side and then measures the silence.
 *
 * Entering a route rather than loading it mounts on top of what is already
 * there, which is the arrangement both of the loops that shipped needed - the
 * inbox was asking from every page because it hung off a menu on every page.
 * Nothing in the panel polls over HTTP, so once the walk has settled the count
 * must stop moving entirely.
 */
async function walk(): Promise<Measurement> {
  const page = await panel.browser.newPage();
  const watcher = watch(page);
  try {
    const mounted = page.waitForRequest((request) => request.url().includes('/api/'), {
      timeout: 30_000,
    });
    await page.goto(`${panel.origin}/i/${panel.account}/defaults`, {
      waitUntil: 'domcontentloaded',
    });
    await mounted;
    await page.waitForTimeout(SETTLE_MS);

    for (const path of [
      `/i/${panel.account}/repositories`,
      `/i/${panel.account}/access/users`,
      '/inbox',
      '/root',
      '/root/runtime/service',
      '/root/runtime/settings',
    ]) {
      const link = page.locator(`a[href$="${path}"]`).first();
      if ((await link.count()) === 0) throw new Error(`nothing in the panel links to ${path}`);
      /* Waited out rather than slept through, unlike the windows above and below: nothing here is
         being measured yet - the counts are cleared once the walk is over - so what this needs is
         for the route to have arrived, and a route that takes longer than a flat second and a half
         to arrive was being walked past. A short deadline because the modules are already loaded
         by now, and because a route svelte-query answers from cache asks for nothing at all. */
      await settle(page, async () => link.click(), { mount: 5_000 });
    }

    const visited = watcher.counts.size;
    if (visited === 0) throw new Error('the walk asked for nothing, so it never loaded anything');

    // Everything above was the walk. Only what comes after it is measured.
    watcher.counts.clear();
    await page.waitForTimeout(IDLE_MS);

    return {
      path: 'a walk through the panel',
      busiest: busiest(watcher.counts),
      addresses: visited,
      floods: watcher.floods,
      crashes: watcher.crashes,
    };
  } finally {
    await page.close();
  }
}

/** Counts what a page asked the panel API for, keyed the way the client keys it. */
function watch(page: Page): { counts: Map<string, number>; floods: string[]; crashes: string[] } {
  const counts = new Map<string, number>();
  const floods: string[] = [];
  const crashes: string[] = [];

  page.on('request', (request) => {
    const url = request.url();
    if (!url.includes('/api/')) return;
    counts.set(url, (counts.get(url) ?? 0) + 1);
  });
  // The runtime guard writes here and nowhere else. Reading it is what makes it
  // a check rather than a note for whoever happens to open the console.
  page.on('console', (message) => {
    if (message.type() === 'error' && message.text().includes('[smyklot]')) {
      floods.push(message.text());
    }
  });
  page.on('pageerror', (error) => crashes.push(error.message));

  return { counts, floods, crashes };
}

function busiest(counts: Map<string, number>): [string, number] | null {
  let worst: [string, number] | null = null;
  for (const [url, count] of counts) {
    const address = new URL(url).pathname + new URL(url).search;
    if (worst === null || count > worst[1]) worst = [address, count];
  }

  return worst;
}

describe('the panel under a request budget', () => {
  it.each(ADDRESSES)('asks for nothing twice over on %s', (template) => {
    const result = measured.get(template);
    if (result === undefined) throw new Error(`${template} was never measured`);

    expect(result.crashes).toEqual([]);
    expect(result.floods).toEqual([]);
    // The precondition. A page that asked for nothing has not loaded yet, and a
    // budget nothing was measured against is a test that passes by not running.
    expect(
      result.addresses,
      `${result.path} asked the panel API for nothing, so nothing was measured`,
    ).toBeGreaterThan(0);
    // The address is named in the failure, because "some address was asked for
    // too often" is not something anybody can act on, and the address is the
    // whole diagnosis.
    expect(
      result.busiest !== null && result.busiest[1] <= BUDGET,
      `${result.path} asked for ${result.busiest?.[0]} ${result.busiest?.[1]} times, ` +
        `and the budget is ${BUDGET}`,
    ).toBe(true);
  });

  it('goes quiet once a walk through the panel has settled', () => {
    expect(walked.crashes).toEqual([]);
    expect(walked.floods).toEqual([]);
    expect(
      walked.busiest,
      `the panel was still asking for ${walked.busiest?.[0]} after it had settled`,
    ).toBeNull();
  });
});
