import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Locator, Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { calloutGeometry } from './callout-geometry';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

async function expectDecisionPaint(notice: Locator, page: Page) {
  const paint = await notice.evaluate((node) => {
    const surface = node.querySelector('.callout')!;
    const style = getComputedStyle(surface);
    const probe = document.createElement('span');
    probe.style.cssText = 'position:absolute;visibility:hidden;transition:none;animation:none';
    surface.append(probe);
    const color = (token: string) => {
      probe.style.color = `var(${token})`;
      return getComputedStyle(probe).color;
    };
    const expected = {
      background: color('--popover-bg'),
      border: color('--decision-accent'),
      hover: color('--decision-accent-hover'),
      pressed: color('--decision-accent-pressed'),
      ink: color('--on-decision-accent'),
    };
    probe.remove();
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 1;
    const context = canvas.getContext('2d')!;
    context.fillStyle = style.backgroundColor;
    context.fillRect(0, 0, 1, 1);
    return {
      expected,
      background: style.backgroundColor,
      border: style.borderTopColor,
      width: style.borderTopWidth,
      backdrop: style.backdropFilter,
      shadow: style.boxShadow,
      alpha: context.getImageData(0, 0, 1, 1).data[3],
    };
  });
  expect(paint.background).toBe(paint.expected.background);
  expect(paint.border).toBe(paint.expected.border);
  expect(paint.width).toBe('2px');
  expect(paint.alpha).toBe(255);
  expect(paint.backdrop).toBe('none');
  expect(paint.shadow).not.toContain('inset');

  const review = notice.getByRole('link', { name: 'Review' });
  if ((await review.count()) === 0) return;
  expect(await review.evaluate((node) => getComputedStyle(node).backgroundColor)).toBe(
    paint.expected.border,
  );
  expect(await review.evaluate((node) => getComputedStyle(node).color)).toBe(paint.expected.ink);
  await review.hover();
  await expect
    .poll(() => review.evaluate((node) => getComputedStyle(node).backgroundColor))
    .toBe(paint.expected.hover);
  await page.mouse.down();
  await expect
    .poll(() => review.evaluate((node) => getComputedStyle(node).backgroundColor))
    .toBe(paint.expected.pressed);
  expect(await review.evaluate((node) => getComputedStyle(node).boxShadow)).not.toBe('none');
  expect(await review.evaluate((node) => getComputedStyle(node).translate)).toBe('0px 1px');
  await review.evaluate((node) =>
    node.addEventListener('click', (event) => event.preventDefault(), { once: true }),
  );
  await page.mouse.up();
  await page.mouse.move(0, 0);
}

describe('shared Callout first-line alignment [Browser]', () => {
  it.each([
    { colorScheme: 'light', width: 1440 },
    { colorScheme: 'dark', width: 1440 },
    { colorScheme: 'light', width: 375 },
    { colorScheme: 'dark', width: 375 },
  ] as const)(
    'aligns visible symbols with the first text line at $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1800 },
        reducedMotion: 'reduce',
      });
      try {
        await visit(page, addressOf(panel, 'workspace/'));
        await page.addScriptTag({
          type: 'module',
          url: `${panel.origin}/tests/support/callout-harness.ts`,
        });
        await page.locator('[data-case="warning-action"]').waitFor();
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        const samples = await calloutGeometry(page.locator('.callout-matrix .callout'));
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await writeFile(
            join(directory, `callout-${colorScheme}-${width}.json`),
            JSON.stringify(samples, null, 2),
          );
          await page.locator('.callout-matrix').screenshot({
            path: join(directory, `callout-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
          for (const kind of ['inactive', 'storage-problem']) {
            await page.locator(`.settings-draft-attention[data-kind="${kind}"]`).screenshot({
              path: join(directory, `draft-${kind}-${colorScheme}-${width}.png`),
              animations: 'disabled',
            });
          }
        }
        expect(samples).toHaveLength(12);
        for (const sample of samples) {
          expect(Math.abs(sample.difference), JSON.stringify(sample)).toBeLessThanOrEqual(0.25);
          expect(sample.overflow, JSON.stringify(sample)).toBeLessThanOrEqual(1);
          if (sample.copyGap !== null)
            expect(Math.abs(sample.copyGap - 8), JSON.stringify(sample)).toBeLessThanOrEqual(0.25);
          for (const gap of sample.lineGaps)
            expect(Math.abs(gap - 8), JSON.stringify(sample)).toBeLessThanOrEqual(0.25);
          expect(
            Math.abs(sample.topSpace - sample.bottomSpace),
            JSON.stringify(sample),
          ).toBeLessThanOrEqual(0.25);
          if (width === 375 && sample.name?.endsWith('-wrapped'))
            expect(sample.lineGaps.length).toBeGreaterThan(0);
        }
        const draft = page.locator('.settings-draft-attention[data-kind="inactive"]');
        const draftLayout = await draft.evaluate((node) => {
          const content = node.querySelector('.callout-content')!.getBoundingClientRect();
          const actions = node.querySelector('.callout-actions')!.getBoundingClientRect();
          return {
            direction: getComputedStyle(node.querySelector('.callout-actions')!).flexDirection,
            contentWidth: content.width,
            actionsLeft: actions.left,
            contentRight: content.right,
          };
        });
        expect(draftLayout.direction).toBe(width === 375 ? 'column' : 'row');
        expect(draftLayout.contentWidth).toBeGreaterThanOrEqual(180);
        expect(draftLayout.actionsLeft).toBeGreaterThan(draftLayout.contentRight);
        for (const notice of await page.locator('.settings-draft-attention').all()) {
          await expectDecisionPaint(notice, page);
        }
      } finally {
        await page.close();
      }
    },
  );
});
