import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { settle, startPanel, visit, type Panel } from './harness';

/**
 * Going back to a view the panel has already read costs nothing.
 *
 * The panel holds a WebSocket the service names every change on, and
 * `invalidateChange` turns each one into an invalidation of exactly the keys it
 * touched. Beside that it also held a 30-second staleness - a guess at how long
 * data stays true, standing in for knowing - so every view revisited after half
 * a minute of reading was fetched again to be told what the stream had already
 * promised: nothing had changed. Moving around the panel cost a request per view
 * per visit, for answers it was holding.
 *
 * So this walks away and comes back, and counts. The measurement is deliberately
 * of the SECOND visit only: the first is what fills the cache and is allowed
 * whatever it needs.
 *
 * What it found was not the staleness at all. The effect that owns the stream
 * read `session.isInvitation`, which reads the pathname, so every navigation
 * re-ran it, closed the socket and opened another - and a new socket answers
 * with `ready`, which is a full resync. Moving between two views refetched
 * everything on both: the panel was telling itself its data was stale because it
 * had just reconnected to say so. One new socket per navigation, measured.
 *
 * The other half of the decision - that losing the stream puts the clock back -
 * is in `tests/query-client.test.ts`, because proving it here would mean waiting
 * out the staleness in a real browser for every run.
 */

/** The views walked between, as the sidebar spells them. */
const FIRST = 'Repositories';
const SECOND = 'Defaults';

async function apiCallsOnReturn(page: Page, panel: Panel): Promise<string[]> {
  await visit(page, `${panel.origin}/i/${panel.account}/repositories`);
  // Away, so the first view's queries go inactive and its data is only in cache.
  await settle(page, () => page.getByRole('link', { name: SECOND, exact: true }).first().click(), {
    mount: 2_000,
  });

  const asked: string[] = [];
  const watch = (request: { url: () => string }): void => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith('/api/')) asked.push(path);
  };
  page.on('request', watch);
  try {
    await settle(page, () => page.getByRole('link', { name: FIRST, exact: true }).first().click(), {
      mount: 2_000,
    });
    // Long enough that anything the return kicked off has been sent.
    await page.waitForTimeout(1_000);
  } finally {
    page.off('request', watch);
  }

  return asked;
}

let panel: Panel;
let live: string[] = [];
let sockets = 0;

beforeAll(async () => {
  panel = await startPanel();

  const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
  try {
    // Counted from the page's own side: a socket opened and closed inside one
    // navigation leaves nothing for a request log to show.
    await page.addInitScript(() => {
      const original = window.WebSocket;
      let opened = 0;
      Object.defineProperty(window, '__panelSockets', { get: () => opened });
      window.WebSocket = new Proxy(original, {
        construct(target, args: ConstructorParameters<typeof WebSocket>) {
          /* The panel's, not vite's. The dev server holds a socket of its own
             for hot reloading and it opens first, so counting every socket on
             the page counted that one too and read two where the panel had
             opened one. */
          if (String(args[0]).includes('/api/v1/events')) opened += 1;
          return new target(...args);
        },
      });
    });
    live = await apiCallsOnReturn(page, panel);
    sockets = await page.evaluate(
      () => (window as unknown as { __panelSockets: number }).__panelSockets,
    );
  } finally {
    await page.close();
  }
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('coming back to a view already read [Integration]', () => {
  it('asks the server for nothing while the stream is up', () => {
    expect(live, `returning to ${FIRST} asked for:\n${live.join('\n')}`).toEqual([]);
  });

  it('holds one stream across the whole walk', () => {
    /* The cause, named rather than left to be inferred from the count above: a
       second socket here means the effect that owns it is being re-run by the
       route again, and every one of those is a resync of everything. */
    expect(sockets, 'the panel opened a new stream while navigating').toBe(1);
  });
});
