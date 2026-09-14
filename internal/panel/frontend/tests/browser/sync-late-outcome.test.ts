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

const outcomes = [
  {
    name: 'approval',
    status: 200,
    body: { status: 'approval_required' },
    message: 'No run was started because these changes needed approval.',
    uncertain: false,
  },
  {
    name: 'running',
    status: 200,
    body: { status: 'already_running' },
    message: 'No new run was started because these changes were already running.',
    uncertain: false,
  },
  {
    name: 'rejected',
    status: 409,
    body: {
      error: {
        code: 'stale_revision',
        message: 'The reviewed revision has changed. Review the current changes.',
      },
    },
    message: 'The reviewed revision has changed. Review the current changes.',
    uncertain: false,
  },
  {
    name: 'unavailable',
    status: 503,
    body: {
      error: {
        code: 'unavailable',
        message: 'The sync service is unavailable. Confirm this request before retrying.',
      },
    },
    message: 'The sync service is unavailable. Confirm this request before retrying.',
    uncertain: true,
  },
] as const;

describe('desktop delayed sync outcomes', () => {
  for (const outcome of outcomes) {
    it.each(['light', 'dark'] as const)(
      `retains ${outcome.name} after navigation in %s`,
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
        let posts = 0;
        let originalPlan: { id: string };
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-lifecycle-${scene}-${colorScheme}.png`),
          });
        };
        try {
          originalPlan = (
            await (await page.request.get(`${panel.origin}/api/v1/targets/2001/sync/plan`)).json()
          ).plan;
          await page.route('**/api/v1/targets/2001/sync/run-now', async (route) => {
            posts++;
            await held;
            await route.fulfill({
              status: outcome.status,
              json: { ...outcome.body, ...(outcome.status === 200 ? { plan: originalPlan } : {}) },
            });
          });
          await visit(page, addressOf(panel, 'workspace/sync'));
          await page.getByRole('button', { name: 'View changes', exact: true }).click();
          const changes = page.getByRole('dialog', { name: 'Sync details', exact: true });
          await changes.getByRole('button', { name: 'Run now', exact: true }).click();
          const confirmation = page.getByRole('dialog', { name: 'Sync now?', exact: true });
          await confirmation
            .getByLabel('Reason', { exact: true })
            .fill('Keep the outcome after navigation');
          await confirmation.getByRole('button', { name: 'Run now', exact: true }).click();
          await expect.poll(() => posts).toBe(1);
          await changes.getByRole('button', { name: 'Close sync details', exact: true }).click();
          await changes.waitFor({ state: 'hidden' });
          expect(
            await page.getByRole('button', { name: 'Confirm request', exact: true }).count(),
          ).toBe(0);
          await page.getByText('Sending your sync request…', { exact: true }).waitFor();
          if (outcome.name === 'approval') await capture('sending');
          const response = page.waitForResponse('**/api/v1/targets/2001/sync/run-now');
          release();
          await response;
          await page.getByText(outcome.message, { exact: true }).waitFor();
          await expect
            .poll(() => page.getByRole('button', { name: 'Confirm request', exact: true }).count())
            .toBe(outcome.uncertain ? 1 : 0);
          expect(await page.getByRole('link', { name: 'View request', exact: true }).count()).toBe(
            0,
          );
          expect(posts).toBe(1);
          await capture(outcome.name);
          if (outcome.status === 200) {
            const link = page.getByRole('link', {
              name: outcome.name === 'approval' ? 'Review changes' : 'View changes',
              exact: true,
            });
            expect(await link.getAttribute('href')).toContain(originalPlan.id);
            const directory = process.env.SMYKLOT_SYNC_RELATED_PLAN_SCREENSHOTS;
            if (directory) {
              await mkdir(directory, { recursive: true });
              await page.screenshot({
                path: join(directory, `F33-related-plan-${outcome.name}-${colorScheme}.png`),
              });
            }
            await link.click();
            await page.getByRole('dialog', { name: 'Sync details', exact: true }).waitFor();
            await page
              .getByRole('dialog', { name: 'Sync details', exact: true })
              .getByRole('button', { name: 'Run now', exact: true })
              .waitFor();
            expect(await link.count()).toBe(0);
            if (directory)
              await page.screenshot({
                path: join(
                  directory,
                  `F33-related-plan-${outcome.name}-inspector-${colorScheme}.png`,
                ),
              });
            expect(page.url()).toContain(originalPlan.id);
            expect(posts).toBe(1);
          }
        } finally {
          release();
          await page.close();
        }
      },
    );
  }
});
