import axe from 'axe-core';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { beforeAll, afterAll, it, expect } from 'vitest';
import { startPanel, type Panel } from './harness';
let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

it.each(['light', 'dark'] as const)(
  'guides first-run installation and recovery in %s',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    const capture = async (scene: string) => {
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (directory) {
        await mkdir(directory, { recursive: true });
        await page.screenshot({
          path: join(directory, `F09-installation-${scene}-${colorScheme}.png`),
          animations: 'disabled',
        });
      }
    };
    try {
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route('**/api/v1/installation', async (route) => {
        await held;
        await route.continue();
      });
      await page.goto(`${panel.origin}/?scenario=empty`);
      await page.getByRole('status').filter({ hasText: 'Loading installation link' }).waitFor();
      await capture('loading');
      release();
      await page.getByRole('link', { name: 'Install GitHub App', exact: true }).waitFor();
      await page.unroute('**/api/v1/installation');
      await page.goto(`${panel.origin}/?scenario=installation-unavailable`);
      const prompt = page.getByRole('region', { name: 'Get workspace access' });
      await prompt
        .getByText('Installation is not available from this panel.', { exact: false })
        .waitFor();
      expect(await prompt.getByRole('link').count()).toBe(0);
      await capture('unavailable');
      await page.goto(`${panel.origin}/?scenario=installation-error`);
      await prompt.getByRole('alert').waitFor();
      await capture('failure');
      // Clear the dev failure while preserving the current document and recovery control.
      await page.request.get(`${panel.origin}/?scenario=empty`);
      await prompt.getByRole('button', { name: 'Retry installation link' }).click();
      const install = prompt.getByRole('link', { name: 'Install GitHub App', exact: true });
      await install.waitFor();
      expect(await install.getAttribute('href')).toBe(
        'https://github.com/apps/smyklot/installations/new',
      );
      expect(await prompt.innerText()).toContain('ask your organization owner');
      expect(await prompt.innerText()).toContain('Ask a workspace owner to give you access');
      await prompt.getByRole('button', { name: 'Reload panel', exact: true }).focus();
      await page.keyboard.press('Shift+Tab');
      expect(await install.evaluate((node) => node === document.activeElement)).toBe(true);
      await page.evaluate(axe.source);
      const accessibility = await prompt.evaluate((node) =>
        (window as unknown as { axe: typeof axe }).axe.run(node),
      );
      expect(accessibility.violations).toEqual([]);
      await capture('ready');
      await page.context().route('https://github.com/apps/smyklot/installations/new', (route) =>
        route.fulfill({
          contentType: 'text/html',
          body: '<title>Installation destination</title><h1>GitHub installation fixture</h1>',
        }),
      );
      const opened = page.waitForEvent('popup');
      await install.press('Enter');
      const popup = await opened;
      await popup.waitForLoadState();
      expect(popup.url()).toBe('https://github.com/apps/smyklot/installations/new');
      expect(await prompt.isVisible()).toBe(true);
      await popup.close();
      await page.request.get(`${panel.origin}/`);
      await prompt.getByRole('button', { name: 'Reload panel', exact: true }).click();
      await page.locator('.app-shell').waitFor();
      expect(await prompt.count()).toBe(0);
    } catch (error) {
      throw new Error(
        `${String(error)} at ${page.url()}\n${await page.locator('body').innerText()}`,
        { cause: error },
      );
    } finally {
      await page.context().unrouteAll({ behavior: 'wait' });
      await page.close();
    }
  },
);
