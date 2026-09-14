import axe from 'axe-core';
import { mkdir, writeFile } from 'node:fs/promises';
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
    const releases: Array<() => void> = [];
    try {
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      releases.push(release);
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
      const install = prompt.getByRole('link', { name: 'Install GitHub App', exact: true });
      for (const moveAway of [true, false]) {
        if (!moveAway) {
          await page.goto(`${panel.origin}/?scenario=installation-error`);
          await prompt.getByRole('alert').waitFor();
        }
        await page.request.get(`${panel.origin}/?scenario=empty`);
        let finish!: () => void;
        const waiting = new Promise<void>((resolve) => {
          finish = resolve;
        });
        releases.push(finish);
        let requests = 0;
        await page.route('**/api/v1/installation', async (route) => {
          requests++;
          await waiting;
          await route.continue();
        });
        const retry = prompt.getByRole('button', { name: 'Retry installation link' });
        await retry.focus();
        await retry.press('Enter');
        await page.waitForFunction(() => document.querySelector('[aria-busy="true"]'));
        expect(await retry.evaluate((node) => node === document.activeElement)).toBe(true);
        await retry.press('Enter');
        expect(requests).toBe(1);
        if (moveAway) await page.keyboard.press('Tab');
        await capture(moveAway ? 'retry-away' : 'retry-busy');
        finish();
        await install.waitFor();
        const expectedFocus = moveAway
          ? prompt.getByRole('button', { name: 'Reload panel', exact: true })
          : install;
        expect(await expectedFocus.evaluate((node) => node === document.activeElement)).toBe(true);
        await page.unroute('**/api/v1/installation');
      }
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
      const contrastBounds = await prompt.evaluate((region) => {
        const canvas = document.createElement('canvas');
        canvas.width = canvas.height = 1;
        const pen = canvas.getContext('2d', { willReadFrequently: true })!;
        const luminance = () => {
          const rgb = [...pen.getImageData(0, 0, 1, 1).data].slice(0, 3).map((c) => {
            const v = c / 255;
            return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
          });
          return rgb[0]! * 0.2126 + rgb[1]! * 0.7152 + rgb[2]! * 0.0722;
        };
        const card = getComputedStyle(region.closest('.night-card')!).backgroundColor;
        return [...region.querySelectorAll<HTMLElement>('h2, p, button')].map((node) => {
          const style = getComputedStyle(node);
          const ratios = ['black', 'white'].map((underlay) => {
            pen.clearRect(0, 0, 1, 1);
            pen.fillStyle = underlay;
            pen.fillRect(0, 0, 1, 1);
            pen.fillStyle = card;
            pen.fillRect(0, 0, 1, 1);
            pen.fillStyle = style.backgroundColor;
            pen.fillRect(0, 0, 1, 1);
            if (node.tagName === 'BUTTON') {
              pen.fillStyle = style.getPropertyValue('--control-state-layer');
              pen.fillRect(0, 0, 1, 1);
            }
            const background = luminance();
            pen.clearRect(0, 0, 1, 1);
            pen.fillStyle = style.color;
            pen.fillRect(0, 0, 1, 1);
            const foreground = luminance();
            return (
              (Math.max(background, foreground) + 0.05) / (Math.min(background, foreground) + 0.05)
            );
          });
          return { text: node.innerText, ratios, card, foreground: style.color };
        });
      });
      for (const result of contrastBounds) {
        expect(Math.min(...result.ratios), result.text).toBeGreaterThanOrEqual(4.5);
      }
      if (process.env.SMYKLOT_INSTALLATION_EVIDENCE) {
        await writeFile(
          join(
            process.env.SMYKLOT_INSTALLATION_EVIDENCE,
            `F09-installation-${colorScheme}-axe.json`,
          ),
          JSON.stringify({ ...accessibility, contrastBounds }, null, 2),
        );
      }
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
      for (const release of releases) release();
      await page.context().unrouteAll({ behavior: 'wait' });
      await page.close();
    }
  },
);
