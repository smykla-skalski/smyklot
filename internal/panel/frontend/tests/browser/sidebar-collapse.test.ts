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

describe('desktop compact navigation', () => {
  it.each(['light', 'dark'] as const)(
    'preserves discoverable scope switching in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        await visit(page, addressOf(panel, 'workspace/settings'));
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SIDEBAR_COLLAPSE_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.screenshot({ path: join(directory, `F40-${scene}-${colorScheme}.png`) });
        };
        const fold = page.locator('.side-fold');
        await fold.waitFor();
        if ((await fold.getAttribute('aria-expanded')) === 'true') await fold.click();
        const expand = page.getByRole('button', { name: 'Expand navigation', exact: true });
        await expect.poll(() => expand.getAttribute('aria-expanded')).toBe('false');
        const switcher = page.locator('.side-ws-mini');
        const originalLabel = await switcher.getAttribute('aria-label');
        expect(originalLabel).toMatch(/^Switch workspace - .+ is open$/);
        await switcher.hover();
        await page.locator('.side-tip').filter({ hasText: originalLabel! }).waitFor();
        await capture('scope');
        await switcher.click();
        const menu = page.getByRole('menu', { name: 'Switch workspace', exact: true });
        await menu.waitFor();
        await capture('menu');
        const other = menu.getByRole('menuitem').nth(1);
        const destination = await other.getAttribute('href');
        await other.click();
        await expect.poll(() => new URL(page.url()).pathname).toBe(destination);
        await expect.poll(() => switcher.getAttribute('aria-label')).not.toBe(originalLabel);
        await switcher.hover();
        await capture('switched');
        await page.reload();
        await expand.waitFor();
        await expand.press('Tab');
        await expect
          .poll(() => switcher.evaluate((node) => node === document.activeElement))
          .toBe(true);
        await capture('keyboard');
        await switcher.press('Enter');
        await menu.waitFor();
        await page.getByRole('searchbox', { name: 'Find a workspace' }).press('Escape');
        await expect
          .poll(() => switcher.evaluate((node) => node === document.activeElement))
          .toBe(true);
        await switcher.press('Enter');
        const consoleLink = menu.getByRole('menuitem', { name: /Operations/ });
        await consoleLink.press('Enter');
        await expect
          .poll(() => switcher.getAttribute('aria-label'))
          .toBe('Switch workspace - the Operations console is open');
        await switcher.hover();
        await capture('console');
        await switcher.press('Enter');
        await menu.getByRole('menuitem').first().press('Enter');
        await expect.poll(() => switcher.getAttribute('aria-label')).toBe(originalLabel);
        await expand.click();
        await page.getByRole('button', { name: 'Collapse navigation', exact: true }).waitFor();
        await capture('expanded');
      } finally {
        await page.close();
      }
    },
  );
});
