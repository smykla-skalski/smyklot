/**
 * Where the Root console's rail draws the section it has selected.
 *
 * The thumb is an absolutely positioned box measured onto the current row from script, so where the
 * selection is drawn is a pair of pixel values written by hand rather than a rule in the
 * stylesheet - the same shape of problem as the segmented control's indicator, and unreachable by a
 * stylesheet test for the same reason.
 *
 * The measurement used to be `offsetTop` and `offsetHeight`, which are rounded to whole pixels. The
 * console's rows sit on a fraction, because the section header above them is trimmed to its cap
 * band and a cap height is not an integer, so the thumb was drawn a fifth of a pixel above every
 * row it was supposed to be the ground of. Small enough to survive review, and exactly the kind of
 * thing this rail is judged on.
 */
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { SETTLE_MS, startPanel, type Panel } from './harness';

/** A box in the viewport's coordinates, which is the only ruler both of these share. */
interface Box {
  top: number;
  height: number;
}

interface Selection {
  thumb: Box;
  row: Box;
  label: string;
}

/** Wide enough that the rail is expanded: it collapses itself below 1152px. */
const VIEWPORT = { width: 1500, height: 950 };

let page: Page;
let panel: Panel;
let landed: Selection;
let moved: Selection;

beforeAll(async () => {
  panel = await startPanel();
  page = await panel.browser.newPage({ viewport: VIEWPORT });
  await page.goto(`${panel.origin}/root`, { waitUntil: 'domcontentloaded' });
  await page.locator('.view-links a.active').waitFor({ state: 'visible', timeout: 30_000 });
  await page.waitForTimeout(SETTLE_MS);
  landed = await measure(page);

  /* Then somewhere else, because arriving and travelling are two different writes of the same two
     values and only one of them is covered by the state the page loads in. */
  await page.locator('.view-links a:not(.active)').first().click();
  await page.waitForTimeout(SETTLE_MS);
  moved = await measure(page);
}, 60_000);

afterAll(async () => {
  await panel?.close();
});

async function measure(target: Page): Promise<Selection> {
  return target.evaluate(() => {
    const box = (element: Element | null): { top: number; height: number } => {
      if (element === null) return { top: -1, height: -1 };
      const rect = element.getBoundingClientRect();

      return { top: rect.top, height: rect.height };
    };
    const row = document.querySelector('.panel-sidebar .view-links a.active');

    return {
      thumb: box(document.querySelector('.panel-sidebar .nav-thumb')),
      row: box(row),
      label: row?.querySelector('.navigation-label')?.textContent ?? '',
    };
  });
}

describe("the Root console rail's selection", () => {
  it('sits on a fraction of a pixel, which is what makes the rest of this worth asserting', () => {
    // The precondition, stated rather than assumed. Were the rows to land on whole pixels the
    // rounding below could not show, and every other check here would pass by measuring nothing.
    expect(landed.row.top % 1, `the rows are on whole pixels: ${landed.row.top}`).not.toBe(0);
  });

  it('grounds the row it arrives on, exactly', () => {
    expect(landed.label, 'nothing was selected when the console loaded').not.toBe('');
    // Under a twentieth of a pixel, not the one pixel a placement test would allow: whole-pixel
    // rounding is the defect, so a tolerance of a pixel is a tolerance of the defect.
    expect(Math.abs(landed.thumb.top - landed.row.top)).toBeLessThan(0.05);
    expect(Math.abs(landed.thumb.height - landed.row.height)).toBeLessThan(0.05);
  });

  it('grounds the row it travels to, exactly', () => {
    expect(moved.label, 'choosing another section selected nothing').not.toBe(landed.label);
    expect(Math.abs(moved.thumb.top - moved.row.top)).toBeLessThan(0.05);
    expect(Math.abs(moved.thumb.height - moved.row.height)).toBeLessThan(0.05);
  });
});
