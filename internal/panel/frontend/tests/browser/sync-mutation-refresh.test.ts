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
describe('desktop sync mutation refresh', () => {
  it.each(['light', 'dark'] as const)(
    'refreshes a refused check without replacing historical evidence in %s',
    async (theme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme: theme,
        reducedMotion: 'reduce',
      });
      try {
        let submitted = false;
        let freshReads = 0;
        await page.route('**/api/v1/targets/2001/sync/plans/history-2', async (route) => {
          const response = await route.fetch();
          const body = await response.json();
          if (!submitted)
            body.check = {
              action: 'check',
              target_id: '2001',
              available: true,
              reason: 'available',
              effect: 'request_repository_check',
            };
          else freshReads++;
          await route.fulfill({ response, json: body });
        });
        await page.route('**/api/v1/targets/2001/sync/run-now', async (route) => {
          submitted = true;
          const response = await route.fetch();
          const body = await response.json();
          expect(body.status).toBe('changes_pending');
          expect(body.plan.id).not.toBe('history-2');
          await route.fulfill({ response });
        });
        const url = addressOf(panel, 'workspace/sync/history/history-2');
        await visit(page, url);
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        const check = inspector.getByRole('button', { name: 'Check repositories', exact: true });
        await check.click();
        const dialog = page.getByRole('dialog', { name: 'Check sync now?', exact: true });
        await dialog.getByLabel('Reason', { exact: true }).fill('Check current repositories');
        await dialog.getByRole('button', { name: 'Check now', exact: true }).click();
        await inspector
          .getByText('Review the current changes before requesting another check.', { exact: true })
          .waitFor();
        await inspector
          .getByRole('button', { name: 'View current changes', exact: true })
          .waitFor();
        expect(await check.isDisabled()).toBe(true);
        expect(freshReads).toBeGreaterThan(0);
        expect(page.url()).toBe(url);
        await inspector
          .getByRole('heading', { name: '14 changes processed', exact: true })
          .waitFor();
        const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-mutation-refresh-refused-${theme}.png`),
          });
        }
      } finally {
        await page.close();
      }
    },
  );
});
