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
