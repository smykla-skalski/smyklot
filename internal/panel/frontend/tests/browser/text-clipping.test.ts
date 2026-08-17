import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

/**
 * Nothing that trims its box then clips what the trim left outside it.
 *
 * `text-box: trim-end cap alphabetic` ends the box on the baseline. It moves the box, not the
 * glyphs: descenders still paint below it, and so does the tail of an `@` and the comma in a list.
 * A box that also carries `overflow: hidden` - which is how every truncating line in the product
 * asks for its ellipsis - clips both axes, so the ellipsis it wanted came with a horizontal cut
 * through the bottom of the words.
 *
 * The remedy is one line and is written out beside the first place it was needed, the sidebar's
 * account card: clip the inline axis and leave the block axis visible. `overflow: hidden` cannot
 * express that, because a box hidden on one axis and visible on the other resolves both to
 * something that clips - `clip` is the value that can.
 *
 * The fault is invisible in every engine but Chrome, which is the only one that implements the
 * trim, and invisible in Chrome too until a name happens to contain a `y`. So it is measured
 * rather than looked at: for every element whose box is trimmed and whose block axis clips, this
 * asks whether the ink it holds is taller than the box left to hold it.
 */

interface Clip {
  route: string;
  where: string;
  text: string;
  lost: number;
}

async function clippedOn(page: Page): Promise<{ examined: number; found: Omit<Clip, 'route'>[] }> {
  return page.evaluate(() => {
    const label = (element: Element): string => {
      const classes = [...element.classList]
        .filter((one) => !one.startsWith('svelte-'))
        .slice(0, 2)
        .join('.');
      return `${element.tagName.toLowerCase()}${classes === '' ? '' : `.${classes}`}`;
    };

    const path = (element: Element): string => {
      const trail: string[] = [];
      for (
        let at: Element | null = element;
        at !== null && trail.length < 3;
        at = at.parentElement
      ) {
        trail.unshift(label(at));
      }
      return trail.join(' > ');
    };

    const measuring = document.createElement('canvas').getContext('2d');

    /** The y of the baseline: a zero-size inline-block sits its bottom edge on it. */
    function baselineOf(element: Element): number {
      const strut = document.createElement('span');
      strut.style.cssText = 'display:inline-block;width:0;height:0;vertical-align:baseline';
      element.append(strut);
      const y = strut.getBoundingClientRect().top;
      strut.remove();
      return y;
    }

    /**
     * Where the ink of this text actually reaches, which is the only measurement that answers the
     * question.
     *
     * Not the line box: `scrollHeight` and a range's client rects both report that, and a trimmed
     * box overflows its line box by definition - the trim IS the difference between them - so
     * either would call every trimmed line in the product clipped. Not nominal font metrics
     * either, which describe the face rather than the string: a word with no descender is not
     * clipped by a box that ends on the baseline, and half the words here have none.
     *
     * `measureText` reports the ink of these exact characters in this exact face, as the ascent
     * above the baseline and the descent below it. `Smykla Skalski` has a `y` and reaches 2.86px
     * past a box trimmed to the baseline; `Admin` has nothing below it and reaches 0.14px, which
     * is the trim's own rounding.
     */
    function inkOf(element: Element, style: CSSStyleDeclaration, text: string) {
      if (measuring === null) return null;
      measuring.font = `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
      const metrics = measuring.measureText(text);
      const baseline = baselineOf(element);

      return {
        top: baseline - metrics.actualBoundingBoxAscent,
        bottom: baseline + metrics.actualBoundingBoxDescent,
      };
    }

    const found: { where: string; text: string; lost: number }[] = [];
    let examined = 0;

    for (const element of document.querySelectorAll<HTMLElement>('*')) {
      const style = getComputedStyle(element);
      // Untrimmed boxes hold their own descenders; clipping one cuts nothing off.
      if (style.textBox === 'normal') continue;
      if (style.overflowY !== 'hidden' && style.overflowY !== 'clip') continue;
      // The `.visually-hidden` idiom clips its label to a single pixel on purpose.
      if (element.classList.contains('visually-hidden')) continue;
      if (element.children.length > 0) continue;
      const text = (element.textContent ?? '').trim();
      if (text === '') continue;

      const box = element.getBoundingClientRect();
      if (box.height === 0 || box.width === 0) continue;
      const ink = inkOf(element, style, text);
      if (ink === null) continue;

      examined += 1;
      /* The room `overflow-clip-margin` adds outside the box, which is the other way to keep a
         descender: clip, but not until a little past the edge. The queue's pull-request names ask
         for 0.4em of it and are not cut at all. */
      const margin = Number.parseFloat(style.getPropertyValue('overflow-clip-margin')) || 0;
      /* Whole device pixels at 1x, and the sweep runs at 1x. A trim lands the box edge on a
         fraction of a pixel routinely, and half of one is not something a reader can see. */
      const lost = Math.max(ink.bottom - box.bottom, box.top - ink.top) - margin;
      if (lost > 0.5) found.push({ where: path(element), text: text.slice(0, 40), lost });
    }

    return { examined, found };
  });
}

let panel: Panel;
let clipped: Clip[] = [];
let examined: Record<string, number> = {};

beforeAll(async () => {
  panel = await startPanel();
  clipped = [];
  examined = {};

  const readings = await inLanes(PANEL_ROUTES, async (route) => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, route));

      return { route, ...(await clippedOn(page)) };
    } finally {
      await page.close();
    }
  });

  for (const reading of readings) {
    examined[reading.route] = reading.examined;
    clipped.push(...reading.found.map((one) => ({ ...one, route: reading.route })));
  }
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('the text that trims its box [Integration]', () => {
  it('found trimmed lines to measure', () => {
    // A route that failed to load reports nothing clipped, which is what a route with nothing
    // wrong reports too. Counting what was looked at is what tells them apart.
    const total = Object.values(examined).reduce((sum, count) => sum + count, 0);

    expect(total, `trimmed lines examined per route: ${JSON.stringify(examined)}`).toBeGreaterThan(
      40,
    );
  });

  it('is never cut off by the box it was trimmed to', () => {
    const worst = [...clipped].sort((first, second) => second.lost - first.lost);
    const summary = worst
      .map((one) => `  ${one.lost.toFixed(2)}px  ${one.route}  ${one.where}  "${one.text}"`)
      .join('\n');

    expect(
      worst.map((one) => `${one.route} ${one.where}`),
      `these clip the ink they trimmed their box away from:\n${summary}`,
    ).toEqual([]);
  });
});
