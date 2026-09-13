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
describe('desktop dispatch capability guidance', () => {
  for (const theme of ['light', 'dark'] as const) {
    it.each(['available', 'plan_expired', 'queue_unavailable', 'admin_or_owner_required'] as const)(
      `explains %s in ${theme}`,
      async (reason) => {
        const page = await panel.browser.newPage({
          viewport: { width: 1920, height: 1200 },
          colorScheme: theme,
          reducedMotion: 'reduce',
        });
        try {
          let reads = 0;
          let posts = 0;
          let release: (() => void) | undefined;
          const unavailable = ['plan_expired', 'queue_unavailable'].includes(reason);
          page.on('request', (request) => {
            if (request.method() === 'POST' && request.url().includes('/sync/')) posts++;
          });
          await page.route('**/api/v1/targets/2001/sync/plans/*', async (route) => {
            if (route.request().method() !== 'GET') return route.continue();
            const response = await route.fetch();
            const body = await response.json();
            if (body.plan) {
              reads++;
              if (unavailable && reads > 1) {
                await new Promise<void>((resolve) => {
                  release = resolve;
                });
                await route.fulfill({ response, json: body });
                return;
              }
              expect(body.plan.dispatch).toBeDefined();
              body.plan.dispatch = {
                ...body.plan.dispatch,
                reason,
                available: reason === 'available',
              };
              if (reason === 'queue_unavailable') delete body.plan.queue_item;
            }
            await route.fulfill({ response, json: body });
          });
          await visit(page, addressOf(panel, 'workspace/sync'));
          await page.getByRole('button', { name: 'View changes', exact: true }).click();
          const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
          await inspector.waitFor();
          const run = inspector.getByRole('button', { name: 'Run now', exact: true });
          if (unavailable) expect(await run.count()).toBe(0);
          else expect(await run.isDisabled()).toBe(reason !== 'available');
          const text = {
            available: 'Run now skips the scheduling window.',
            plan_expired: 'These changes have expired.',
            queue_unavailable: 'The execution record is unavailable.',
            admin_or_owner_required: 'An Admin or Owner can request',
          }[reason];
          await inspector.getByText(text, { exact: false }).waitFor();
          const capture = async (scene: string) => {
            const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
            if (!directory || !unavailable) return;
            await mkdir(directory, { recursive: true });
            await page.mouse.move(1900, 20);
            await page.screenshot({
              path: join(
                directory,
                scene === reason
                  ? `F33-dispatch-capability-${scene}-${theme}.png`
                  : `F33-current-status-${reason}-${scene}-${theme}.png`,
              ),
            });
          };
          if (unavailable) {
            await inspector
              .getByRole('heading', {
                name:
                  reason === 'plan_expired'
                    ? 'These changes have expired'
                    : 'Execution status unavailable',
                exact: true,
              })
              .waitFor();
            expect(
              await inspector
                .getByRole('heading', { name: 'Execution schedule', exact: true })
                .count(),
            ).toBe(0);
          }
          await capture(reason);
          if (unavailable) {
            const url = page.url();
            await inspector.getByRole('button', { name: 'Refresh status', exact: true }).click();
            await inspector.getByRole('button', { name: 'Refreshing…', exact: true }).waitFor();
            await capture('refreshing');
            await expect.poll(() => release !== undefined).toBe(true);
            release!();
            await inspector.getByRole('button', { name: 'Run now', exact: true }).waitFor();
            await expect
              .poll(() =>
                inspector
                  .getByRole('heading', { name: /changes queued$/ })
                  .evaluate((heading) => document.activeElement === heading),
              )
              .toBe(true);
            expect(page.url()).toBe(url);
            expect(posts).toBe(0);
            await capture('recovered');
          }
          if (reason === 'available') {
            await run.click();
            await page.getByRole('dialog', { name: 'Sync now?', exact: true }).waitFor();
            await capture('confirmation');
          }
        } finally {
          await page.close();
        }
      },
    );
  }
});
