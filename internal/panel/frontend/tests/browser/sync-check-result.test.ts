import { checkResponse } from './sync-check-fixture';
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

describe('desktop check result identity and return navigation', () => {
  it.each(['light', 'dark'] as const)('keeps the original result in %s', async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(10_000);
    try {
      const iso = (offset: number) => new Date(Date.now() + offset).toISOString();
      const plan = syncPlanSeed(iso);
      plan.id = 'retained:original';
      plan.state = 'applied';
      plan.execution_stage = 'Sync completed';
      plan.actions = [
        {
          ...plan.actions[0]!,
          subject: 'original-check-label',
          detail: { label: { name: 'original-check-label', color: '0e8a16' } },
          state: 'applied',
        },
      ];
      plan.counts = { create: 1, update: 0, delete: 0 };
      const check = {
        ...queueSeeds(iso)[0]!,
        id: 'scan:original',
        kind: 'sync_scan' as const,
        repository_id: undefined,
        repository_name: undefined,
        source_kind: 'sync_scan',
        source_id: '2001',
        estimated_start_at: undefined,
        finished_at: iso(-60_000),
        state: 'succeeded' as const,
        title: 'Original repository check',
        summary: 'Found one change',
        progress_current: 0,
        progress_total: 0,
        details: { result_plan_id: plan.id } as Record<string, unknown>,
      };
      let missing = false;
      const requested: string[] = [];
      await page.route('**/api/v1/targets/*/sync/checks/scan%3Aoriginal', (route) =>
        route.fulfill({ json: checkResponse(check) }),
      );
      await page.route('**/api/v1/targets/*/sync/plan', (route) =>
        route.fulfill({ json: { plan: { ...syncPlanSeed(iso), id: 'newer-plan' } } }),
      );
      await page.route('**/api/v1/targets/*/sync/plans/*', (route) => {
        requested.push(
          decodeURIComponent(new URL(route.request().url()).pathname.split('/').at(-1)!),
        );
        return missing
          ? route.fulfill({
              status: 404,
              json: { error: { code: 'not_found', message: 'Sync result not found' } },
            })
          : route.fulfill({ json: { plan } });
      });
      const checkURL = addressOf(panel, 'workspace/sync/check/scan%3Aoriginal');
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
          path: join(directory, `F33-check-result-${scene}-${colorScheme}.png`),
          fullPage: false,
        });
      };
      const resultLink = page.getByRole('link', {
        name: 'View changes from this check',
        exact: true,
      });
      await resultLink.waitFor();
      expect(await resultLink.getAttribute('href')).toContain(
        '/sync/check/scan%3Aoriginal/result/retained%3Aoriginal',
      );
      await capture('source');
      await resultLink.click();
      const assertResult = async () => {
        await page.getByRole('heading', { name: '1 change processed', exact: true }).waitFor();
        expect(await page.getByRole('dialog').count()).toBe(1);
        const group = page.locator('.repo-row').filter({ hasText: 'platform-infra' });
        if ((await group.getAttribute('aria-expanded')) !== 'true') await group.click();
        await page.getByText('original-check-label', { exact: true }).waitFor();
        expect(requested.length).toBeGreaterThan(0);
        expect(requested.every((id) => id === plan.id)).toBe(true);
      };
      await assertResult();
      await capture('retained');
      await page.reload();
      await assertResult();
      await page.getByRole('button', { name: 'Back to check', exact: true }).click();
      await page.waitForURL(checkURL);
      await resultLink.waitFor();
      await page.goBack();
      await assertResult();
      missing = true;
      await page.reload();
      await page.getByText('These changes are unavailable', { exact: true }).waitFor();
      await capture('missing');
      await page.getByRole('button', { name: 'Back to check', exact: true }).click();
      await page.waitForURL(checkURL);
      await resultLink.waitFor();
      check.details = {};
      await page.reload();
      await page.getByRole('heading', { name: 'Repository check', exact: true }).waitFor();
      expect(await resultLink.count()).toBe(0);
      await capture('unlinked');
    } finally {
      await page.close();
    }
  });
});
