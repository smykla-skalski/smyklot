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
    it.each([
      'available',
      'plan_expired',
      'queue_unavailable',
      'admin_or_owner_required',
      'already_running',
    ] as const)(`explains %s in ${theme}`, async (reason) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme: theme,
        reducedMotion: 'reduce',
      });
      try {
        let outcomeReasons: 'missing' | 'dependency' | 'disabled' = 'missing';
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
            if (reason === 'already_running') {
              body.plan.state = 'applying';
              const fileIndex = body.plan.actions.findIndex(
                (action: Record<string, unknown>, index: number) =>
                  index > 2 && action.kind === 'files',
              );
              expect(fileIndex).toBeGreaterThan(2);
              body.plan.actions = body.plan.actions.map(
                (action: Record<string, unknown>, index: number) => ({
                  ...action,
                  state:
                    index === fileIndex
                      ? 'applied'
                      : (['applied', 'failed', 'skipped'][index] ?? 'pending'),
                  ...(index === fileIndex
                    ? {
                        proposal_url:
                          outcomeReasons === 'disabled'
                            ? ''
                            : 'https://github.com/example/repo/pull/42',
                      }
                    : {}),
                  ...(index === 1 && outcomeReasons !== 'missing'
                    ? { error: 'GitHub denied this label change.' }
                    : {}),
                  ...(index === 2 && outcomeReasons !== 'missing'
                    ? {
                        blocker:
                          outcomeReasons === 'dependency'
                            ? 'labels'
                            : 'Sync is disabled for this category.',
                      }
                    : {}),
                }),
              );
              body.plan.queue_item = {
                ...body.plan.queue_item,
                state: 'running',
                summary: 'Applying repository changes',
                progress_current: 4,
                progress_total: 14,
                attempt: 2,
              };
            }
          }
          await route.fulfill({ response, json: body });
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: 'View changes', exact: true }).click();
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        await inspector.waitFor();
        const run = inspector.getByRole('button', { name: 'Run now', exact: true });
        if (unavailable || reason === 'already_running') expect(await run.count()).toBe(0);
        else expect(await run.isDisabled()).toBe(reason !== 'available');
        const text = {
          available: 'Run now skips the scheduling window.',
          already_running: 'These changes are already running.',
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
        if (reason === 'already_running') {
          await inspector
            .getByRole('heading', { name: '4 of 14 changes processed', exact: true })
            .waitFor({ timeout: 5000 });
          await inspector
            .getByText('2 succeeded · 1 failed · 1 skipped', { exact: true })
            .waitFor();
          await inspector
            .getByText('Failed: No failure details were recorded.', { exact: true })
            .waitFor();
          await inspector.getByText('Skipped: No reason was recorded.', { exact: true }).waitFor();
          const details = inspector.locator('details').filter({ hasText: 'Scheduling details' });
          expect(await details.getAttribute('open')).toBeNull();
          await inspector.getByText('Succeeded', { exact: true }).waitFor();
          await inspector.getByText('Proposed in a pull request', { exact: true }).waitFor();
          // The first repository is expanded; the other repositories keep their rows collapsed.
          expect(await inspector.getByText('Pending', { exact: true }).count()).toBe(2);
          const directory = process.env.SMYKLOT_SYNC_ACTION_OUTCOME_SCREENSHOTS;
          if (directory) {
            await mkdir(directory, { recursive: true });
            await page.screenshot({ path: join(directory, `F33-action-outcomes-${theme}.png`) });
          }
          for (const mode of ['dependency', 'disabled'] as const) {
            outcomeReasons = mode;
            await page.reload({ waitUntil: 'domcontentloaded' });
            await inspector
              .getByText('Failed: GitHub denied this label change.', { exact: true })
              .waitFor({ timeout: 5000 });
            await inspector
              .getByText(
                mode === 'dependency'
                  ? 'Skipped because labels failed earlier in this repository.'
                  : 'Skipped: Sync is disabled for this category.',
                { exact: true },
              )
              .waitFor({ timeout: 5000 });
            await inspector
              .getByText(
                mode === 'disabled'
                  ? 'Completed; no pull request link recorded'
                  : 'Proposed in a pull request',
                { exact: true },
              )
              .waitFor();
            if (directory)
              await page.screenshot({
                path: join(directory, `F33-action-outcomes-${mode}-${theme}.png`),
              });
          }
          expect(posts).toBe(0);
        }
        if (reason === 'available') {
          const details = inspector.locator('details').filter({ hasText: 'Scheduling details' });
          const summary = details.locator('summary');
          expect(await details.getAttribute('open')).toBeNull();
          expect(
            await inspector.getByText('Runs no earlier than', { exact: true }).isVisible(),
          ).toBe(false);
          const directory = process.env.SMYKLOT_SCHEDULE_HIERARCHY_SCREENSHOTS;
          const captureSchedule = async (scene: string) => {
            if (!directory) return;
            await mkdir(directory, { recursive: true });
            await page.mouse.move(1900, 20);
            await page.screenshot({
              path: join(directory, `F35-schedule-${scene}-${theme}.png`),
            });
          };
          await captureSchedule('collapsed');
          await summary.focus();
          await summary.press('Enter');
          await inspector.getByText('Runs no earlier than', { exact: true }).waitFor();
          expect(await run.isVisible()).toBe(true);
          await captureSchedule('expanded');
          await summary.press('Enter');
          expect(
            await inspector.getByText('Runs no earlier than', { exact: true }).isVisible(),
          ).toBe(false);
          expect(await summary.evaluate((element) => element === document.activeElement)).toBe(
            true,
          );
          expect(posts).toBe(0);
          await run.click();
          await page.getByRole('dialog', { name: 'Sync now?', exact: true }).waitFor();
          await capture('confirmation');
        }
      } finally {
        await page.close();
      }
    });
  }
});
