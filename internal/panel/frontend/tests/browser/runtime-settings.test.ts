import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page, Request } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

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

/**
 * Addresses the panel has no route for, which come in two shapes.
 *
 * One the server refuses, answering with its own error document. One it serves - because
 * it decides from the decoded path, while the router matches on the raw one, so a
 * percent-encoded separator means the console to the server and nothing to the router.
 * The panel reads its route from the router now, so the second shape is where reading the
 * route and reading the address disagree, and it is the reason the getters fall back.
 */
describe('an address that resolves to nothing', () => {
  it('shows what happened when the server refuses it', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await visit(page, `${panel.origin}/root/definitely-not-a-page`, {
        ready: '.error-body',
        mount: 5_000,
      });

      expect(await page.locator('body').innerText()).toContain('Not found');
      expect(new URL(page.url()).pathname).toBe('/root/definitely-not-a-page');
    } finally {
      await page.close();
    }
  });

  it('stays put when the server serves it and no route matches', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await visit(page, `${panel.origin}/root%2Finstallations`, { mount: 5_000 });

      // Without the fallback the console does not know it is the console, and the
      // workspace resolver replaces this address with an installation.
      expect(new URL(page.url()).pathname, 'the panel navigated away').toBe(
        '/root%2Finstallations',
      );
    } finally {
      await page.close();
    }
  });
});
