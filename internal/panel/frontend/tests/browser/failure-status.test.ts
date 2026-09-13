import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

describe('desktop failure recovery guidance', () => {
  it.each(
    [
      'root',
      'root/history/failures',
      'workspace/history/failures',
      'root/workspaces/{account}/history/failures',
    ].flatMap((route) =>
      (['light', 'dark'] as const).map((colorScheme) => ({ route, colorScheme })),
    ),
  )(
    'separates failure classification from automatic retry at $route in $colorScheme',
    async ({ route, colorScheme }) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(5000);
      try {
        await visit(page, addressOf(panel, route), { ready: '.object-row .pill' });
        expect(await page.locator('html').getAttribute('data-theme')).toBe(colorScheme);
        const rows =
          route === 'root'
            ? page
                .locator('.card')
                .filter({
                  has: page.getByRole('heading', { name: 'Recent failures', exact: true }),
                })
                .locator('.object-row')
            : page.locator('.history-panel .object-row');
        await rows.first().waitFor();
        const recoverable = rows.filter({ hasText: 'May succeed on retry' });
        const terminal = rows.filter({ hasText: 'Needs a fix' });
        expect(await recoverable.count()).toBeGreaterThan(0);
        expect(await terminal.count()).toBeGreaterThan(0);
        for (const row of await recoverable.all()) {
          expect(await row.innerText()).toContain('Automatic retries have stopped');
        }
        for (const row of await terminal.all()) {
          expect(await row.innerText()).toContain('Fix the cause before retrying');
        }
        expect((await rows.allTextContents()).join(' ')).not.toMatch(/Retrying|retries on its own/);

        const inspect = rows.getByRole('button', { name: 'Inspect queue item' }).first();
        await inspect.click();
        const dialog = page.getByRole('dialog');
        await dialog.getByRole('heading', { name: /^Process / }).waitFor();
        expect(await dialog.locator('dd').allTextContents()).toContain('Failed');
        expect(await dialog.innerText()).toContain('repository configuration is invalid');
        await page.keyboard.press('Escape');
        await expect.poll(() => dialog.count()).toBe(0);
        await expect
          .poll(() => inspect.evaluate((element) => element === document.activeElement))
          .toBe(true);

        if (route !== 'root') {
          const filter = page.getByRole('radio', { name: /^May succeed on retry/ });
          await filter.locator('..').click();
          await expect.poll(() => recoverable.count()).toBeGreaterThan(0);
          expect(await terminal.count()).toBe(0);
          expect((await rows.allTextContents()).join(' ')).not.toMatch(
            /Retrying|retries on its own/,
          );
        }
      } finally {
        await page.close();
      }
    },
  );
});

describe('desktop failure queue record lifecycle', () => {
  it('revalidates a retained record and hides actions for expired history', async () => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    try {
      await visit(page, addressOf(panel, 'root/history/failures'), { ready: '.object-row .pill' });
      const rows = page.locator('.history-panel .object-row');
      expect(await rows.count()).toBeGreaterThan(2);
      expect(await rows.nth(2).getByRole('button', { name: 'Inspect queue item' }).count()).toBe(0);
      const inspect = page.getByRole('button', { name: 'Inspect queue item' }).first();
      await inspect.click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('heading', { name: /^Process / }).waitFor();
      await page.keyboard.press('Escape');
      await expect.poll(() => dialog.count()).toBe(0);
      await page.route(/\/api\/v1\/root\/queue\/[^/?]+$/, async (route) => {
        const response = await route.fetch();
        const detail = await response.json();
        detail.item.state = 'running';
        await route.fulfill({ json: detail });
      });
      await inspect.click();
      await expect.poll(() => dialog.locator('dd').allTextContents()).toContain('Running');
      expect(await dialog.locator('dd').allTextContents()).not.toContain('Failed');
    } finally {
      await page.close();
    }
  });

  it.each([403, 404, 503])('handles a queue lookup returning %s', async (status) => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    page.setDefaultTimeout(5000);
    let requests = 0;
    try {
      await visit(page, addressOf(panel, 'root/history/failures'), { ready: '.object-row .pill' });
      await page.route(/\/api\/v1\/root\/queue\/[^/?]+$/, async (route) => {
        requests += 1;
        if (requests === 1) {
          await route.fulfill({
            status,
            json: { error: { code: 'lookup_failed', message: 'Test lookup failure' } },
          });
        } else {
          await route.continue();
        }
      });
      const inspect = page.getByRole('button', { name: 'Inspect queue item' }).first();
      await inspect.click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('alert').waitFor();
      expect(await dialog.getByRole('alert').innerText()).toContain(
        status === 404
          ? 'no longer available'
          : status === 403
            ? 'no longer have access'
            : 'could not be loaded',
      );
      expect(requests).toBe(1);
      if (status === 503) {
        await dialog.getByRole('button', { name: 'Try again' }).click();
        await dialog.getByRole('heading', { name: /^Process / }).waitFor();
        expect(requests).toBe(2);
      } else {
        expect(await dialog.getByRole('button', { name: 'Try again' }).count()).toBe(0);
      }
      await page.keyboard.press('Escape');
      await expect.poll(() => dialog.count()).toBe(0);
      await expect
        .poll(() => inspect.evaluate((element) => element === document.activeElement))
        .toBe(true);
    } finally {
      await page.close();
    }
  });
});
