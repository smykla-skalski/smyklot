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

describe('desktop confirmation preserves uncertain requests', () => {
  it.each(['light', 'dark'] as const)(
    'rejects unrelated receipts and only retries explicitly in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        const pending = {
          action: 'dispatch',
          request_key: 'original-dispatch-key',
          reason: 'Run reviewed changes',
          plan_id: 'original-plan',
          expected_revision: 3,
        };
        await page.addInitScript((input) => {
          const get = Storage.prototype.getItem;
          let seeded = false;
          Storage.prototype.getItem = function (key) {
            if (!seeded && key.startsWith('smyklot.sync-check.v1:')) {
              this.setItem(key, JSON.stringify(input));
              seeded = true;
            }
            return get.call(this, key);
          };
        }, pending);
        let mode = 'missing';
        const commands: unknown[] = [];
        await page.route('**/api/v1/targets/2001/sync/run-now', async (route) => {
          commands.push(route.request().postDataJSON());
          await route.fulfill({
            status: 202,
            json: {
              status: 'dispatch_accepted',
              plan_id: pending.plan_id,
              queue_id: 'original-worker',
              repeated: true,
            },
          });
        });
        await page.route(
          '**/api/v1/targets/2001/sync/requests/dispatch/original-dispatch-key',
          async (route) => {
            if (mode === 'missing' || mode === 'denied' || mode === 'error') {
              await route.fulfill({
                status: mode === 'missing' ? 404 : mode === 'denied' ? 403 : 500,
                json: { error: { code: 'lookup_failed', message: 'Request lookup failed' } },
              });
              return;
            }
            const acceptance = {
              ...pending,
              queue_id: 'original-worker',
              accepted_at: '2026-09-14T10:00:00Z',
              ...JSON.parse(mode),
            };
            await route.fulfill({
              json: {
                target_id: JSON.parse(mode).target_id ?? '2001',
                acceptance,
                observation_started_at: '2026-09-14T10:01:00Z',
                observed_at: '2026-09-14T10:01:00Z',
                comparison: null,
                plan: null,
                execution: null,
                check: { available: true },
                dispatch: null,
              },
            });
          },
        );
        await visit(page, addressOf(panel, 'workspace/sync'));
        const confirm = page.getByRole('button', { name: 'Confirm request', exact: true });
        const retry = page.getByRole('button', { name: 'Retry original request', exact: true });
        const saved = () =>
          page.evaluate(() =>
            Object.entries(sessionStorage)
              .filter(([key]) => key.startsWith('smyklot.sync-check.v1:'))
              .map(([, value]) => JSON.parse(value)),
          );
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-confirm-${scene}-${colorScheme}.png`),
          });
        };
        await confirm.click();
        await retry.waitFor();
        expect(commands).toEqual([]);
        expect(await saved()).toEqual([pending]);
        await capture('missing');
        for (const failure of ['denied', 'error']) {
          mode = failure;
          await confirm.click();
          await page.getByText('Request lookup failed', { exact: true }).waitFor();
          expect(await retry.count()).toBe(0);
          expect(await saved()).toEqual([pending]);
          expect(commands).toEqual([]);
          await capture(failure);
        }
        for (const mismatch of [
          { target_id: 'unrelated-target' },
          { request_key: 'unrelated-key' },
          { action: 'check', check_id: 'unrelated-check' },
          { reason: 'A different request' },
          { plan_id: 'unrelated-plan' },
          { expected_revision: 4 },
          { queue_id: '' },
        ]) {
          mode = JSON.stringify(mismatch);
          await confirm.click();
          await page
            .getByText(
              'The saved request could not be matched to this response. Keep it and try confirming again.',
              { exact: true },
            )
            .waitFor();
          expect(await saved()).toEqual([pending]);
          expect(commands).toEqual([]);
        }
        await capture('mismatch');
        mode = 'missing';
        await confirm.click();
        await retry.click();
        await page.getByRole('link', { name: 'View accepted changes', exact: true }).waitFor();
        expect(commands).toEqual([pending]);
        expect(await saved()).toEqual([]);
        await capture('explicit-retry');
      } finally {
        await page.close();
      }
    },
  );
});
