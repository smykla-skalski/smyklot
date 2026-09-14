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
describe('desktop empty sync recovery', () => {
  for (const theme of ['light', 'dark'] as const) {
    it.each(['missing', 'running'] as const)(
      `explains %s capability in ${theme}`,
      async (state) => {
        const page = await panel.browser.newPage({
          viewport: { width: 1920, height: 1200 },
          colorScheme: theme,
          reducedMotion: 'reduce',
        });
        try {
          let refreshed = false;
          let posts = 0;
          page.on('request', (request) => {
            if (request.method() === 'POST' && request.url().includes('/sync/')) posts++;
          });
          await page.route('**/api/v1/targets/2001/sync/plan', async (route) => {
            const check =
              state === 'running'
                ? {
                    action: 'check',
                    target_id: '2001',
                    available: false,
                    reason: 'check_running',
                    effect: 'request_repository_check',
                    running_check_id: 'current-check',
                  }
                : refreshed
                  ? {
                      action: 'check',
                      target_id: '2001',
                      available: true,
                      reason: 'available',
                      effect: 'request_repository_check',
                    }
                  : undefined;
            await route.fulfill({ json: { plan: null, check } });
          });
          await visit(page, addressOf(panel, 'workspace/sync/plan'));
          const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
          await inspector
            .getByRole('heading', { name: 'No changes to review', exact: true })
            .waitFor();
          const check = inspector.getByRole('button', { name: 'Check repositories', exact: true });
          expect(await check.isDisabled()).toBe(true);
          expect(
            await inspector.getByText('No sync is in progress', { exact: false }).count(),
          ).toBe(0);
          const capture = async (scene: string) => {
            const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
            if (!directory) return;
            await mkdir(directory, { recursive: true });
            await page.mouse.move(1900, 20);
            await page.screenshot({
              path: join(directory, `F33-empty-recovery-${scene}-${theme}.png`),
            });
          };
          if (state === 'running') {
            await inspector
              .getByText('A repository check is already running.', { exact: false })
              .waitFor();
            await inspector
              .getByRole('button', { name: 'View running check', exact: true })
              .waitFor();
            await capture('running');
            await inspector
              .getByRole('button', { name: 'View running check', exact: true })
              .click();
            await page.waitForURL(addressOf(panel, 'workspace/sync/check/current-check'));
          } else {
            await inspector
              .getByText('Refresh status to check whether', { exact: false })
              .waitFor();
            await capture('missing');
            const url = page.url();
            refreshed = true;
            await inspector.getByRole('button', { name: 'Refresh status', exact: true }).click();
            await expect.poll(() => check.isDisabled()).toBe(false);
            expect(page.url()).toBe(url);
            await capture('available');
          }
          expect(posts).toBe(0);
        } finally {
          await page.close();
        }
      },
    );
  }
});
