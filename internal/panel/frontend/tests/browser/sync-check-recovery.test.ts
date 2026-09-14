import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel({ liveSync: true });
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop uncertain check recovery', () => {
  it.each(['light', 'dark'] as const)(
    'recovers the same acceptance after reload in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        const endpoint = `${panel.origin}/api/v1/targets/2001/sync/plan`;
        await expect
          .poll(async () => (await (await page.request.get(endpoint)).json()).plan, {
            timeout: 20_000,
          })
          .toBeNull();
        let originalInput: { request_key: string } | undefined;
        let acceptedId = '';
        let requests = 0;
        await page.route('**/api/v1/targets/2001/sync/run-now', async (route) => {
          requests++;
          const input = route.request().postDataJSON();
          const response = await route.fetch();
          const body = await response.json();
          if (requests === 1) {
            originalInput = input;
            expect(body.status).toBe('check_accepted');
            acceptedId = body.check_id;
            await route.abort('failed');
          } else {
            expect(input).toEqual(originalInput);
            expect(body).toEqual({
              status: 'check_accepted',
              check_id: acceptedId,
              repeated: true,
            });
            await route.fulfill({ response });
          }
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: 'Check now', exact: true }).click();
        const recover = page.getByRole('button', { name: 'Confirm request', exact: true });
        await recover.waitFor();
        expect(await page.getByRole('button', { name: 'Check now', exact: true }).count()).toBe(0);
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-check-recovery-${scene}-${colorScheme}.png`),
          });
        };
        await capture('lost-response');
        await page.reload();
        await recover.waitFor();
        expect(requests).toBe(1);
        await capture('reloaded');
        await recover.click();
        const link = page.getByRole('link', { name: 'View request', exact: true });
        await link.waitFor();
        expect(await link.getAttribute('href')).toContain(
          encodeURIComponent(originalInput!.request_key),
        );
        expect(await recover.count()).toBe(0);
        expect(requests).toBe(1);
        await capture('accepted');
        await link.click();
        await page.getByRole('dialog', { name: 'Sync request', exact: true }).waitFor();
        await page.getByRole('link', { name: 'View repository results', exact: true }).click();
        await page
          .getByRole('dialog')
          .getByRole('heading', { name: 'Repository check', exact: true })
          .waitFor();
        expect(decodeURIComponent(new URL(page.url()).pathname)).toContain(acceptedId);
        await page.reload();
        await page
          .getByRole('dialog')
          .getByRole('heading', { name: 'Repository check', exact: true })
          .waitFor();
        await page
          .getByRole('dialog')
          .getByRole('heading', { name: 'Check finished with gaps', exact: true })
          .waitFor();
        expect(requests).toBe(1);
      } finally {
        await page.close();
      }
    },
  );
});
