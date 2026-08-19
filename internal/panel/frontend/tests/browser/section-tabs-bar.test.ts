/**
 * Where a tab strip draws its bar, at rest and under the pointer.
 *
 * The bar is placed from script - two pixel values written onto an absolutely positioned box - so
 * where it lands is not a rule any stylesheet test can read. Same shape of problem as the Root
 * rail's thumb, and covered here for the same reason.
 *
 * Two rules, and they are the whole design: at rest the bar is the width of the label, so it reads
 * as naming the word; under the pointer it spreads to the whole padded box, which is the width the
 * hover ground covers, so the reach of the target is shown rather than stated. A bar that is always
 * the label's width says the tab is smaller than it is, and one that is always the box's width says
 * nothing about which word it belongs to.
 *
 * The third case is the one that was actually wrong: a tab that is clicked is a tab that is
 * hovered, so the bar has to arrive at the new tab already spread. No `mouseenter` is coming to
 * tell it - the pointer never moved - and the first version measured the label there, so every
 * click ended with the bar a size smaller than the pointer sitting on it.
 */
import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { SETTLE_MS, startPanel, visit, type Panel } from './harness';

/** Widths, in the strip's own coordinates, of the three boxes that have to agree. */
interface Reading {
  bar: number;
  /** The label, and the count beside it where the tab carries one. */
  ink: number;
  tab: number;
  /** Where the bar starts, so a spread is told from a bar that merely grew rightwards. */
  left: number;
  tabLeft: number;
}

const VIEWPORT = { width: 1500, height: 950 };
/** Away from the strip, and over nothing that answers a pointer. */
const NOWHERE = { x: 740, y: 700 };

let page: Page;
let panel: Panel;

async function read(): Promise<Reading> {
  return page.evaluate(() => {
    const strip = document.querySelector('.section-tabs');
    if (strip === null) throw new Error('no tab strip on the page');
    const bar = strip.querySelector('span[class*="section-tabs-bar"]');
    const tab = strip.querySelector('[aria-current="page"]');
    const word = tab?.querySelector('[class*="tab-word"]');
    if (bar === null || tab === null || word === null || word === undefined) {
      throw new Error('the strip is missing its bar, its open tab, or that tab’s label');
    }
    const count = tab.querySelector('[class*="tab-count"]');
    const origin = strip.getBoundingClientRect().left;
    const barBox = bar.getBoundingClientRect();
    const tabBox = tab.getBoundingClientRect();
    const wordBox = word.getBoundingClientRect();

    return {
      bar: barBox.width,
      ink: (count ?? word).getBoundingClientRect().right - wordBox.left,
      tab: tabBox.width,
      left: barBox.left - origin,
      tabLeft: tabBox.left - origin,
    };
  });
}

/** The bar moves on an animation, and no request reports the end of one. */
async function rest(): Promise<Reading> {
  await page.waitForTimeout(400);

  return read();
}

beforeAll(async () => {
  panel = await startPanel();
  page = await panel.browser.newPage({ viewport: VIEWPORT });
  await visit(page, `${panel.origin}/i/smykla-skalski/sync`);
  await page.locator('.section-tabs [aria-current="page"]').waitFor({ state: 'visible' });
  await page.waitForTimeout(SETTLE_MS);
}, 120_000);

afterAll(async () => {
  await panel?.close();
});

describe('section tabs bar [Integration]', () => {
  it('rests at the width of the open tab’s ink', async () => {
    await page.mouse.move(NOWHERE.x, NOWHERE.y);
    const reading = await rest();

    expect(reading.ink).toBeGreaterThan(0);
    expect(reading.bar).toBeCloseTo(reading.ink, 1);
    // Not the whole tab: the padding is the difference, and it is what this rule is about.
    expect(reading.tab - reading.ink).toBeGreaterThan(8);
  });

  it('spreads to the whole tab under the pointer, and comes back', async () => {
    await page.locator('.section-tabs [aria-current="page"]').hover();
    const spread = await rest();

    expect(spread.bar).toBeCloseTo(spread.tab, 1);
    expect(spread.left).toBeCloseTo(spread.tabLeft, 1);

    await page.mouse.move(NOWHERE.x, NOWHERE.y);
    const back = await rest();

    expect(back.bar).toBeCloseTo(back.ink, 1);
  });

  it('arrives at a clicked tab already spread, because the pointer is on it', async () => {
    const next = page.locator('.section-tabs a:not([aria-current="page"])').first();
    await next.hover();
    await next.click();
    await page.waitForTimeout(SETTLE_MS);
    const landed = await rest();

    expect(landed.bar).toBeCloseTo(landed.tab, 1);
    expect(landed.left).toBeCloseTo(landed.tabLeft, 1);

    await page.mouse.move(NOWHERE.x, NOWHERE.y);
    const settled = await rest();

    expect(settled.bar).toBeCloseTo(settled.ink, 1);
  });

  it('gives an unopened tab a preview that grows to the same width', async () => {
    const other = page.locator('.section-tabs a:not([aria-current="page"])').last();
    await other.hover();
    await page.waitForTimeout(400);
    const preview = await other.evaluate((tab) => {
      const style = getComputedStyle(tab, '::after');
      const word = tab.querySelector('[class*="tab-word"]');
      const count = tab.querySelector('[class*="tab-count"]');
      const left = word?.getBoundingClientRect().left ?? 0;
      const right = (count ?? word)?.getBoundingClientRect().right ?? 0;

      return {
        opacity: Number.parseFloat(style.opacity),
        scale: new DOMMatrixReadOnly(style.transform).a,
        share: Number.parseFloat(tab.style.getPropertyValue('--word-share')),
        ink: right - left,
        tab: tab.getBoundingClientRect().width,
      };
    });

    expect(preview.opacity).toBe(1);
    expect(preview.scale).toBeCloseTo(1, 2);
    // And the share it rests at is that tab's own ink, which is where it grew from.
    expect(preview.share * preview.tab).toBeCloseTo(preview.ink, 1);
  });
});
