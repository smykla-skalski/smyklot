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

describe('desktop delayed sync acceptance', () => {
  it.each(['light', 'dark'] as const)(
    'receives the original acceptance after inspector navigation in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      let submitted = '';
      let posts = 0;
      try {
        await page.route('**/api/v1/targets/2001/sync/run-now', async (route) => {
          posts++;
          submitted = route.request().postDataJSON().request_key;
          const response = await route.fetch();
          expect((await response.json()).status).toBe('dispatch_accepted');
          await held;
          await route.fulfill({ response });
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: 'View changes', exact: true }).click();
        const changes = page.getByRole('dialog', { name: 'Sync details', exact: true });
        await changes.getByRole('button', { name: 'Run now', exact: true }).click();
        const confirmation = page.getByRole('dialog', { name: 'Sync now?', exact: true });
        await confirmation
          .getByLabel('Reason', { exact: true })
          .fill('Follow this request after navigating');
        await confirmation.getByRole('button', { name: 'Run now', exact: true }).click();
        await expect.poll(() => posts).toBe(1);
        await changes.getByRole('button', { name: 'Close sync details', exact: true }).click();
        await changes.waitFor({ state: 'hidden' });
        expect(
          await page.getByRole('button', { name: 'Confirm request', exact: true }).count(),
        ).toBe(0);
        release();
        const link = page.getByRole('link', { name: 'View request', exact: true });
        await link.waitFor();
        expect(await link.getAttribute('href')).toContain(submitted);
        await expect
          .poll(() => page.getByRole('button', { name: 'Confirm request', exact: true }).count())
          .toBe(0);
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-late-acceptance-${scene}-${colorScheme}.png`),
          });
        };
        await capture('received');
        await link.click();
        const inspector = page.getByRole('dialog', { name: 'Sync request', exact: true });
        await inspector
          .getByText('Follow this request after navigating', { exact: true })
          .waitFor();
        await capture('request');
        expect(posts).toBe(1);
      } finally {
        release();
        await page.close();
      }
    },
  );
});
