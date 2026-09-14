import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import axe from 'axe-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop search accessibility', () => {
  it.each(['light', 'dark'] as const)(
    'keeps results and actions separate in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        await page.addInitScript(() =>
          localStorage.setItem(
            'smyklot-panel-recent-searches',
            JSON.stringify(['settings', 'queue']),
          ),
        );
        await visit(page, addressOf(panel, 'workspace/settings'));
        const trigger = page.locator('.side-search');
        const dialog = page.getByRole('dialog', { name: 'Search', exact: true });
        const field = dialog.getByRole('combobox', { name: 'Search', exact: true });
        const options = dialog.getByRole('option');
        const capture = async (scene: string) => {
          await page.evaluate(axe.source);
          const result = await page.evaluate(async () =>
            (window as unknown as { axe: typeof axe }).axe.run('.find-panel', {
              runOnly: {
                type: 'rule',
                values: [
                  'aria-required-children',
                  'aria-required-parent',
                  'aria-valid-attr-value',
                  'aria-input-field-name',
                ],
              },
            }),
          );
          expect(result.violations, scene).toEqual([]);
          const directory = process.env.SMYKLOT_SEARCH_A11Y_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          const evidence = process.env.SMYKLOT_SEARCH_A11Y_EVIDENCE;
          if (evidence) {
            await mkdir(evidence, { recursive: true });
            await writeFile(
              join(evidence, `F05-${scene}-${colorScheme}-axe.json`),
              JSON.stringify(result, null, 2),
            );
          }
          await page.mouse.move(1900, 20);
          await page.screenshot({ path: join(directory, `F05-${scene}-${colorScheme}.png`) });
        };
        await trigger.click();
        await field.waitFor();
        await expect
          .poll(() => field.getAttribute('aria-activedescendant'))
          .toBe(await options.first().getAttribute('id'));
        await field.press('ArrowDown');
        expect(await field.getAttribute('aria-activedescendant')).toBe(
          await options.nth(1).getAttribute('id'),
        );
        await capture('recents');
        await field.press('Tab');
        await page.keyboard.press('Enter');
        await expect.poll(() => dialog.isVisible()).toBe(false);
        await expect
          .poll(() => trigger.evaluate((node) => node === document.activeElement))
          .toBe(true);
        await trigger.click();
        await field.press('Tab');
        await page.keyboard.press('Tab');
        await page.keyboard.press('Enter');
        await expect.poll(() => options.count()).toBe(0);
        expect(await field.getAttribute('aria-activedescendant')).toBeNull();
        expect(await field.evaluate((node) => node === document.activeElement)).toBe(true);
        await capture('empty');
        await field.fill('zzzz-unmatched');
        await dialog.getByText('Nothing here matches "zzzz-unmatched"', { exact: true }).waitFor();
        const scope = dialog.getByRole('button', { name: /Search .* as well/ });
        expect(await scope.evaluate((node) => node.closest('[role="listbox"]') === null)).toBe(
          true,
        );
        await field.press('Enter');
        expect(await scope.isVisible()).toBe(true);
        await capture('no-matches');
        await field.press('Tab');
        await page.keyboard.press('Tab');
        await page.keyboard.press('Enter');
        await expect.poll(() => scope.count()).toBe(0);
        await field.fill('settings');
        await options.first().waitFor();
        await capture('cross-scope');
        await field.fill('th');
        const allResults = dialog.getByRole('link', { name: /See all results/ });
        await allResults.waitFor();
        expect(await allResults.evaluate((node) => node.closest('[role="listbox"]') === null)).toBe(
          true,
        );
        await capture('overflow');

        const ids = await options.evaluateAll((nodes) => nodes.map((node) => node.id));
        expect(ids.length).toBeGreaterThan(1);
        for (const id of ids.slice(1)) {
          await field.press('ArrowDown');
          expect(await field.getAttribute('aria-activedescendant')).toBe(id);
        }
        const selected = dialog.locator('[role="option"][aria-selected="true"]');
        const visible = await selected.evaluate((node) => {
          const row = node.getBoundingClientRect();
          const menu = node.closest('.find-menu')!.getBoundingClientRect();
          return row.top >= menu.top && row.bottom <= menu.bottom;
        });
        expect(visible).toBe(true);
        await capture('overflow-selected');
        const destination = await selected.getAttribute('href');
        expect(await field.evaluate((node) => node === document.activeElement)).toBe(true);
        await field.press('Enter');
        await expect.poll(() => new URL(page.url()).pathname).toBe(destination);
        await trigger.click();
        await field.fill('x');
        expect(await field.getAttribute('aria-activedescendant')).toBeNull();
        expect(await options.count()).toBe(0);
        await capture('short-query');
        await field.press('Escape');
        await expect
          .poll(() => trigger.evaluate((node) => node === document.activeElement))
          .toBe(true);
        await trigger.click();
        await field.fill('th');
        await scope.click();
        await allResults.waitFor();
        await field.press('Tab');
        await page.keyboard.press('Tab');
        await page.keyboard.press('Enter');
        await expect.poll(() => new URL(page.url()).pathname).toBe('/search');
        expect(new URL(page.url()).searchParams.get('q')).toBe('th');
        expect(await dialog.isVisible()).toBe(false);
      } finally {
        await page.close();
      }
    },
  );
});
