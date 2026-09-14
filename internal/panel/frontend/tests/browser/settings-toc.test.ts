import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop settings section navigation', () => {
  it.each(['light', 'dark'] as const)(
    'tracks boundaries and reveals linked controls in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      try {
        const url = addressOf(panel, 'workspace/settings');
        await visit(page, url, { ready: '.page-toc' });
        const toc = page.getByRole('navigation', { name: 'On this page' });
        const first = toc.getByRole('link', { name: 'Configuration file sync', exact: true });
        const timing = toc.getByRole('link', { name: 'Timing', exact: true });
        await expect.poll(() => first.getAttribute('aria-current')).toBe('location');
        expect(await page.locator('#ws-timing').getAttribute('open')).toBeNull();
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SETTINGS_TOC_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({ path: join(directory, `F27-${scene}-${colorScheme}.png`) });
        };
        await capture('top');
        const merging = toc.getByRole('link', { name: 'Merging', exact: true });
        await merging.press('Enter');
        await expect.poll(() => merging.getAttribute('aria-current')).toBe('location');
        await capture('merging');
        await page.goBack();
        await expect.poll(() => first.getAttribute('aria-current')).toBe('location');
        await timing.click();
        await expect.poll(() => page.locator('#ws-timing').getAttribute('open')).not.toBeNull();
        await page.locator('#ws-timing').getByText('File index', { exact: true }).waitFor();
        await expect.poll(() => timing.getAttribute('aria-current')).toBe('location');
        await capture('timing');
        await page.goBack();
        await expect.poll(() => first.getAttribute('aria-current')).toBe('location');
        await page.goto(`${url}#ws-timing`, { waitUntil: 'domcontentloaded' });
        await page.reload({ waitUntil: 'domcontentloaded' });
        await expect.poll(() => page.locator('#ws-timing').getAttribute('open')).not.toBeNull();
        await expect.poll(() => timing.getAttribute('aria-current')).toBe('location');
        await capture('hash');
        await page.evaluate(() => window.scrollTo(0, 0));
        await expect.poll(() => first.getAttribute('aria-current')).toBe('location');
        await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
        await expect.poll(() => timing.getAttribute('aria-current')).toBe('location');
      } finally {
        await page.close();
      }
    },
  );
});
