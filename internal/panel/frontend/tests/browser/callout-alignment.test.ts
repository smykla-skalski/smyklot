import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
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
      } finally {
        await page.close();
      }
    },
  );
});
