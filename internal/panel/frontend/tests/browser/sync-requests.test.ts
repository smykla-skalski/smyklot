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

describe('desktop accepted request discovery', () => {
  it.each(['light', 'dark'] as const)(
    'preserves scope and read recovery in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        let mode = 'ok';
        let releaseInitial = () => {};
        const initial = new Promise<void>((resolve) => {
          releaseInitial = resolve;
        });
        let firstRead = true;
        let mutations = 0;
        const reads: string[] = [];
        page.on('request', (request) => {
          if (new URL(request.url()).pathname.includes('/sync/') && request.method() !== 'GET')
            mutations++;
        });
        await page.route('**/api/v1/targets/2001/sync/requests?*', async (route) => {
          if (firstRead) {
            firstRead = false;
            await initial;
          }
          const url = new URL(route.request().url());
          reads.push(url.search);
          expect(url.searchParams.get('limit')).toBe('20');
          const older = url.searchParams.has('cursor');
          const status =
            mode === 'error' ? 500 : mode === 'denied' ? 403 : mode === 'stale' ? 400 : 200;
          await route.fulfill({
            status,
            json:
              status !== 200
                ? {
                    error: {
                      code: mode === 'stale' ? 'invalid_request_history_query' : 'read_failed',
                      message: 'Read failed',
                    },
                  }
                : {
                    items:
                      mode === 'empty'
                        ? []
                        : older
                          ? [
                              {
                                action: 'check',
                                request_key: 'earlier',
                                check_id: 'earlier-check',
                                accepted_at: '2026-09-12T12:00:00Z',
                                reason: 'Earlier check before changing labels',
                              },
                            ]
                          : [
                              {
                                action: 'check',
                                request_key: 'check-key',
                                check_id: 'original-check',
                                accepted_at: '2026-09-13T12:00:00Z',
                                reason: 'Verify saved labels after review',
                              },
                              {
                                action: 'dispatch',
                                request_key: 'dispatch-key',
                                plan_id: 'original-plan',
                                queue_id: 'original-worker',
                                expected_revision: 3,
                                accepted_at: '2026-09-13T11:00:00Z',
                                reason: 'Apply the reviewed changes to repository defaults',
                              },
                            ],
                    next_cursor: older || mode === 'empty' ? null : 'older-page',
                  },
          });
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        const trigger = page.getByRole('button', { name: 'Your sync requests', exact: true });
        await trigger.click();
        const dialog = page.getByRole('dialog', { name: 'Your sync requests', exact: true });
        const refresh = dialog.getByRole('button', { name: 'Refresh requests', exact: true });
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-requests-${scene}-${colorScheme}.png`),
          });
        };
        await dialog.getByText('Loading your accepted requests…', { exact: true }).waitFor();
        await capture('loading');
        releaseInitial();
        await dialog.getByText('Verify saved labels after review', { exact: true }).waitFor();
        expect(
          await dialog.getByRole('link', { name: 'View check' }).getAttribute('href'),
        ).toContain('/check/original-check');
        expect(
          await dialog.getByRole('link', { name: 'View changes' }).getAttribute('href'),
        ).toContain('original-plan');
        await capture('latest');
        await dialog.getByRole('button', { name: 'Older requests' }).click();
        await dialog.getByText('Earlier check before changing labels', { exact: true }).waitFor();
        expect(reads.at(-1)).toContain('cursor=older-page');
        await capture('older');
        mode = 'stale';
        await refresh.click();
        await dialog.getByText('This page can no longer be continued.', { exact: false }).waitFor();
        await capture('stale');
        mode = 'ok';
        await dialog.getByRole('button', { name: 'Latest requests', exact: true }).click();
        await dialog.getByText('Verify saved labels after review', { exact: true }).waitFor();
        expect(reads.at(-1)).not.toContain('cursor=');
        for (const [next, text] of [
          ['error', 'Requests could not be loaded'],
          ['denied', 'Your request history is unavailable'],
          ['empty', 'No accepted requests found'],
        ] as const) {
          await expect.poll(() => refresh.getAttribute('aria-disabled')).toBe('false');
          mode = next;
          await refresh.focus();
          await page.keyboard.press('Enter');
          await dialog.getByText(text, { exact: true }).waitFor();
          expect(
            await dialog.getByRole('list', { name: 'Your accepted sync requests' }).count(),
          ).toBe(0);
          await expect
            .poll(() => refresh.evaluate((el) => el === document.activeElement))
            .toBe(true);
          await capture(next);
        }
        expect(
          await dialog.getByRole('navigation', { name: 'Accepted request pages' }).count(),
        ).toBe(0);
        mode = 'ok';
        await refresh.click();
        await dialog.getByText('Verify saved labels after review', { exact: true }).waitFor();
        await dialog.getByRole('button', { name: 'Close requests' }).click();
        await expect.poll(() => trigger.evaluate((el) => el === document.activeElement)).toBe(true);
        await page.reload();
        await trigger.click();
        await dialog.getByRole('link', { name: 'View check', exact: true }).click();
        await page.waitForURL(/\/check\/original-check$/u);
        await page.getByRole('dialog', { name: 'Repository check', exact: true }).waitFor();
        expect(await dialog.count()).toBe(0);
        expect(mutations).toBe(0);
      } finally {
        await page.close();
      }
    },
  );
});
