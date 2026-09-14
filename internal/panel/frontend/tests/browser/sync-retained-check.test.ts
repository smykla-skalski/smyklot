import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, type Panel } from './harness';
let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop retained comparison inspector', () => {
  it.each(['light', 'dark'] as const)(
    'separates retained, empty and unavailable facts in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      try {
        let state = 'retained';
        let releaseInitial: (() => void) | undefined;
        const initialRead = new Promise<void>((resolve) => {
          releaseInitial = resolve;
        });
        let firstRead = true;
        const body = () => ({
          check_id: 'retained-check',
          target_id: '2001',
          observed_at: '2026-09-14T00:00:00Z',
          result:
            state === 'running'
              ? null
              : {
                  outcome: {
                    completed_at: '2026-09-13T12:00:00Z',
                    disposition: 'checked',
                    summary:
                      state === 'empty'
                        ? 'No enabled repositories to check'
                        : 'Compared repository labels with the saved configuration',
                    counts: state === 'empty' ? {} : { matched: 1 },
                    cached: 0,
                    missing_permissions: [],
                  },
                },
          execution:
            state === 'failed' || state === 'running'
              ? {
                  state: state === 'failed' ? 'failed' : 'running',
                  summary:
                    state === 'failed'
                      ? 'The worker lost its lease after recording the comparison'
                      : 'Checking repository settings',
                  progress_current: 1,
                  progress_total: 2,
                  attempt: 1,
                  started_at: '2026-09-13T12:00:00Z',
                  finished_at: null,
                }
              : null,
          check: {
            action: 'check',
            target_id: '2001',
            available: true,
            reason: 'available',
            effect: 'request_repository_check',
          },
        });
        await page.route('**/api/v1/targets/2001/sync/checks/retained-check', async (route) => {
          if (firstRead) {
            firstRead = false;
            await initialRead;
          }
          const status =
            state === 'error' ? 500 : state === 'denied' ? 403 : state === 'missing' ? 404 : 200;
          await route.fulfill({
            status,
            json:
              status === 200 ? body() : { error: { code: 'read_failed', message: 'Read failed' } },
          });
        });
        await page.route(
          '**/api/v1/targets/2001/sync/checks/retained-check/observations?*',
          async (route) => {
            await route.fulfill({
              json: {
                items:
                  state === 'empty'
                    ? []
                    : [
                        {
                          repository_id: 'repo-1',
                          repository: 'smykla-skalski/original-repository',
                          kind: 'labels',
                          outcome: 'matched',
                          observed_at: '2026-09-13T12:00:00Z',
                          input_digest: 'original-input',
                          cached: false,
                        },
                      ],
                total: state === 'empty' ? 0 : 1,
                next_cursor: null,
              },
            });
          },
        );
        // This scene deliberately holds a read open, so do not wait for network settlement.
        await page.goto(addressOf(panel, 'workspace/sync/check/retained-check'), {
          waitUntil: 'domcontentloaded',
        });
        const dialog = page.getByRole('dialog', { name: 'Repository check', exact: true });
        const refresh = dialog.getByRole('button', { name: 'Refresh check', exact: true });
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-retained-check-${scene}-${colorScheme}.png`),
          });
        };
        await dialog.getByText('Loading check…', { exact: true }).waitFor();
        await capture('loading');
        releaseInitial?.();
        await dialog.getByText('smykla-skalski/original-repository', { exact: true }).waitFor();
        await capture('retained');
        for (const [next, text] of [
          ['failed', 'Execution failed'],
          ['error', 'Check could not be loaded'],
          ['retained', 'Execution details are unavailable.'],
          ['denied', 'Check access unavailable'],
          ['missing', 'Check not found'],
          ['empty', 'No enabled repositories to check'],
          ['running', 'Check in progress'],
        ] as const) {
          state = next;
          await refresh.focus();
          await page.keyboard.press('Enter');
          await dialog.getByText(text, { exact: false }).first().waitFor();
          await expect
            .poll(() => refresh.evaluate((el) => el === document.activeElement))
            .toBe(true);
          await capture(next === 'retained' ? 'recovered' : next);
        }
        await dialog.getByText('Execution details', { exact: true }).click();
        await dialog.getByText('Attempt', { exact: true }).waitFor();
        await capture('execution');
        await page.reload();
        await dialog.getByRole('heading', { name: 'Check in progress', exact: true }).waitFor();
        expect(page.url()).toContain('/check/retained-check');
        await dialog.getByRole('button', { name: 'Close check', exact: true }).click();
        await expect.poll(() => page.getByRole('dialog').count()).toBe(0);
      } finally {
        await page.close();
      }
    },
  );
});
