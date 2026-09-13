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

describe('desktop sync request intent', () => {
  it.each(['light', 'dark'] as const)(
    'keeps a check separate from approved dispatch in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        const endpoint = `${panel.origin}/api/v1/targets/2001/sync/plan`;
        const original = (await (await page.request.get(endpoint)).json()).plan;
        expect(original.state).toBe('approved');
        let hidePlan = true;
        await page.route('**/api/v1/targets/*/sync/plan', (route) =>
          hidePlan ? route.fulfill({ json: { plan: null } }) : route.continue(),
        );
        await visit(page, addressOf(panel, 'workspace/sync'));
        hidePlan = false;
        const checkRequest = page.waitForRequest(
          (request) => request.method() === 'POST' && request.url().endsWith('/sync/run-now'),
        );
        await page.getByRole('button', { name: 'Check now', exact: true }).click();
        expect((await checkRequest).postDataJSON()).toEqual({
          action: 'check',
          request_key: expect.any(String),
          reason: 'Check sync from the status view',
        });
        await page
          .getByText(
            'Earlier changes are still pending. Review them before requesting another check.',
            { exact: true },
          )
          .waitFor();
        const retained = (await (await page.request.get(endpoint)).json()).plan;
        expect(retained.queue_item).toEqual(original.queue_item);
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.screenshot({
            path: join(directory, `F33-explicit-intent-${scene}-${colorScheme}.png`),
          });
        };
        await capture('check-blocked');
        await page.getByRole('button', { name: 'View changes', exact: true }).click();
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        await inspector.getByRole('button', { name: 'Run now', exact: true }).click();
        const confirmation = page.getByRole('dialog', { name: 'Sync now?', exact: true });
        await confirmation.getByLabel('Reason', { exact: true }).fill('Run these reviewed changes');
        await capture('dispatch-confirmation');
        const dispatchRequest = page.waitForRequest(
          (request) => request.method() === 'POST' && request.url().endsWith('/sync/run-now'),
        );
        const dispatchResponse = page.waitForResponse(
          (response) =>
            response.request().method() === 'POST' && response.url().endsWith('/sync/run-now'),
        );
        await confirmation.getByRole('button', { name: 'Run now', exact: true }).click();
        expect((await dispatchResponse).status()).toBe(202);
        await expect
          .poll(() =>
            page.getByText('Your selected changes are queued to run now', { exact: true }).count(),
          )
          .toBe(1);
        expect((await dispatchRequest).postDataJSON()).toEqual({
          action: 'dispatch',
          plan_id: original.id,
          expected_revision: original.queue_item.revision,
          reason: 'Run these reviewed changes',
        });
        await inspector.getByRole('button', { name: 'Close sync details', exact: true }).click();
        await inspector.waitFor({ state: 'hidden' });
        await page
          .getByText('Your selected changes are queued to run now', { exact: true })
          .waitFor();
        await capture('dispatched');
      } finally {
        await page.close();
      }
    },
  );
});
