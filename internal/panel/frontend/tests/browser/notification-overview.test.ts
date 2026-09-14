import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { expect, it } from 'vitest';
import { addressOf, startPanel, visit } from './harness';

it.each(['light', 'dark'] as const)(
  'keeps personal unread counts aligned after reads in %s',
  async (theme) => {
    const panel = await startPanel();
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme: theme,
      reducedMotion: 'reduce',
    });
    const rootCounts: number[] = [];
    page.on('response', async (response) => {
      if (new URL(response.url()).pathname === '/api/v1/root/overview') {
        rootCounts.push((await response.json()).unread_security_events);
      }
    });
    async function capture(scene: string) {
      const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.mouse.move(1900, 20);
      await page.screenshot({ path: join(directory, `F11-${scene}-${theme}.png`) });
    }
    async function returnToOverview() {
      await page.locator('a[href="/root"]').first().click();
      await page.waitForURL(addressOf(panel, 'root'));
    }
    try {
      await visit(page, addressOf(panel, 'root'));
      const prompt = page.getByRole('link').filter({ hasText: 'Review unread notifications' });
      expect(await prompt.textContent()).toContain('2 unread');
      expect(await prompt.textContent()).toContain('unread in your inbox');
      await capture('overview-unread');
      await prompt.click();
      await page.waitForURL(addressOf(panel, 'inbox'));
      const inbox = page.locator('.inbox-page');
      await inbox.getByText('2 unread', { exact: true }).waitFor();
      expect(await inbox.textContent()).toContain('Operator changes to workspaces you own');
      await capture('inbox-unread');
      await inbox
        .getByRole('button', { name: /^Mark read -/ })
        .first()
        .click();
      await inbox.getByText('1 unread', { exact: true }).waitFor();
      await returnToOverview();
      await expect.poll(() => prompt.textContent()).toContain('1 unread');
      await capture('overview-one-unread');
      await prompt.click();
      await inbox.getByText('1 unread', { exact: true }).waitFor();
      await capture('inbox-one-unread');
      await inbox.getByRole('button', { name: 'Mark all read', exact: true }).click();
      await inbox.getByText('All read', { exact: true }).waitFor();
      await capture('inbox-all-read');
      await returnToOverview();
      await expect.poll(() => rootCounts.at(-1)).toBe(0);
      await expect.poll(() => prompt.count()).toBe(0);
      expect(rootCounts).toContain(2);
      expect(rootCounts).toContain(1);
      await capture('overview-all-read');
    } finally {
      await page.close();
      await panel.close();
    }
  },
);
