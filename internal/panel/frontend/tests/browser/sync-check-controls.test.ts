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
describe('desktop fresh check recovery', () => {
  for (const theme of ['light', 'dark'] as const) {
    it(`opens the current blocker from a historical result in ${theme}`, async () => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme: theme,
        reducedMotion: 'reduce',
      });
      try {
        await visit(page, addressOf(panel, 'workspace/sync/history/history-2'));
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        await inspector.waitFor();
        const response = await page.request.get(`${panel.origin}/api/v1/targets/2001/sync/plan`);
        const current = await response.json();
        expect(current.check.reason).toBe('changes_pending');
        expect(current.check.blocking_plan_id).toBe(current.plan.id);
        expect(
          await inspector
            .getByRole('button', { name: 'Check repositories', exact: true })
            .isDisabled(),
        ).toBe(true);
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-check-controls-${scene}-${theme}.png`),
          });
        };
        await capture('blocked-history');
        await inspector.getByRole('button', { name: 'View current changes', exact: true }).click();
        await page.waitForURL(addressOf(panel, `workspace/sync/history/${current.plan.id}`));
        await inspector.getByRole('button', { name: 'Run now', exact: true }).waitFor();
        await capture('current-blocker');
      } finally {
        await page.close();
      }
    });
    it(`confirms explicit check intent from expired work in ${theme}`, async () => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme: theme,
        reducedMotion: 'reduce',
      });
      try {
        await page.route('**/api/v1/targets/2001/sync/plans/*', async (route) => {
          if (route.request().method() !== 'GET') return route.continue();
          const response = await route.fetch();
          const body = await response.json();
          body.plan.dispatch = { ...body.plan.dispatch, available: false, reason: 'plan_expired' };
          body.check = {
            action: 'check',
            target_id: '2001',
            available: true,
            reason: 'available',
            effect: 'request_repository_check',
          };
          await route.fulfill({ response, json: body });
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: 'View changes', exact: true }).click();
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        const check = inspector.getByRole('button', { name: 'Check repositories', exact: true });
        await check.waitFor();
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-check-controls-${scene}-${theme}.png`),
          });
        };
        await capture('expired');
        await check.click();
        const dialog = page.getByRole('dialog', { name: 'Check sync now?', exact: true });
        await dialog.waitFor();
        await dialog
          .getByLabel('Reason', { exact: true })
          .fill('Check the current saved configuration');
        await capture('confirmation');
        const request = page.waitForRequest(
          (request) => request.method() === 'POST' && request.url().endsWith('/sync/run-now'),
        );
        await dialog.getByRole('button', { name: 'Check now', exact: true }).click();
        const sent = (await request).postDataJSON();
        expect(sent).toMatchObject({
          action: 'check',
          request_key: expect.any(String),
          reason: 'Check the current saved configuration',
        });
        expect(sent.plan_id).toBeUndefined();
        expect(sent.expected_revision).toBeUndefined();
      } finally {
        await page.close();
      }
    });
  }
});
