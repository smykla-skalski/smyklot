/**
 * Where the segmented control draws the option it has selected.
 *
 * The thumb is one absolutely positioned box measured onto the checked option from script, so
 * "which option is selected" is a pair of pixel values written by hand rather than a rule in the
 * stylesheet. That makes it the one part of the control a stylesheet test cannot reach, and it
 * failed in the place hardest to notice: a control inside a *closed* popover has no box at all,
 * `offsetWidth` reports that as 0 exactly as it reports a real zero, and the one measurement the
 * control had ever taken was the one it took there. The account menu's theme switch opened with
 * no option marked at all and stayed that way until a different theme was chosen, which was the
 * only thing that asked for another measurement.
 *
 * So this opens the layers and looks. Two of them, because the bug was in the control rather than
 * in either caller, and one of them passing would not have said so. A popover is only the first
 * way a control can be laid out for the first time after the page has settled - a section that is
 * not rendered until somebody opens it is the same hazard with different markup, which is why the
 * second is one of those rather than a second popover.
 */
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { SETTLE_MS, startPanel, type Panel } from './harness';

/**
 * Where the thumb sat in one frame, in the control's own coordinates rather than the viewport's.
 *
 * `offsetLeft` and `offsetWidth` are read against the fieldset, so they say where the thumb is on
 * the control and nothing else. The viewport's numbers would also move with the popover, which
 * places itself against its trigger when it opens - a frame-by-frame record of those would be
 * reading the layer's arrival as if the thumb had travelled.
 */
interface Frame {
  left: number;
  width: number;
}

interface Thumb {
  /** Every frame the thumb was drawn in, from before the popover opened. */
  frames: Frame[];
  /** The thumb and the option it claims to be sitting on, once everything has settled. */
  thumb: { left: number; width: number };
  option: { left: number; width: number };
  checked: string | null;
  /** Every frame drawn after a *different* option was chosen, and where it ended up. */
  movedFrames: Frame[];
  moved: { thumb: { left: number; width: number }; option: { left: number; width: number } } | null;
}

interface Menu {
  name: string;
  path: (account: string) => string;
  /** What has to be on the page before the layer can be asked for. */
  ready: string;
  /** The layer the control is in, which matches nothing until it is open. */
  layer: string;
  open: (page: Page) => Promise<void>;
}

/** A layer holding a segmented control, and how to get it open. */
const MENUS: Menu[] = [
  {
    name: "the account menu's theme switch",
    path: (account: string) => `/workspace/${account}/settings`,
    ready: '.rail button[aria-label^="Account menu for"]',
    layer: '.app-popover[data-state="open"]',
    /* The rail's, not the collapsed sidebar's: both open the same menu, and the
       sidebar's is in the DOM at every width even where the rail is drawn. */
    open: (page) => page.locator('.rail button[aria-label^="Account menu for"]').click(),
  },
  {
    /* The history page's display options used to stand here, and its segmented control
       is gone: how a time is written is one question among the several that page asks,
       and it is asked where the others are. A repository's formatting is the same
       hazard in a plainer form - the section is not in the document at all until its
       adjuster is opened, so the control's first layout happens long after load. */
    name: "a repository's formatting, once its adjuster is open",
    path: (account: string) => `/workspace/${account}/sync/files/renovate.json`,
    ready: '.format-status',
    layer: '.repository-formatting',
    open: (page) => page.getByRole('button', { name: /^smyklot changes/u }).click(),
  },
];

let panel: Panel;
const opened = new Map<string, Thumb>();

beforeAll(async () => {
  panel = await startPanel();
  /* One at a time, because the whole measurement is a frame-by-frame record and Chromium
     throttles `requestAnimationFrame` to a crawl on a page that is not the front tab. Two
     of these open at once and the one behind reports three identical frames over 800ms,
     which reads as a thumb that never moved. */
  for (const menu of MENUS) {
    opened.set(menu.name, await openAndWatch(menu));
  }
});

afterAll(async () => {
  await panel?.close();
});

/**
 * Opens one popover and reports what its thumb did, watched from the frame before the press.
 *
 * Sampled every frame rather than read once after a delay: the difference between appearing in
 * place and sliding in from the corner is a 240ms transition, and any single sample either misses
 * it or catches it depending on how loaded the machine is. A frame-by-frame record settles it
 * without depending on the timing at all - if the thumb slid, one of these frames saw it narrow.
 */
