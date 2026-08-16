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
import type { Browser } from 'playwright-core';
import { chromium } from 'playwright-core';
import { createServer, type ViteDevServer } from 'vite';

/** Long enough for a route to load its data. */
export const SETTLE_MS = 1500;

export interface Panel {
  origin: string;
  browser: Browser;
  /** The workspace the mock seeded, read from where signing in landed rather than named. */
  account: string;
  close: () => Promise<void>;
}

export async function startPanel(): Promise<Panel> {
  process.env.SMYKLOT_PANEL_DEV_MOCK = '1';
  // Bound to the address the browser is told to use. Vite's default host resolves to the IPv6
  // loopback on some machines and the IPv4 one on others, and it reports the same port either way,
  // so naming one is the difference between a measurement and a connection refused.
  const server: ViteDevServer = await createServer({
    logLevel: 'error',
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

/** Signs in against the mock and reports the workspace it landed on. */
async function signIn(browser: Browser, origin: string): Promise<string> {
  const page = await browser.newPage();
  try {
    await page.goto(`${origin}/?scenario=default`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(SETTLE_MS);
    const landing = new URL(page.url()).pathname;
    const match = /^\/i\/([^/]+)\//u.exec(landing);
    if (match?.[1] === undefined) {
      throw new Error(`signing in did not land on a workspace, it landed on ${landing}`);
    }

    return match[1];
  } finally {
    await page.close();
  }
}
