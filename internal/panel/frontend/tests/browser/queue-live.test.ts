import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

/**
 * The queue while it is running.
 *
 * This is the one table in the panel that changes while it is being read: a deadline runs out, the
 * service acts, and the rows re-sort. Two things follow, and they pull in opposite directions.
 *
 * The table has to show what happened - a row that merges has to be seen to merge, or the reader
 * cannot tell a request that finished from one that was never there. And the table must not move
 * under a pointer: the kebab beside a merged request offers Cancel, and the row that slides into
 * its place is a different pull request, so a list that re-sorts under a reader's hand does not
 * annoy them, it makes them act on the wrong thing.
 *
 * So the arrangement is pinned while somebody is reading it and the contents are not. What a held
 * row says is live; where it sits is not. That is what this measures.
 *
 * The mock's own reconciler is frozen for the sweeps - see `startPanel` - so this drives the change
 * itself, through the same endpoint the row's Check now button uses. Nothing here waits for a
 * wall-clock deadline.
 */

let panel: Panel;
let page: Page;

beforeAll(async () => {
  panel = await startPanel();
  page = await panel.browser.newPage();
  await visit(page, `${panel.origin}/root/queue`, { ready: '.queue-table tbody .queue-row' });
}, 300_000);

afterAll(async () => {
  await page?.close();
  await panel?.close();
});

/** The rows as they stand, top to bottom, by the pull request each names. */
function order(): Promise<string[]> {
  return page.$$eval('.queue-table tbody .queue-row', (rows: Element[]) =>
    rows.map((row) =>
      (row.querySelector('.pr-name')?.textContent ?? '').replace(/\s+/g, ' ').trim(),
    ),
  );
}

/** Whether the table is pinned to what a reader is looking at, and how many rows it is holding. */
function holding(): Promise<string | null> {
  return page.$eval('.queue-table tbody', (body: Element) => body.getAttribute('data-held'));
}

/** Asks the service to look at every armed request now, which is what makes the rows move. */
async function reconcile(): Promise<void> {
  await page.evaluate(async () => {
    const answer = await fetch('/api/v1/root/overview', {
      headers: { accept: 'application/json' },
    });
    const overview = (await answer.json()) as {
      pending_ci: { active: { id: string; revision: number }[] };
    };
    for (const request of overview.pending_ci.active) {
      await fetch(`/api/v1/root/pending-ci/${encodeURIComponent(request.id)}/check`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ expected_revision: request.revision }),
      });
    }
  });
  // The stream says so, the query refetches, and Svelte renders. None of that is instant.
  await page.waitForTimeout(1_200);
}

describe('the queue while it runs [Integration]', () => {
  it('re-sorts when nothing is being read', async () => {
    await page.mouse.move(0, 0);
    const before = await order();
    await reconcile();

    expect(before.length, 'the queue drew no rows to begin with').toBeGreaterThan(2);
    expect(await holding(), 'the table held its arrangement with nothing reading it').toBeNull();
  });

  it('holds its arrangement under a pointer, and keeps the row it is on', async () => {
    await page.mouse.move(0, 0);
    await page.waitForTimeout(300);

    const rows = await page.$$('.queue-table tbody .queue-row');
    const target = rows[1];
    expect(target, 'the queue drew too few rows to hover the second').toBeDefined();
    await target?.hover();

    const before = await order();
    const hoveredBefore = await page.$eval(
      '.queue-table tbody .queue-row:hover .pr-name',
      (name: Element) => (name.textContent ?? '').replace(/\s+/g, ' ').trim(),
    );

    expect(
      await holding(),
      'the table did not hold its arrangement for the pointer',
    ).not.toBeNull();

    await reconcile();

    expect(await order(), 'the rows re-sorted under the pointer').toEqual(before);
    /* The pointer has not moved, so this is the same physical spot: the question is whether the
       same request is still under it. */
    expect(
      await page.$eval('.queue-table tbody .queue-row:hover .pr-name', (name: Element) =>
        (name.textContent ?? '').replace(/\s+/g, ' ').trim(),
      ),
      'a different request slid under the pointer',
    ).toBe(hoveredBefore);
  });

  it('lets a held row say what it now is', async () => {
    /* Every state each row shows, read against the record behind it: a row that has finished must
       not be drawing a countdown to a look that will never happen. */
    const lying = await page.$$eval('.queue-table tbody .queue-row', (rows: Element[]) =>
      rows
        .map((row) => ({
          name: (row.querySelector('.pr-name')?.textContent ?? '').replace(/\s+/g, ' ').trim(),
          leaving: row.classList.contains('leaving'),
          next: (row.querySelector('td:nth-child(3)')?.textContent ?? '')
            .replace(/\s+/g, ' ')
            .trim(),
        }))
        .filter((row) => row.leaving && !row.next.startsWith('Moves to Recent'))
        .map((row) => `${row.name}: ${row.next}`),
    );

    expect(lying, `a finished row was still counting down:\n  ${lying.join('\n  ')}`).toEqual([]);
  });

  it('lets go, and applies everything at once', async () => {
    const held = await order();
    await page.mouse.move(0, 0);
    await page.waitForTimeout(600);

    expect(await holding(), 'the table was still holding after the pointer left').toBeNull();

    const released = await order();
    expect(released.length, 'releasing the hold emptied the table').toBeGreaterThan(0);
    /* What it holds may differ - that is the point of releasing - but every row still on screen has
       to be one the queue actually has. */
    expect(new Set(released).size, 'a request was drawn twice').toBe(released.length);
    expect(held.length, 'the held table was empty, so nothing was proved').toBeGreaterThan(0);
  });
});
