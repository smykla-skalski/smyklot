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
describe('desktop exact dispatch recovery', () => {
  it.each(['light', 'dark'] as const)(
    'recovers accepted changes after reload in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10000);
      try {
        let submitted: unknown;
        let accepted: { plan_id: string; queue_id: string } | undefined;
        let count = 0;
        await page.route('**/api/v1/targets/2001/sync/run-now', async (route) => {
          count++;
          const input = route.request().postDataJSON();
          const response = await route.fetch();
          const body = await response.json();
          expect(body.status).toBe('dispatch_accepted');
          if (count === 1) {
            submitted = input;
            accepted = body;
            await route.abort('failed');
          } else {
            expect(input).toEqual(submitted);
            expect(body).toEqual({ ...accepted, repeated: true });
            await route.fulfill({ response });
          }
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: 'View changes', exact: true }).click();
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        await inspector.getByRole('button', { name: 'Run now', exact: true }).click();
        const confirmation = page.getByRole('dialog', { name: 'Sync now?', exact: true });
        await confirmation.getByLabel('Reason', { exact: true }).fill('Run the selected changes');
        await confirmation.getByRole('button', { name: 'Run now', exact: true }).click();
        await inspector.getByRole('button', { name: 'Recover run request', exact: true }).waitFor();
        expect(
          await inspector.getByRole('button', { name: 'Run now', exact: true }).isDisabled(),
        ).toBe(true);
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-dispatch-recovery-${scene}-${colorScheme}.png`),
          });
        };
        await capture('lost-response-inspector');
        await inspector.getByRole('button', { name: 'Close sync details', exact: true }).click();
        await inspector.waitFor({ state: 'hidden' });
        await page.waitForURL(/\/sync$/);
        await page.reload();
        const recover = page.getByRole('button', { name: 'Recover run request', exact: true });
        await recover.waitFor();
        expect(count).toBe(1);
        await capture('reloaded');
        await recover.click();
        const link = page.getByRole('link', { name: 'View accepted changes', exact: true });
        await link.waitFor();
        expect(await link.getAttribute('href')).toContain(encodeURIComponent(accepted!.plan_id));
        expect(count).toBe(2);
        await capture('accepted');
        await link.click();
        await inspector.waitFor();
        expect(decodeURIComponent(new URL(page.url()).pathname)).toContain(accepted!.plan_id);
        await capture('original-changes');
        await page.reload();
        await inspector.waitFor();
        expect(count).toBe(2);
      } finally {
        await page.close();
      }
    },
  );
  it.each(['light', 'dark'] as const)(
    'allows a fresh intent after a definite rejection in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10000);
      try {
        let rejectedKey = '';
        let count = 0;
        await page.route('**/api/v1/targets/2001/sync/run-now', async (route) => {
          count++;
          const input = route.request().postDataJSON();
          if (count === 1) {
            rejectedKey = input.request_key;
            const response = await route.fetch({
              postData: JSON.stringify({
                ...input,
                expected_revision: input.expected_revision + 1,
              }),
            });
            expect(response.status()).toBe(409);
            await route.fulfill({ response });
          } else {
            expect(input.request_key).not.toBe(rejectedKey);
            await route.continue();
          }
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: 'View changes', exact: true }).click();
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        const submit = async () => {
          await inspector.getByRole('button', { name: 'Run now', exact: true }).click();
          const confirmation = page.getByRole('dialog', { name: 'Sync now?', exact: true });
          await confirmation
            .getByLabel('Reason', { exact: true })
            .fill('Run current reviewed changes');
          await confirmation.getByRole('button', { name: 'Run now', exact: true }).click();
        };
        await submit();
        await inspector
          .getByText('sync queue item changed; review the latest state', { exact: true })
          .waitFor();
        expect(
          await inspector.getByRole('button', { name: 'Recover run request', exact: true }).count(),
        ).toBe(0);
        await expect
          .poll(() => inspector.getByRole('button', { name: 'Run now', exact: true }).isDisabled())
          .toBe(false);
        const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-dispatch-recovery-rejected-${colorScheme}.png`),
          });
        }
        await submit();
        await page
          .getByText('Your request to run these changes was accepted', { exact: true })
          .waitFor();
        expect(count).toBe(2);
      } finally {
        await page.close();
      }
    },
  );
});
