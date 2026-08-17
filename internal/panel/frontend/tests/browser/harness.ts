/**
 * The panel, served by vite and driven in Chrome.
 *
 * Shared by every check in this directory, because the two fiddly parts are the ones worth having
 * a single copy of: which address vite is told to bind, and how the seeded workspace is found.
 *
 * Chrome is driven through `channel`, so nothing is downloaded: the runner image ships it and so
 * does a developer's machine. There is no skip if it is missing - a guard that stands down when it
 * cannot run is not a guard.
 */
import type { Browser, Page } from 'playwright-core';
import { chromium } from 'playwright-core';
import { createServer, defaultClientConditions, type ViteDevServer } from 'vite';

/** Long enough for a route to load its data. */
export const SETTLE_MS = 1500;

export interface Panel {
  origin: string;
  browser: Browser;
  /** The workspace the mock seeded, read from where signing in landed rather than named. */
  account: string;
  close: () => Promise<void>;
}

/**
 * Every shape of page the panel has, addressed the way its own router spells it: a workspace view
 * under `/i/<account>`, the console under `/root`, and the inbox under neither, since it belongs
 * to the reader.
 *
 * Here rather than in one of the files that walks it, because more than one does and a second copy
 * is a list that goes stale: a route added to one sweep and not the other is a page that looks
 * checked and is not.
 */
export const PANEL_ROUTES = [
  'i/settings',
  'i/repositories',
  /* One repository's own page, which is a route in its own right and was in none
     of these sweeps: it has a header, a switch, a way back and three panes, and
     every rule the others are held to applies to it too. */
  'i/repositories/api-gateway',
  'i/sync',
  'i/users',
  'i/invitations',
  'i/history',
  'root/settings',
  'root/queue',
  'root/queue/recent',
  'root/queue/request/pending-ci-0',
  'root/installations',
  'root/access/users',
  'root/access/invitations',
  'root/history/audit',
  'inbox',
] as const;

/** One of those routes as an address, filling in the workspace that signing in found. */
export function addressOf(panel: Panel, route: string): string {
  return route.startsWith('i/')
    ? `${panel.origin}/i/${panel.account}/${route.slice(2)}`
    : `${panel.origin}/${route}`;
}

export async function startPanel(): Promise<Panel> {
  process.env.SMYKLOT_PANEL_DEV_MOCK = '1';
  /* The mock's queue runs its own reconciler, so that a deadline in development expires the way it
     expires in production and the merge can be watched. A sweep that measures a table cannot also
     have the table re-sort itself half way through the measurement, so it asks for the queue to
     hold still. Nothing else about the mock changes. */
  process.env.SMYKLOT_PANEL_DEV_MOCK_FROZEN = '1';
  // Bound to the address the browser is told to use. Vite's default host resolves to the IPv6
  // loopback on some machines and the IPv4 one on others, and it reports the same port either way,
  // so naming one is the difference between a measurement and a connection refused.
  const server: ViteDevServer = await createServer({
    logLevel: 'error',
    // This server runs inside a Vitest worker. Testing Library replaces Vite's
    // client conditions there, which would otherwise resolve Svelte's default
    // (server) entry in the real browser and fail before the panel mounts.
    resolve: { conditions: [...defaultClientConditions] },
    server: { host: '127.0.0.1', port: 0 },
  });
  let browser: Browser | undefined;

  /* Everything from here is torn down on the way out, because a setup that throws half way through
     otherwise leaves a vite server and a Chrome behind holding the process open: the failure that
     is worth reading arrives as a run that never exits rather than as a message. `afterAll` cannot
     do it - there is nothing to hand it until this returns. */
  try {
    await server.listen();

    const address = server.httpServer?.address();
    if (address === null || address === undefined || typeof address === 'string') {
      throw new Error('the dev server did not report a port');
    }
    const origin = `http://127.0.0.1:${address.port}`;

    const launched = await chromium.launch({ channel: 'chrome' });
    browser = launched;

    return {
      origin,
      browser: launched,
      account: await signIn(launched, origin),
      close: async () => {
        await launched.close();
        await server.close();
      },
    };
  } catch (failure) {
    /* Settled rather than awaited in turn, and the original failure is what leaves. A `close()`
       that rejects - Chrome already gone, which is how a machine under pressure loses it - would
       otherwise skip the server behind it and leave the leak this exists to prevent, then report
       itself in place of the failure worth reading. */
    await Promise.allSettled([browser?.close(), server.close()]);
    throw failure;
  }
}

