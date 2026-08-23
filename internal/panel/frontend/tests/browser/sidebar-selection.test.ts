/**
 * Where the sidebar draws the row it has selected.
 *
 * The thumb is an absolutely positioned box measured onto the current row from script, so where
 * the selection is drawn is a pair of pixel values written by hand rather than a rule in the
 * stylesheet - the same shape of problem as the segmented control's indicator, and unreachable by
 * a stylesheet test for the same reason.
 *
 * The measurement used to be `offsetTop` and `offsetHeight`, which are rounded to whole pixels.
 * The console's rows sit on a fraction, because the trimmed header above them is cut to its cap
 * band and a cap height is not an integer, so the thumb was drawn a fifth of a pixel above every
 * row it was supposed to be the ground of. Small enough to survive review, and exactly the kind
 * of thing this sidebar is judged on.
 */
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { SETTLE_MS, startPanel, visit, type Panel } from './harness';

/** A box in the viewport's coordinates, which is the only ruler both of these share. */
interface Box {
  top: number;
  height: number;
}

interface Selection {
  thumb: Box;
  row: Box;
  label: string;
  /** Whether the sidebar is drawing its full rows, which is what puts them on a fraction. */
  expanded: boolean;
}

const VIEWPORT = { width: 1500, height: 950 };

let page: Page;
let panel: Panel;
let landed: Selection;
let moved: Selection;
let requestParentSelected: boolean;

beforeAll(async () => {
  panel = await startPanel();
  page = await panel.browser.newPage({ viewport: VIEWPORT });
  await visit(page, `${panel.origin}/root`);
  await page
    .locator('.tree .tree-row.is-active, .tree .tree-page.is-active > .tree-row')
    .first()
    .waitFor({ state: 'visible', timeout: 30_000 });
  await expand(page);
  landed = await measure(page);

  /* Then somewhere else, because arriving and travelling are two different writes of the same two
     values and only one of them is covered by the state the page loads in. */
  // Slept through rather than waited out: the thumb travels on a transition, and no request
  // reports the end of one.
  await page.locator('.tree a.tree-row:not(.is-active)').first().click();
  await page.waitForTimeout(SETTLE_MS);
  moved = await measure(page);

  await visit(page, `${panel.origin}/root/queue/request/pending-ci-0`);
  const requestParent = page.locator('.tree-page.is-active > .tree-row.is-active');
  await requestParent.waitFor({ state: 'visible' });
  requestParentSelected = (await requestParent.textContent())?.includes('Queue') === true;
}, 60_000);

afterAll(async () => {
  await panel?.close();
});

/**
 * The sidebar as this is written about it: expanded. A collapsed strip draws icon rows on its own
 * grid, and the trimmed header is the whole reason the full rows stand on a fraction - so the
 * precondition below would measure a whole pixel and the two checks after it would pass over
 * nothing. Expanded through the sidebar's own fold control rather than by seeding the preference:
 * what a reader does to see this is what the test should do.
 */
async function expand(target: Page): Promise<void> {
  const trigger = target.locator('.side-fold');
  if ((await trigger.getAttribute('aria-expanded')) === 'true') return;
  await trigger.click();
  // The sidebar opens on a transition, and the thumb is measured onto a row that is still moving.
  await target.waitForTimeout(SETTLE_MS);
}

async function measure(target: Page): Promise<Selection> {
  return target.evaluate(() => {
    const box = (element: Element | null): { top: number; height: number } => {
      if (element === null) return { top: -1, height: -1 };
      const rect = element.getBoundingClientRect();

      return { top: rect.top, height: rect.height };
    };
    const kid = document.querySelector<HTMLElement>('.tree .tree-kid.is-active');
    const row =
      kid !== null && kid.offsetParent !== null
        ? kid
        : document.querySelector(
            '.tree .tree-row.is-active, .tree .tree-page.is-active > .tree-row',
          );

    return {
      thumb: box(document.querySelector('.tree .nav-thumb')),
      row: box(row),
      label: row?.querySelector('.t')?.textContent ?? '',
      expanded: document.querySelector('.app-shell.sidebar-collapsed') === null,
    };
  });
}

describe("the Root console sidebar's selection", () => {
  it('sits on a fraction of a pixel, which is what makes the rest of this worth asserting', () => {
    // The precondition, stated rather than assumed. Were the rows to land on whole pixels the
    // rounding below could not show, and every other check here would pass by measuring nothing.
    // The sidebar is asserted first, because a collapsed one is the way this stops being true and
    // "the rows are on whole pixels" does not say so.
    expect(landed.expanded, 'the sidebar is collapsed, so it draws no full rows').toBe(true);
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

  it('selects the parent row while a detail route has no selected child', () => {
    expect(requestParentSelected).toBe(true);
  });
});
