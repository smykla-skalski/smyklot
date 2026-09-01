import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

/**
 * A table with nothing in it says so, in words the reader can see.
 *
 * Every table here already writes an empty state, and writing one is not the same as showing one.
 * The history tables filled theirs across the body it sits in - `position: absolute; inset: 0` -
 * and nothing gives that body a height when the rows are what is missing, so a search matching
 * nothing answered with a column header and a strip of background. The markup was right, the
 * element was in the page, and it measured zero.
 *
 * So this asks the question by doing what a reader does: type something nothing matches, and look.
 * Every route with a search field is walked, which is how a table added later is covered by
 * writing the same markup rather than by remembering.
 */

const NO_MATCH = 'zzzqqq-nothing-matches-this';

/** Tall enough to be an answer rather than a seam: two lines of copy and the room around them. */
const VISIBLE_HEIGHT = 40;

interface Verdict {
  route: string;
  rows: number;
  emptyHeight: number;
  text: string;
  /** The panel's opener - the words inside its `strong`. */
  opens: string;
  says: number;
}

async function emptyStateOn(page: Page): Promise<Verdict | null> {
  const search = page.locator('input[type="search"]').first();
  if ((await search.count()) === 0) return null;

  await search.fill(NO_MATCH);
  // The searches debounce, and the answer is a request.
  await page.waitForTimeout(1200);

  const verdict = await page.evaluate(() => {
    /* One selector, because there is one recipe. This used to name six - two of
       which no longer existed and two of which were a second and a third shape
       for the same answer. A list of accepted spellings is how a design system
       stops being one. */
    const empty = document.querySelector('.state-panel');
    const box = empty?.getBoundingClientRect();
    const opener = empty?.querySelector('strong');

    return {
      route: location.pathname,
      rows: document.querySelectorAll('tbody tr:not(.row-notice):not(.virtual-spacer)').length,
      emptyHeight: box?.height ?? 0,
      text: (empty?.textContent ?? '').trim().slice(0, 60),
      /* The shape, not just the presence: a state names what happened in an
         opener and then says what to do about it. Panels that were only half of
         that read as a stray line rather than as an answer. */
      opens: (opener?.textContent ?? '').trim(),
      says: (empty?.textContent ?? '').trim().length,
    };
  });

  /* Put back, because a table's search is a synced preference and the mock keeps them in a file
     that outlives this process. Left set, every other sweep in this directory opens on a table
     filtered to nothing and measures an empty state instead of the page it came to look at. */
  await search.fill('');
  await page.waitForTimeout(600);

  return verdict;
}

let panel: Panel;
let verdicts: Verdict[] = [];

beforeAll(async () => {
  panel = await startPanel();

  const readings = await inLanes(PANEL_ROUTES, async (route) => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, route));

      return { route, verdict: await emptyStateOn(page) };
    } finally {
      await page.close();
    }
  });

  verdicts = readings
    .filter((reading) => reading.verdict !== null)
    .map((reading) => ({ ...(reading.verdict as Verdict), route: reading.route }));
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('the tables with nothing to show [Integration]', () => {
  it('found searchable tables to empty', () => {
    // Every route answering "no search field here" is indistinguishable from every route
    // answering nothing at all, and this file would pass by looking at none of them.
    expect(
      verdicts.map((one) => one.route),
      'no route offered a search field',
    ).not.toEqual([]);
  });

  it('say so where the rows would have been', () => {
    const silent = verdicts.filter((one) => one.rows === 0 && one.emptyHeight < VISIBLE_HEIGHT);

    expect(
      silent.map((one) => one.route),
      `these answered a search that matches nothing with no visible empty state:\n${silent
        .map((one) => `  ${one.route}  ${one.emptyHeight.toFixed(1)}px  "${one.text}"`)
        .join('\n')}`,
    ).toEqual([]);
  });

  /**
   * The panel names the state and then says something about it.
   *
   * Both halves, because either alone is the shape this sweep exists to refuse: an
   * opener with nothing after it is a headline where an answer should be, and a
   * sentence with no opener is the stray line four of these were before the one
   * recipe reached them.
   */
  it('name the state and then say what follows from it', () => {
    const shown = verdicts.filter((one) => one.rows === 0 && one.emptyHeight >= VISIBLE_HEIGHT);
    const bare = shown.filter((one) => one.opens === '' || one.says <= one.opens.length + 1);

    expect(
      bare.map((one) => one.route),
      `these said too little to be an answer:\n${bare
        .map((one) => `  ${one.route}  opener "${one.opens}"  whole "${one.text}"`)
        .join('\n')}`,
    ).toEqual([]);
  });
});