/**
 * Opens a route and waits for it to be finished rather than for a fixed length of time.
 *
 * Every sweep in this directory used to `goto` and then sleep `SETTLE_MS`, which is two guesses at
 * once: too long for the routes that are ready in a third of it - and every file here walks ten to
 * forty routes, so that guess is most of what this suite costs - and too short for whichever route
 * is slowest on a loaded machine, where the reward for guessing wrong is a measurement taken of a
 * page that had not drawn yet.
 *
 * What the sweeps are actually waiting for is the route's data, so that is what this waits for.
 * Three things have to be true, and they fail differently:
 *
 *  - The panel has to mount before it can ask for anything, and between `DOMContentLoaded` and the
 *    first request there is a gap that looks exactly like a settled page. So the first API call is
 *    the starting gun, and it is given a long deadline: on a cold dev server the client chunks are
 *    still being compiled, and that is slow rather than broken. A caller measuring one particular
 *    thing can name it as `ready` instead, which is a stronger gun than any request.
 *  - Nothing may have been in flight for `QUIET_MS`. A response that arrives late restarts the
 *    window, which is what makes this cover a route that loads a list and then a page of it.
 *  - Nothing on the page may still say it is loading. Quiet alone is not enough and CI proved it:
 *    a view whose query has resolved but whose next one has not yet been issued is quiet, and
 *    `i/users` was measured in exactly that gap - the table header it renders was still a skeleton,
 *    and the run reported the panel drawing no table header at all. The panel announces the state
 *    itself, in `aria-busy` and in the skeleton it puts up in place of rows, so that announcement
 *    is what gets read rather than a longer guess at how big the gap might be.
 *
 * None of them throws on running out of time. The measurement is taken anyway, and every file here
 * already states its own precondition - a heading, a table header, five rows - which says far more
 * about what went wrong than a navigation timeout does.
 */
export function visit(page: Page, url: string, options: Settle = {}): Promise<void> {
  return settle(
    page,
    async () => void (await page.goto(url, { waitUntil: 'domcontentloaded' })),
    options,
  );
}

/**
 * The same wait, around something other than a page load.
 *
 * Entering a route by pressing a link is the arrangement two files here need - it mounts on top of
 * what is already there, which is where both of the request loops that shipped lived - and it is a
 * navigation the browser never reports. The watchers have to be attached before the press either
 * way, so the press is handed over rather than made first.
 */
export async function settle(
  page: Page,
  act: () => Promise<void>,
  options: Settle = {},
): Promise<void> {
  const quiet = options.quiet ?? QUIET_MS;
  const ceiling = options.ceiling ?? QUIET_CEILING_MS;

  let inFlight = 0;
  let mounted = false;
  let last = Date.now();

  const asked = (request: { url: () => string }): void => {
    inFlight += 1;
    last = Date.now();
    if (request.url().includes('/api/')) mounted = true;
  };
  const answered = (): void => {
    inFlight = Math.max(0, inFlight - 1);
    last = Date.now();
  };

  page.on('request', asked);
  page.on('requestfinished', answered);
  page.on('requestfailed', answered);

  try {
    await act();

    const mountBy = Date.now() + (options.mount ?? MOUNT_MS);
    if (options.ready === undefined) {
      while (!mounted && Date.now() < mountBy) await page.waitForTimeout(POLL_MS);
    } else {
      await page
        .locator(options.ready)
        .first()
        .waitFor({ state: 'attached', timeout: mountBy - Date.now() })
        .catch(() => {
          // Reported by the caller's own precondition, which names the route as well as the thing.
        });
    }

    const settleBy = Date.now() + ceiling;
    while (Date.now() < settleBy) {
      // The page is asked only once the network has gone quiet, so a lane costs one round trip to
      // settle rather than one every `POLL_MS`.
      if (inFlight === 0 && Date.now() - last >= quiet && (await drawn(page))) break;
      await page.waitForTimeout(POLL_MS);
    }

    /* Text is measured cap-to-baseline in more than one file here, and a fallback face has
       different metrics from the one the page ends up drawing. Cheap when the faces are already
       resolved, which after the wait above they almost always are. */
    await page.evaluate(async () => {
      await document.fonts.ready;
    });
  } finally {
    page.off('request', asked);
    page.off('requestfinished', answered);
    page.off('requestfailed', answered);
  }
}

/** Whether the panel has stopped saying it is loading. */
function drawn(page: Page): Promise<boolean> {
  return page.evaluate(
    () =>
      document.querySelector('[aria-busy="true"], [class*="skeleton"]') === null &&
      // A view that draws no skeleton says the same thing by rendering nothing yet.
      document.querySelector('main, [role="main"]') !== null,
  );
}

