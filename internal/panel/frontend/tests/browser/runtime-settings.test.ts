import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page, Request } from 'playwright-core';

import { startPanel, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

function runtimeUpdate(page: Page): Promise<Request> {
  return page.waitForRequest(
    (request) =>
      request.method() === 'PUT' && new URL(request.url()).pathname === '/api/v1/root/settings',
  );
}

describe('Root merge-after-CI timing', () => {
  it('applies and restores the quiet period through the development API', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    const crashes: string[] = [];
    const settingsUptimes: number[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));
    await page.route('**/api/v1/root/settings', async (route) => {
      if (route.request().method() !== 'GET') {
        await route.continue();
        return;
      }
      const response = await route.fetch();
      const body = (await response.json()) as { service: { uptime_seconds: number } };
      body.service.uptime_seconds += settingsUptimes.length + 1;
      settingsUptimes.push(body.service.uptime_seconds);
      await route.fulfill({ response, json: body });
    });

    try {
      await page.goto(`${panel.origin}/root/settings`, { waitUntil: 'domcontentloaded' });
      const source = page.getByRole('group', {
        name: 'Merge-after-CI quiet period source',
      });
      await source.waitFor({ state: 'visible', timeout: 30_000 });
      await source.getByText('Custom', { exact: true }).click();

      const form = page.getByRole('form', { name: 'Custom merge-after-CI quiet period' });
      await form.getByRole('spinbutton', { name: 'Merge-after-CI quiet period' }).fill('2');
      await form
        .getByRole('combobox', { name: 'Merge-after-CI quiet period unit' })
        .selectOption('minutes');

      const readsBeforeRefetch = settingsUptimes.length;
      const uptimeBeforeRefetch = settingsUptimes.at(-1) ?? -1;
      expect(readsBeforeRefetch).toBeGreaterThan(0);
      const refetched = page.waitForResponse(
        (response) =>
          response.request().method() === 'GET' &&
          new URL(response.url()).pathname === '/api/v1/root/settings',
      );
      await page.evaluate(() => {
        Date.now = () => new Date().getTime() + 31_000;
        window.dispatchEvent(new Event('visibilitychange'));
      });
      await refetched;
      await page.evaluate(() => {
        Date.now = () => new Date().getTime();
      });
      expect(settingsUptimes.length).toBeGreaterThan(readsBeforeRefetch);
      expect(settingsUptimes.at(-1) ?? -1).toBeGreaterThan(uptimeBeforeRefetch);
      await expect
        .poll(() =>
          form.getByRole('spinbutton', { name: 'Merge-after-CI quiet period' }).inputValue(),
        )
        .toBe('2');
      await expect
        .poll(() =>
          form.getByRole('combobox', { name: 'Merge-after-CI quiet period unit' }).inputValue(),
        )
        .toBe('minutes');

      const applied = runtimeUpdate(page);
      await form.getByRole('button', { name: 'Apply' }).click();
      const applyRequest = await applied;
      expect(applyRequest.postDataJSON()).toMatchObject({
        merge_after_ci_quiet_period_seconds: 120,
      });
      await page.getByText('Effective: 2 minutes').waitFor({ state: 'visible' });

      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByText('Effective: 2 minutes').waitFor({ state: 'visible' });
      const restored = runtimeUpdate(page);
      await page
        .getByRole('button', {
          name: /Overrides the deployment configuration .* restores 30 seconds/u,
        })
        .click();
      const restoreRequest = await restored;
      expect(restoreRequest.postDataJSON()).toMatchObject({
        merge_after_ci_quiet_period_seconds: null,
      });
      await page.getByText('Effective: 30 seconds').waitFor({ state: 'visible' });
      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });
});
