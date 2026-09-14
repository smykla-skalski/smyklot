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

describe('desktop sync storage recovery', () => {
  it.each(['light', 'dark'] as const)(
    'recovers the known identity through storage failure in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        const pending = {
          action: 'check',
          request_key: 'saved-before-response-loss',
          reason: 'Verify original labels',
        };
        await page.addInitScript((input) => {
          const state = window as unknown as { blockSyncStorage: boolean };
          state.blockSyncStorage = true;
          const get = Storage.prototype.getItem;
          const set = Storage.prototype.setItem;
          let seeded = false;
          Storage.prototype.getItem = function (key) {
            if (key.startsWith('smyklot.sync-check.v1:')) {
              if (!seeded) {
                set.call(this, key, JSON.stringify(input));
                seeded = true;
              }
              if (state.blockSyncStorage) throw new Error('Storage temporarily unavailable');
            }
            return get.call(this, key);
          };
        }, pending);
        const commands: unknown[] = [];
        await page.route('**/api/v1/targets/2001/sync/run-now', async (route) => {
          commands.push(route.request().postDataJSON());
          await route.fulfill({
            status: 202,
            json: { status: 'check_accepted', check_id: 'original-check', repeated: true },
          });
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-storage-${scene}-${colorScheme}.png`),
          });
        };
        const retry = page.getByRole('button', { name: 'Retry browser storage', exact: true });
        await retry.waitFor();
        await retry.click();
        expect(commands).toHaveLength(0);
        await capture('unavailable');
        await page.evaluate(() => {
          (window as unknown as { blockSyncStorage: boolean }).blockSyncStorage = false;
        });
        await retry.click();
        const recover = page.getByRole('button', { name: 'Recover check', exact: true });
        await recover.waitFor();
        expect(commands).toHaveLength(0);
        await capture('known');
        await page.evaluate(() => {
          (window as unknown as { blockSyncStorage: boolean }).blockSyncStorage = true;
        });
        await recover.click();
        await page
          .getByText(
            'The response was received, but its saved request could not be cleared. New requests are paused.',
            { exact: true },
          )
          .waitFor();
        expect(commands).toEqual([pending]);
        await page.getByRole('link', { name: 'View check', exact: true }).waitFor();
        await capture('cleanup-blocked');
        await page.evaluate(() => {
          (window as unknown as { blockSyncStorage: boolean }).blockSyncStorage = false;
        });
        await page.getByRole('button', { name: 'Finish recovery', exact: true }).click();
        await expect.poll(() => recover.count()).toBe(0);
        expect(commands).toEqual([pending]);
        await expect
          .poll(() =>
            page.evaluate(
              () =>
                Object.keys(sessionStorage).filter((key) =>
                  key.startsWith('smyklot.sync-check.v1:'),
                ).length,
            ),
          )
          .toBe(0);
        await page.getByRole('link', { name: 'View check', exact: true }).waitFor();
        await capture('confirmed');
      } finally {
        await page.close();
      }
    },
  );
});
