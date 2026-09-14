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

describe('desktop sync operation', () => {
  it.each(['light', 'dark'] as const)(
    'keeps the original request and read recovery in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        let mode = 'retained';
        let first = true;
        let release = () => {};
        const initial = new Promise<void>((resolve) => {
          release = resolve;
        });
        let mutations = 0;
        page.on('request', (request) => {
          if (new URL(request.url()).pathname.includes('/sync/') && request.method() !== 'GET')
            mutations++;
        });
        await page.route('**/api/v1/targets/2001/sync/requests/*/*', async (route) => {
          if (first) {
            first = false;
            await initial;
          }
          const dispatch = new URL(route.request().url()).pathname.includes('/dispatch/');
          const status =
            mode === 'error' ? 500 : mode === 'denied' ? 403 : mode === 'missing' ? 404 : 200;
          await route.fulfill({
            status,
            json:
              status !== 200
                ? { error: { code: 'read_failed', message: 'Read failed' } }
                : {
                    target_id: '2001',
                    observation_started_at: '2026-09-14T00:00:00Z',
                    observed_at: '2026-09-14T00:00:01Z',
                    acceptance: {
                      action: dispatch ? 'dispatch' : 'check',
                      request_key: dispatch ? 'dispatch-key' : 'check-key',
                      reason: dispatch
                        ? 'Apply reviewed repository defaults'
                        : 'Verify saved labels after review',
                      accepted_at: '2026-09-13T12:00:00Z',
                      ...(dispatch
                        ? {
                            plan_id: 'original-plan',
                            queue_id: 'original-worker',
                            expected_revision: 3,
                          }
                        : { check_id: 'original-check' }),
                    },
                    comparison:
                      dispatch || mode === 'unavailable' || mode === 'running'
                        ? null
                        : {
                            outcome: {
                              completed_at: '2026-09-13T12:01:00Z',
                              disposition: 'checked',
                              summary: 'Compared repository labels with the saved configuration',
                              counts: { matched: 2 },
                              cached: 0,
                              missing_permissions: [],
                            },
                          },
                    execution:
                      mode === 'failed' || mode === 'running'
                        ? {
                            state: mode === 'failed' ? 'failed' : 'running',
                            summary:
                              mode === 'failed'
                                ? dispatch
                                  ? 'The worker lost its lease after recording the plan outcome'
                                  : 'The worker lost its lease after recording the comparison'
                                : 'Checking repository labels',
                            progress_current: 1,
                            progress_total: 2,
                            attempt: 1,
                            started_at: '2026-09-13T12:00:00Z',
                            finished_at: null,
                          }
                        : null,
                    plan:
                      dispatch && mode !== 'unavailable'
                        ? {
                            id: 'original-plan',
                            trigger: 'manual',
                            state: 'applied',
                            counts: { create: 1, update: 2, delete: 0 },
                            computed_at: '2026-09-13T11:00:00Z',
                            finished_at: '2026-09-13T12:01:00Z',
                          }
                        : null,
                    check: {
                      action: 'check',
                      target_id: '2001',
                      available: true,
                      reason: 'available',
                      effect: 'request_repository_check',
                    },
                    dispatch: null,
                  },
          });
        });
        await visit(page, addressOf(panel, 'workspace/sync/request/check/check-key'));
        const dialog = page.getByRole('dialog', { name: 'Sync request', exact: true });
        const capture = async (scene: string) => {
          const dir = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!dir) return;
          await mkdir(dir, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({ path: join(dir, `F33-operation-${scene}-${colorScheme}.png`) });
        };
        await dialog.getByText('Loading request…', { exact: true }).waitFor();
        await capture('loading');
        release();
        await dialog.getByText('Verify saved labels after review', { exact: true }).waitFor();
        await capture('retained');
        const refresh = dialog.getByRole('button', { name: 'Refresh request', exact: true });
        for (const [next, text] of [
          ['failed', 'Execution failed'],
          ['running', 'Check in progress'],
          ['unavailable', 'Check outcome unavailable'],
          ['error', 'Request could not be loaded'],
          ['denied', 'Request access unavailable'],
          ['missing', 'Request not found'],
        ] as const) {
          mode = next;
          await refresh.focus();
          await page.keyboard.press('Enter');
          await dialog.getByText(text, { exact: true }).waitFor();
          await expect
            .poll(() => refresh.evaluate((el) => el === document.activeElement))
            .toBe(true);
          if (['error', 'denied', 'missing'].includes(next))
            expect(
              await dialog.getByText('Verify saved labels after review', { exact: true }).count(),
            ).toBe(0);
          await capture(next);
        }
        mode = 'running';
        await refresh.click();
        await dialog.getByText('Check in progress', { exact: true }).waitFor();
        await dialog.getByText('Execution details', { exact: true }).click();
        await dialog.getByText('1 of 2', { exact: true }).waitFor();
        await capture('execution');
        mode = 'retained';
        await page.reload();
        await dialog.getByText('Verify saved labels after review', { exact: true }).waitFor();
        await dialog.getByRole('link', { name: 'View repository results', exact: true }).click();
        await page.waitForURL(/\/check\/original-check$/u);
        await page.goBack();
        await dialog.getByText('Verify saved labels after review', { exact: true }).waitFor();
        await page.goto(addressOf(panel, 'workspace/sync/request/dispatch/dispatch-key'));
        await dialog.getByText('Changes applied', { exact: true }).waitFor();
        await capture('dispatch');
        expect(
          await dialog.getByRole('link', { name: 'View change results' }).getAttribute('href'),
        ).toContain('/history/original-plan');
        mode = 'failed';
        await refresh.click();
        await dialog.getByText('Plan recorded as applied', { exact: true }).waitFor();
        await capture('dispatch-failed');
        mode = 'unavailable';
        await refresh.click();
        await dialog.getByText('Change details unavailable', { exact: true }).waitFor();
        await capture('dispatch-unavailable');
        await dialog.getByRole('button', { name: 'Close request', exact: true }).click();
        await expect.poll(() => dialog.count()).toBe(0);
        const trigger = page.getByRole('button', { name: 'Your sync requests', exact: true });
        await expect
          .poll(() =>
            trigger.evaluate(
              () => document.activeElement?.tagName + ':' + document.activeElement?.id,
            ),
          )
          .toBe('BUTTON:sync-requests-trigger');
        await capture('closed');
        await page.goto(addressOf(panel, 'workspace/sync/request/check/check-key'));
        await dialog.waitFor();
        await page.keyboard.press('Escape');
        await expect.poll(() => dialog.count()).toBe(0);
        await expect.poll(() => trigger.evaluate((el) => el === document.activeElement)).toBe(true);
        expect(mutations).toBe(0);
      } finally {
        await page.close();
      }
    },
  );
});
