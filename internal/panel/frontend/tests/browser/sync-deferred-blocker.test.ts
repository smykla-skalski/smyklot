import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { queueSeeds, syncPlanSeed } from '../../dev/fixtures';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop deferred-check blocker identity and return navigation', () => {
  it.each(['light', 'dark'] as const)('keeps the earlier blocker in %s', async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(10_000);
    try {
      const iso = (offset: number) => new Date(Date.now() + offset).toISOString();
      const plan = syncPlanSeed(iso);
      plan.id = 'retained:earlier';
      plan.state = 'applied';
      plan.execution_stage = 'Sync completed';
      plan.actions = [
        {
          ...plan.actions[0]!,
          subject: 'earlier-change-label',
          detail: { label: { name: 'earlier-change-label', color: '0e8a16' } },
          state: 'applied',
        },
      ];
      plan.counts = { create: 1, update: 0, delete: 0 };
      const check = {
        ...queueSeeds(iso)[0]!,
        id: 'scan:deferred',
        kind: 'sync_scan' as const,
        repository_id: undefined,
        repository_name: undefined,
        source_kind: 'sync_scan',
        source_id: '2001',
        estimated_start_at: undefined,
        finished_at: iso(-60_000),
        state: 'succeeded' as const,
        title: 'Deferred repository check',
        summary: 'Earlier changes were still in progress when this check ran',
        progress_current: 0,
        progress_total: 0,
        details: {
          outcome: {
            completed_at: iso(-60_000),
            disposition: 'deferred',
            summary: 'Earlier changes were still in progress when this check ran',
            counts: {},
            cached: 0,
            missing_permissions: [],
            blocking_plan_id: plan.id,
          },
        } as Record<string, unknown>,
      };
      let unavailable: 0 | 403 | 404 = 0;
      const requested: string[] = [];
      await page.route('**/api/v1/targets/*/queue/scan%3Adeferred', (route) =>
        route.fulfill({ json: { item: check, events: [] } }),
      );
      await page.route('**/api/v1/targets/*/sync/plan', (route) =>
        route.fulfill({ json: { plan: { ...syncPlanSeed(iso), id: 'newer-plan' } } }),
      );
      await page.route('**/api/v1/targets/*/sync/plans/*', (route) => {
        requested.push(
          decodeURIComponent(new URL(route.request().url()).pathname.split('/').at(-1)!),
        );
        return unavailable
          ? route.fulfill({
              status: unavailable,
              json: {
                error: {
                  code: 'not_found',
                  message:
                    unavailable === 403
                      ? 'You no longer have access to these changes'
                      : 'Sync result not found',
                },
              },
            })
          : route.fulfill({ json: { plan } });
      });
      await page.route('**/api/v1/targets/*/sync/checks/*/observations*', (route) =>
        route.fulfill({ json: { items: [], total: 0, next_cursor: null } }),
      );
      const checkURL = addressOf(panel, 'workspace/sync/check/scan%3Adeferred');
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
      await visit(page, checkURL);
      const capture = async (scene: string) => {
        const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.mouse.move(1900, 20);
        await page.screenshot({
          path: join(directory, `F33-deferred-blocker-${scene}-${colorScheme}.png`),
          fullPage: false,
        });
      };
      const resultLink = page.getByRole('link', {
        name: 'View earlier changes',
        exact: true,
      });
      await resultLink.waitFor();
      expect(await resultLink.getAttribute('href')).toContain(
        '/sync/check/scan%3Adeferred/result/retained%3Aearlier',
      );
      await capture('source');
      await resultLink.click();
      const assertResult = async () => {
        await page.getByRole('heading', { name: '1 change processed', exact: true }).waitFor();
        expect(await page.getByRole('dialog').count()).toBe(1);
        const group = page.locator('.repo-row').filter({ hasText: 'platform-infra' });
        if ((await group.getAttribute('aria-expanded')) !== 'true') await group.click();
        await page.getByText('earlier-change-label', { exact: true }).waitFor();
        expect(requested.length).toBeGreaterThan(0);
        expect(requested.every((id) => id === plan.id)).toBe(true);
      };
      await assertResult();
      await capture('retained');
      await page.reload();
      await assertResult();
      await page.getByRole('button', { name: 'Close sync details', exact: true }).click();
      await page.waitForURL(checkURL);
      await resultLink.waitFor();
      await page.goBack();
      await assertResult();
      unavailable = 404;
      await page.reload();
      await page.getByText('Sync result not found', { exact: true }).waitFor();
      await capture('missing');
      await page.getByRole('button', { name: 'Close sync details', exact: true }).click();
      await page.waitForURL(checkURL);
      await resultLink.waitFor();
      unavailable = 403;
      await resultLink.click();
      await page.getByText('You no longer have access to these changes', { exact: true }).waitFor();
      expect(await page.getByText('earlier-change-label', { exact: true }).count()).toBe(0);
      await capture('denied');
      await page.getByRole('button', { name: 'Close sync details', exact: true }).click();
      await page.waitForURL(checkURL);
      delete (check.details.outcome as Record<string, unknown>).blocking_plan_id;
      await page.reload();
      await page.getByRole('heading', { name: check.title, exact: true }).waitFor();
      expect(await resultLink.count()).toBe(0);
      await page
        .getByText('This check did not record which earlier changes prevented it from proceeding', {
          exact: true,
        })
        .waitFor();
      await capture('unlinked');
    } finally {
      await page.close();
    }
  });
});
