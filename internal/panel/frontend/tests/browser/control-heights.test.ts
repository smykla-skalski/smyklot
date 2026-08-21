import { writeFileSync } from 'node:fs';

import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, inLanes, PANEL_ROUTES, startPanel, visit, type Panel } from './harness';

/**
 * Every control is one of a few heights, and every one of those is a whole pixel.
 *
 * A control's height should be a decision. Two of them here were an outcome instead: `.chip` and
 * `.add-chip` were built out of padding, so each stood as tall as whatever it happened to hold -
 * a chip carrying a clear button measured 27.59px beside an add-chip at 23.8, and the pair sat
 * side by side in one row looking like two different products. Neither was a whole pixel, so
 * neither could be lined up with anything by eye or by arithmetic.
 *
 * The fractional part matters on its own. A control on a half pixel puts its border on a half
 * pixel, and the rasteriser then paints one edge darker than the other on every display that is
 * not exactly 2x - which reads as a control that is subtly the wrong shape rather than as one that
 * is the wrong size.
 *
 * This sweeps the real pages rather than a list of components, because the defect is not in any
 * one component: it is in a control meeting a caller that gives it something taller than it
 * expected.
 */

/**
 * The heights the panel has, from `app.css`.
 *
 * A short list on purpose. Anything not on it is either a control nobody sized or a fourth size
 * somebody added without saying so, and both are worth failing over.
 */
const HEIGHTS = new Set([
  20, // --control-height-chip-small
  24, // --control-height-chip
  26, // the setting-clear disc beside a policy row
  28, // the sidebar's own collapse trigger, and the value selects
  30, // the shell grammar's chips: add-chip, cmd-chip, pattern entries
  34, // --control-height-compact
  38, // a rail tile
  40, // --control-height
  48, // the identity row
  52, // the workspace trigger
]);

interface Control {
  route: string;
  where: string;
  height: number;
}

/**
 * What counts as a control, and what is deliberately left out.
 *
 * A `<button>` wrapping a whole card, a row or a tile is a target rather than a control - it is
 * sized by what it contains, which is the point of it. Named by the class it carries rather than
 * by guessing from its size, so a control that grows past a threshold is still measured.
 */
const NOT_A_CONTROL =
  '.tile, .kind-card, .legend-row, .attn-row, .data-row, .repo-group, .queue-row, ' +
  '.finder-opt, .summary-card, .board-well *, [role="option"], [role="tab"], summary';

async function controlsOn(page: Page): Promise<Omit<Control, 'route'>[]> {
  return page.evaluate((skip) => {
    const found: { where: string; height: number }[] = [];
    const label = (element: Element): string => {
      const classes = [...element.classList]
        .filter((one) => !one.startsWith('svelte-'))
        .slice(0, 2)
        .join('.');

      return `${element.tagName.toLowerCase()}${classes === '' ? '' : `.${classes}`}`;
    };

    for (const control of document.querySelectorAll<HTMLElement>(
      'button, input, select, a.btn, .chip, .add-chip, .state-mark',
    )) {
      if (control.closest(skip) !== null) continue;
      const style = getComputedStyle(control);
      if (style.display === 'none' || style.visibility === 'hidden') continue;
      // The `.visually-hidden` idiom clips to a single pixel and keeps its box in layout.
      if (style.clipPath === 'inset(50%)') continue;
      if (control instanceof HTMLInputElement && ['checkbox', 'radio'].includes(control.type)) {
        continue;
      }
      /* A button that paints nothing is a link wearing a button, and its height is a line box
         rather than a size anybody chose. `.audit-toggle` is the shape: no ground, no edge, no
         padding, `font: inherit` - it is the word "Show the change" in a sentence. */
      const paints =
        style.backgroundColor !== 'rgba(0, 0, 0, 0)' ||
        style.backgroundImage !== 'none' ||
        style.boxShadow !== 'none' ||
        style.outlineStyle === 'dashed' ||
        Number.parseFloat(style.borderTopWidth) > 0;
      if (!paints) continue;

      const rect = control.getBoundingClientRect();
      if (rect.height === 0 || rect.width === 0) continue;
      // Wrapped onto a second line: its height is its content's, which is the caller's layout
      // rather than the control's own size.
      if (rect.height > 60) continue;

      found.push({ where: label(control), height: rect.height });
    }

    return found;
  }, NOT_A_CONTROL);
}

let panel: Panel;
let controls: Control[];

beforeAll(async () => {
  panel = await startPanel();
  controls = [];

  const readings = await inLanes(PANEL_ROUTES, async (route) => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, route));

      return { route, found: await controlsOn(page) };
    } finally {
      await page.close();
    }
  });

  for (const reading of readings) {
    controls.push(...reading.found.map((control) => ({ ...control, route: reading.route })));
  }
}, 300_000);

afterAll(async () => {
  await panel?.close();
});

describe('control heights [Integration]', () => {
  it('found controls to measure', () => {
    expect(controls.length).toBeGreaterThan(50);
  });

  it('stands every control on a whole pixel', () => {
    const fractional = controls.filter((control) => !Number.isInteger(control.height));
    writeFileSync(
      '/tmp/control-heights.txt',
      controls
        .map((control) => `${control.height.toFixed(2)}  ${control.route}  ${control.where}`)
        .sort()
        .join('\n'),
    );

    expect(
      [...new Set(fractional.map((one) => `${one.where} ${one.height.toFixed(2)}px`))],
      `${fractional.length} controls are on a fraction`,
    ).toEqual([]);
  });

  it('gives every control one of the panel’s own heights', () => {
    const strange = controls.filter((control) => !HEIGHTS.has(control.height));

    expect(
      [...new Set(strange.map((one) => `${one.where} ${one.height}px`))],
      `heights the panel does not declare: ${[...HEIGHTS].join(', ')}`,
    ).toEqual([]);
  });
});