async function openAndWatch(menu: Menu): Promise<Thumb> {
  const page: Page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
  const crashes: string[] = [];
  page.on('pageerror', (error) => crashes.push(error.message));

  try {
    await page.goto(`${panel.origin}${menu.path(panel.account)}`, {
      waitUntil: 'domcontentloaded',
    });
    await page.locator(menu.ready).first().waitFor({ state: 'visible', timeout: 30_000 });
    await page.waitForTimeout(SETTLE_MS);

    /* The one control this reads: the first in the layer that has an option chosen,
       which is the whole of a popover's and one of several in a formatting editor.
       Every measurement below comes off it, so the thumb and the checked option are
       always the same control's - a layer holding four of them has four checked
       inputs. Having one is part of the selector rather than an assertion, because a
       control with no value draws no thumb on purpose: the formatting editor opens on
       a row whose width is inherited and therefore chosen nowhere. */
    const control = `${menu.layer} fieldset:has(.selection-indicator):has(input:checked)`;

    await page.evaluate((control: string) => {
      const frames: { left: number; width: number }[] = [];
      (window as unknown as { thumbFrames: typeof frames }).thumbFrames = frames;
      const tick = (): void => {
        // Scoped to the open layer: a closed popover still has its markup, and its thumb is
        // legitimately nothing at all.
        const indicator = document
          .querySelector(control)
          ?.querySelector<HTMLElement>('.selection-indicator');
        if (indicator !== undefined && indicator !== null) {
          frames.push({ left: indicator.offsetLeft, width: indicator.offsetWidth });
        }
        requestAnimationFrame(tick);
      };
      tick();
    }, control);

    await menu.open(page);
    await page.locator(control).first().waitFor({ state: 'visible', timeout: 30_000 });
    await page.waitForTimeout(SETTLE_MS);

    const measured = await page.evaluate((control: string) => {
      const layer = document.querySelector(control);
      const indicator = layer?.querySelector('.selection-indicator') ?? null;
      const checked = layer?.querySelector<HTMLInputElement>('input:checked') ?? null;
      const option = checked?.closest('label') ?? null;
      const box = (element: Element | null): { left: number; width: number } => {
        if (element === null) return { left: -1, width: -1 };
        const rect = element.getBoundingClientRect();

        return { left: rect.left, width: rect.width };
      };

      return {
        frames: (window as unknown as { thumbFrames: { left: number; width: number }[] })
          .thumbFrames,
        thumb: box(indicator),
        option: box(option),
        checked: checked?.value ?? null,
      };
    }, control);

    /* Now choose a different option and watch again. Appearing in place and travelling between
       options are opposite requirements on the same transition, and only one of them was covered:
       switching the transition off entirely would satisfy every check above. The move is the half
       that says the control still animates at all. */
    const moved = await page.evaluate(async (control: string) => {
      const window_ = window as unknown as { thumbFrames: { left: number; width: number }[] };
      const layer = document.querySelector(control);
      const options = [...(layer?.querySelectorAll<HTMLInputElement>('input[type="radio"]') ?? [])];
      const other = options.find((option) => !option.checked && !option.disabled);
      if (other === undefined) return null;

      // Emptied in place: the sampler pushes into the array it captured, so handing it a new one
      // here would leave it filling something nothing reads.
      window_.thumbFrames.length = 0;
      other.click();
      await new Promise((settle) => setTimeout(settle, 800));

      const indicator = layer?.querySelector('.selection-indicator') ?? null;
      const option = other.closest('label');
      const box = (element: Element | null): { left: number; width: number } => {
        if (element === null) return { left: -1, width: -1 };
        const rect = element.getBoundingClientRect();

        return { left: rect.left, width: rect.width };
      };

      return {
        frames: [...window_.thumbFrames],
        thumb: box(indicator),
        option: box(option),
      };
    }, control);

    if (crashes.length > 0) throw new Error(`the page crashed: ${crashes.join(', ')}`);

    return {
      ...measured,
      movedFrames: moved?.frames ?? [],
      moved: moved === null ? null : { thumb: moved.thumb, option: moved.option },
    };
  } finally {
    await page.close();
  }
}

describe('the segmented control inside a popover', () => {
  it.each(MENUS.map((menu) => menu.name))('marks its selection the first time %s opens', (name) => {
    const result = opened.get(name);
    if (result === undefined) throw new Error(`${name} was never opened`);

    // The precondition. Every one of these controls has a value, so nothing checked means the
    // popover never opened and the rest of this would pass by measuring an empty room.
    expect(result.checked, `${name} opened with no option checked`).not.toBeNull();

    expect(result.thumb.width, `${name} opened with no selection marked`).toBeGreaterThan(0);
    // On the option, not merely somewhere. A thumb of the right size under the wrong option is
    // the same bug one measurement later.
    expect(Math.abs(result.thumb.left - result.option.left)).toBeLessThanOrEqual(0.05);
    expect(Math.abs(result.thumb.width - result.option.width)).toBeLessThanOrEqual(0.05);
  });

  it.each(MENUS.map((menu) => menu.name))('does not slide it into place in %s', (name) => {
    const result = opened.get(name);
    if (result === undefined) throw new Error(`${name} was never opened`);

    const first = result.frames.find((frame) => frame.width > 0);
    const settled = result.frames.at(-1);
    expect(first, `${name} never drew a thumb in any frame`).toBeDefined();
    expect(settled, `${name} drew no frames at all`).toBeDefined();

    /* The first frame that drew anything drew all of it, in both axes. A selection growing out of
       the left edge is the control announcing a move nobody made, and so is one arriving from the
       wrong option - width alone would miss the second, which is what happens the moment the two
       geometry writes stop landing in the same style flush. Compared against the last frame rather
       than against a rect measured afterwards, so both numbers come from the same ruler. */
    expect(
      { left: first?.left, width: first?.width },
      `${name} opened at ${JSON.stringify(first)} and settled at ${JSON.stringify(settled)}`,
    ).toEqual({ left: settled?.left, width: settled?.width });
  });

  it.each(MENUS.map((menu) => menu.name))('does travel between options in %s', (name) => {
    const result = opened.get(name);
    if (result === undefined) throw new Error(`${name} was never opened`);
    if (result.moved === null) throw new Error(`${name} has only one option, so nothing can move`);

    // It arrives, first of all. Everything below is about how.
    expect(Math.abs(result.moved.thumb.left - result.moved.option.left)).toBeLessThanOrEqual(0.05);
    expect(Math.abs(result.moved.thumb.width - result.moved.option.width)).toBeLessThanOrEqual(
      0.05,
    );

    /* And it went there rather than appearing there. Landing in place and travelling between
       options pull against each other on one transition, so a change that simply turned the
       transition off would pass every other check in this file. The positions between the two ends
       are the evidence that it did not. */
    const places = new Set(result.movedFrames.map((frame) => `${frame.left}x${frame.width}`));
    expect(
      places.size,
      `${name} jumped straight to the new option instead of travelling: ${[...places].join(' ')}`,
    ).toBeGreaterThan(2);
  });
});
