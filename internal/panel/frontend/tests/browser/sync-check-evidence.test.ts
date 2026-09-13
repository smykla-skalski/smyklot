import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { queueSeeds } from '../../dev/fixtures';
import type { SyncCheckObservation, SyncCheckOutcome } from '../../src/lib/types';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop retained check evidence', () => {
  it.each(['light', 'dark'] as const)(
    'reads exact check evidence and recovers in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        const observed = '2026-09-12T10:00:00Z';
        const iso = (offset: number) => new Date(Date.now() + offset).toISOString();
        const outcome: SyncCheckOutcome = {
          completed_at: iso(-60000),
          disposition: 'checked',
          summary: 'Compared repositories. Some comparisons could not finish.',
          counts: { matched: 8, different: 1, failed: 1 },
          cached: 1,
          missing_permissions: ['rulesets'],
        };
        const check = {
          ...queueSeeds(iso)[0]!,
          id: 'scan:evidence',
          kind: 'sync_scan' as const,
          target_id: '2001',
          repository_id: undefined,
          repository_name: undefined,
          source_kind: 'sync_scan',
          source_id: '2001',
          state: 'succeeded' as const,
          title: 'Repository check',
          summary: outcome.summary,
          details: { outcome } as Record<string, unknown>,
        };
        const rows: SyncCheckObservation[] = Array.from({ length: 11 }, (_, index) => ({
          repository_id: String(100 + index),
          repository: `smykla-skalski/original-repository-${index + 1}`,
          kind: 'labels',
          outcome: index === 0 ? 'failed' : index === 1 ? 'different' : 'matched',
          observed_at: index === 10 ? observed : outcome.completed_at,
          input_digest: `saved-input-${index}`,
          cached: index === 10,
          ...(index === 0
            ? { reason: 'GitHub could not be reached. This repository was not confirmed.' }
            : {}),
        }));
        let mode: 'ok' | 'error' | 'missing' | 'empty' = 'ok';
        let release: (() => void) | undefined;
        let gate: Promise<void> | undefined = new Promise<void>((resolve) => {
          release = resolve;
        });
        const requests: string[] = [];
        await page.route('**/api/v1/targets/*/queue/scan%3Aevidence', (route) =>
          route.fulfill({ json: { item: check, events: [] } }),
        );
        await page.route('**/api/v1/targets/*/sync/checks/*/observations?*', async (route) => {
          const url = new URL(route.request().url());
          requests.push(url.pathname);
          if (gate) await gate;
          if (mode === 'error' || mode === 'missing')
            return route.fulfill({
              status: mode === 'error' ? 503 : 404,
              json: { error: { code: 'unavailable', message: 'Unavailable' } },
            });
          const offset = url.searchParams.get('cursor') ? 10 : 0;
          const limit = Number(url.searchParams.get('limit'));
          const items = mode === 'empty' ? [] : rows.slice(offset, offset + limit);
          return route.fulfill({
            json: {
              items,
              total: mode === 'empty' ? 0 : rows.length,
              next_cursor: mode !== 'empty' && offset + limit < rows.length ? 'page-two' : null,
            },
          });
        });
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: /Account menu for/u }).click();
        await page
          .getByRole('radio', {
            name: `${colorScheme === 'light' ? 'Light' : 'Dark'} theme`,
            exact: true,
          })
          .locator('..')
          .click();
        await page.keyboard.press('Escape');
        const url = addressOf(panel, 'workspace/sync/check/scan%3Aevidence');
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-check-evidence-${scene}-${colorScheme}.png`),
            fullPage: false,
          });
        };
        await visit(page, url);
        const dialog = page.getByRole('dialog');
        await dialog
          .getByRole('heading', { name: 'Check finished with gaps', exact: true })
          .waitFor();
        await dialog.getByText('Loading repository evidence…', { exact: true }).waitFor();
        expect(await dialog.getByText('Priority', { exact: true }).isVisible()).toBe(false);
        await capture('loading');
        release?.();
        gate = undefined;
        await dialog.getByText(rows[0]!.repository, { exact: true }).waitFor();
        await capture('mixed');
        const pagination = dialog.locator('[aria-label="Repository evidence pagination"]');
        await pagination.scrollIntoViewIfNeeded();
        await capture('page-one');
        await pagination.getByRole('button', { name: 'Next', exact: true }).click();
        await dialog.getByText(rows[10]!.repository, { exact: true }).waitFor();
        expect(await dialog.locator(`time[datetime="${observed}"]`).count()).toBe(1);
        expect(
          await dialog
            .locator('li')
            .getByText(/Reused observation/u)
            .count(),
        ).toBe(1);
        expect(
          await dialog
            .getByRole('heading', { name: 'Repository evidence', exact: true })
            .evaluate((element) => element === document.activeElement),
        ).toBe(true);
        await capture('reused');
        // Same check reopening retains the visited boundary and does not switch to current status.
        await dialog.getByRole('button', { name: 'Close', exact: true }).click();
        await page.waitForURL(/\/sync$/u);
        await page.goBack();
        await page.waitForURL(url);
        await dialog.getByText(rows[10]!.repository, { exact: true }).waitFor();
        expect(requests.every((path) => path.includes('/2001/sync/checks/scan%3Aevidence/'))).toBe(
          true,
        );
        const summary = dialog.locator('summary').filter({ hasText: 'Execution details' });
        await summary.click();
        await dialog.getByText('Priority', { exact: true }).waitFor();
        await dialog.getByText('Priority', { exact: true }).scrollIntoViewIfNeeded();
        await capture('execution');
        await pagination.getByRole('combobox', { name: 'Repository evidence per page' }).click();
        await page.getByRole('option', { name: '20', exact: true }).click();
        await dialog.getByText(rows[0]!.repository, { exact: true }).waitFor();
        expect(await dialog.locator('li').count()).toBe(11);
        expect(
          await pagination.getByRole('button', { name: 'Next', exact: true }).isDisabled(),
        ).toBe(true);
        mode = 'error';
        await page.reload();
        await dialog
          .getByText('Repository evidence could not be loaded. Try again.', { exact: true })
          .waitFor();
        await capture('error');
        mode = 'ok';
        await dialog.getByRole('button', { name: 'Try again', exact: true }).click();
        await dialog.getByText(rows[0]!.repository, { exact: true }).waitFor();
        mode = 'missing';
        await page.reload();
        await dialog
          .getByText('The evidence is no longer available, or you no longer have access to it.', {
            exact: true,
          })
          .waitFor();
        expect(await dialog.getByRole('button', { name: 'Try again', exact: true }).count()).toBe(
          0,
        );
        await capture('unavailable');
        mode = 'empty';
        check.details = {
          outcome: {
            ...outcome,
            counts: {},
            cached: 0,
            missing_permissions: [],
            summary: 'No enabled repository categories were eligible for comparison.',
          },
        };
        await page.reload();
        await dialog
          .getByText(
            'No repository comparisons were recorded for this check. This does not confirm a match.',
            { exact: true },
          )
          .waitFor();
        expect(await pagination.count()).toBe(0);
        await capture('empty');
        for (const [disposition, title, message] of [
          ['disabled', 'Sync was switched off', 'Sync was disabled when this check ran.'],
          [
            'unpermitted',
            'Check needs GitHub permissions',
            'No enabled categories had the required GitHub permissions.',
          ],
          [
            'deferred',
            'Check deferred',
            'An earlier plan still needs review before another check can prepare changes.',
          ],
        ] as const) {
          check.details = {
            outcome: {
              ...outcome,
              counts: {},
              cached: 0,
              missing_permissions: disposition === 'unpermitted' ? ['labels'] : [],
              disposition,
              summary: message,
            },
          };
          await page.reload();
          await dialog.getByRole('heading', { name: title, exact: true }).waitFor();
          await dialog
            .getByText(
              'No repository comparisons were recorded for this check. This does not confirm a match.',
              { exact: true },
            )
            .waitFor();
          await capture(disposition);
        }
        check.details = {};
        await page.reload();
        await dialog
          .getByRole('heading', { name: 'Check outcome unavailable', exact: true })
          .waitFor();
        expect(
          await dialog.getByRole('heading', { name: 'Repository evidence', exact: true }).count(),
        ).toBe(0);
        await capture('legacy');
      } finally {
        await page.close();
      }
    },
  );
});