export interface Settle {
  /**
   * What the caller is about to measure, as a selector. A far better starting gun than the first
   * API call when there is one to name: the request only says the panel asked.
   */
  ready?: string;
  /** How long nothing may be in flight before the route counts as loaded. */
  quiet?: number;
  /** Longest to wait for that quiet before measuring anyway. */
  ceiling?: number;
  /**
   * Longest to wait for the starting gun. The default is generous because a cold dev server
   * compiling for six lanes at once is slow rather than broken - but a route entered by pressing a
   * link has its modules already, and a route svelte-query answers from cache fires no request at
   * all, so a caller pressing links should name a short one rather than wait out this.
   */
  mount?: number;
}

const QUIET_MS = 250;
const QUIET_CEILING_MS = 8000;
const MOUNT_MS = 30_000;
const POLL_MS = 25;

/**
 * Measures many routes at once, each in its own page.
 *
 * A route measurement is a navigation and then a wait, and the wait is nearly all of it. Taken one
 * after another that wait is paid once per route; taken together it is paid once. `request-budget`
 * has always done this - it opens every address at the same time and says why: a count does not
 * change under load, only how long a page takes to reach it does. The same holds for everything
 * else measured here, because it is all geometry and computed style, and neither has a clock in
 * it. What load does change is the settling, and `visit` above waits for that rather than assuming
 * it, which is what makes running them together safe.
 *
 * Bounded rather than unbounded: the dev server compiling modules for thirty pages at once is the
 * one way this could make a route slower to settle than the ceiling allows.
 */
export async function inLanes<T, R>(
  items: readonly T[],
  measure: (item: T) => Promise<R>,
): Promise<R[]> {
  const results = Array.from({ length: items.length }) as R[];
  let next = 0;

  const lane = async (): Promise<void> => {
    for (;;) {
      const index = next++;
      if (index >= items.length) return;
      results[index] = await measure(items[index]!);
    }
  };

  await Promise.all(Array.from({ length: Math.min(LANES, items.length) }, lane));

  return results;
}

/**
 * How many pages are measured at once.
 *
 * Not derived from the core count, because cores are not what a lane spends its time on: a route
 * loads for a few hundred milliseconds and then waits, and it is the waiting that overlaps. Six is
 * where the return flattens on a four-core runner - past it the bound stops being the machine and
 * becomes the single thread serving the modules, and every lane simply queues behind it.
 *
 * `SMYKLOT_BROWSER_LANES=1` puts any of these files back to one page at a time, which is what to
 * reach for when a failure needs to be watched rather than reproduced.
 */
export const LANES = Math.max(1, Number.parseInt(process.env.SMYKLOT_BROWSER_LANES ?? '', 10) || 6);

/** Signs in against the mock and reports a workspace the viewer owns. */
async function signIn(browser: Browser, origin: string): Promise<string> {
  const page = await browser.newPage();
  try {
    await page.goto(`${origin}/?scenario=default`, { waitUntil: 'domcontentloaded' });
    try {
      await page.waitForURL(
        (url) =>
          /^\/i\/[^/]+\//u.test(url.pathname) ||
          url.pathname === '/root' ||
          url.pathname.startsWith('/root/'),
        { timeout: 30_000 },
      );
    } catch {
      // The error below reports the actual landing path, which is more useful
      // than Playwright's generic navigation timeout.
    }
    const landing = new URL(page.url()).pathname;
    const match = /^\/i\/([^/]+)\//u.exec(landing);
    if (match?.[1] !== undefined) return decodeURIComponent(match[1]);

    // Root viewers deliberately land in the application console instead of an
    // installation. Ask the same authenticated API the workspace switcher uses
    // rather than teaching this fixture the seeded account's name.
    if (landing === '/root' || landing.startsWith('/root/')) {
      const account = await page.evaluate(async () => {
        const response = await fetch('/api/v1/targets');
        if (!response.ok) throw new Error(`targets returned HTTP ${response.status}`);

        const body = (await response.json()) as {
          targets?: { account?: { login?: unknown } }[];
        };
        const login = body.targets?.[0]?.account?.login;
        return typeof login === 'string' && login.trim() !== '' ? login : null;
      });
      if (account !== null) return account;
    }

    throw new Error(`signing in did not expose a workspace, it landed on ${landing}`);
  } finally {
    await page.close();
  }
}
