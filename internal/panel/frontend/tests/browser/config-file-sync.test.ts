import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { CONFIG_FILE_STATUS_FIXTURES, mockConfigFileStatus } from '../../dev/config-file-status';
import type { ConfigFileSyncStatus } from '../../src/lib/config-file-sync';
import { startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});
const card = (page: Page) =>
  page.getByRole('region', { name: 'Configuration file sync', exact: true });
async function capture(page: Page, name: string) {
  const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
  if (!directory) return;
  await mkdir(directory, { recursive: true });
  await card(page).scrollIntoViewIfNeeded();
  await page.mouse.move(0, 0);
  await card(page).screenshot({ path: join(directory, `${name}.png`) });
}
async function switchGeometry(page: Page) {
  const geometry = await card(page)
    .locator('.policy-row')
    .filter({ has: page.getByRole('checkbox', { name: 'Sync settings with file' }) })
    .evaluate((row) => {
      const bounds = row.getBoundingClientRect();
      const copy = row.querySelector('.setting-say')!.getBoundingClientRect();
      const control = row.querySelector('.switch-track')!.getBoundingClientRect();
      return {
        left: control.left,
        right: control.right,
        rowRight: bounds.right,
        paddingRight: parseFloat(getComputedStyle(row).paddingRight),
        copyRight: copy.right,
        copyMiddle: (copy.top + copy.bottom) / 2,
        controlMiddle: (control.top + control.bottom) / 2,
      };
    });
  expect(geometry.left).toBeGreaterThanOrEqual(geometry.copyRight);
  expect(Math.abs(geometry.controlMiddle - geometry.copyMiddle)).toBeLessThanOrEqual(1);
  expect(geometry.rowRight - geometry.right).toBeLessThanOrEqual(geometry.paddingRight + 1);
}

const workspace = () => `${panel.origin}/workspace/${panel.account}/settings`;
const repository = (name: string) =>
  `${panel.origin}/workspace/${panel.account}/repositories/${name}`;

async function optIn(page: Page, checked: boolean) {
  const input = card(page).getByRole('checkbox', { name: 'Sync settings with file' });
  if ((await input.isChecked()) !== checked) await input.locator('..').click();
  await expect.poll(() => input.isChecked()).toBe(checked);
}

async function save(page: Page) {
  const response = page.waitForResponse(
    (r) => r.request().method() === 'PUT' && r.url().endsWith('/settings'),
  );
  const button = page.getByRole('button', { name: 'Save', exact: true });
  await expect.poll(() => button.isEnabled()).toBe(true);
  await button.click();
  const saved = await response;
  expect(saved.ok(), await saved.text()).toBe(true);
  return saved;
}

describe('configuration-file connection [Browser]', () => {
  it('stages, reverts, saves and reloads workspace opt-in without changing the observed state early', async () => {
    const page = await panel.browser.newPage({
      viewport: { width: 1440, height: 1000 },
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(8000);
    let reads = 0;
    page.on('request', (request) => {
      if (request.url().endsWith('/config-file')) reads++;
    });
    try {
      await visit(page, workspace(), { ready: '[aria-label="Configuration file sync"]' });
      await card(page).getByText('Off', { exact: true }).waitFor();
      const before = reads;
      await optIn(page, true);
      await card(page).getByText('Starts after you save').waitFor();
      expect(await card(page).getByText('Off', { exact: true }).count()).toBe(1);
      expect(reads).toBe(before);
      await optIn(page, false);
      await expect.poll(() => page.locator('.settings-composer').count()).toBe(0);
      await optIn(page, true);
      const response = await save(page);
      expect(response.request().postDataJSON().target.config_file_sync_enabled).toBe(true);
      await card(page).getByText('Waiting for check', { exact: true }).waitFor();
      await page.reload();
      await card(page).getByText('Waiting for check', { exact: true }).waitFor();
      expect(await card(page).getByRole('checkbox').isChecked()).toBe(true);
      expect(await page.locator('[data-unsaved]').count()).toBe(0);
      await optIn(page, false);
      await save(page);
      await card(page).getByText('Off', { exact: true }).waitFor();
    } finally {
      await page.close();
    }
  });

  it('preserves repository file bypass and scope through actual SPA navigation, save and reload', async () => {
    const page = await panel.browser.newPage({
      viewport: { width: 1440, height: 1000 },
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(8000);
    try {
      await visit(page, repository('api-gateway'), {
        ready: '[aria-label="Configuration file sync"]',
      });
      const useFile = page.getByRole('checkbox', { name: 'Use file settings', exact: true });
      expect(await useFile.isChecked()).toBe(false);
      await optIn(page, true);
      await card(page).getByText('Starts after you save').waitFor();
      const result = await save(page);
      const input = result.request().postDataJSON();
      expect(input.target).toBeUndefined();
      expect(input.repositories).toHaveLength(1);
      expect(input.repositories[0]).toMatchObject({
        config_file_sync_enabled: true,
        ignore_repository_file: true,
      });
      await card(page).getByText('File settings off', { exact: true }).waitFor();
      expect(await useFile.isChecked()).toBe(false);
      await page.locator(`a[href="/workspace/${panel.account}/settings"]`).first().click();
      await card(page).getByText('Workspace file', { exact: true }).waitFor();
      expect(await card(page).getByRole('checkbox').isChecked()).toBe(false);
      await page.goBack();
      await card(page).getByText('File settings off', { exact: true }).waitFor();
      await page.reload();
      await card(page).getByText('File settings off', { exact: true }).waitFor();
      await optIn(page, false);
      await save(page);
      expect(await useFile.isChecked()).toBe(false);
    } finally {
      await page.close();
    }
  });

  for (const theme of ['light', 'dark'] as const) {
    for (const width of [375, 768, 1024, 1440]) {
      it(`shows saved connection variants with shared rows in ${theme} at ${width}`, async () => {
        const page = await panel.browser.newPage({
          viewport: { width, height: 1100 },
          reducedMotion: 'reduce',
        });
        const crashes: string[] = [];
        page.on('pageerror', (error) => crashes.push(error.message));
        try {
          await page.addInitScript((value) => localStorage.setItem('smyklot:theme', value), theme);
          await visit(page, workspace(), { ready: '[aria-label="Configuration file sync"]' });
          await page.evaluate((value) => {
            document.documentElement.dataset.theme = value;
          }, theme);
          await card(page).getByText('Off', { exact: true }).waitFor();
          await capture(page, `workspace-off-${theme}-${width}`);
          await switchGeometry(page);
          await optIn(page, true);
          await capture(page, `workspace-draft-${theme}-${width}`);
          await switchGeometry(page);
          await optIn(page, false);
          // One real repository page; only its cached backend observation is varied.
          const checkedAt = new Date(Date.now() - 5 * 60_000).toISOString();
          let status: ConfigFileSyncStatus = mockConfigFileStatus(
            'ready',
            'smykla-skalski/auth-service',
            true,
            1,
            false,
            false,
            checkedAt,
          );
          await page.route('**/repositories/*/config-file', (route) =>
            route.fulfill({ json: status }),
          );
          await visit(page, repository('auth-service'), {
            ready: '[aria-label="Configuration file sync"]',
          });
          await page.evaluate((value) => {
            document.documentElement.dataset.theme = value;
          }, theme);
          for (const variant of Object.keys(
            CONFIG_FILE_STATUS_FIXTURES,
          ) as (keyof typeof CONFIG_FILE_STATUS_FIXTURES)[]) {
            if (variant === 'off' || variant === 'missingAccess') continue;
            status = mockConfigFileStatus(
              variant,
              'smykla-skalski/auth-service',
              true,
              1,
              false,
              false,
              checkedAt,
            );
            // An actual route reload owns a fresh status query, without manufacturing a check.
            await page.reload();
            await card(page).waitFor();
            await expect
              .poll(() => card(page).getByText('Loading status', { exact: true }).count())
              .toBe(0);
            await page.evaluate((value) => {
              document.documentElement.dataset.theme = value;
            }, theme);
            await capture(page, `repository-${variant}-${theme}-${width}`);
            await switchGeometry(page);
            const geometry = await card(page).evaluate((element) => {
              const rect = element.getBoundingClientRect();
              const switchRect = element.querySelector('.switch-track')!.getBoundingClientRect();
              return {
                width: rect.width,
                left: rect.left,
                right: rect.right,
                viewport: innerWidth,
                scroll: element.scrollWidth,
                client: element.clientWidth,
                switchWidth: switchRect.width,
                switchHeight: switchRect.height,
              };
            });
            expect(geometry.left).toBeGreaterThanOrEqual(0);
            expect(geometry.right).toBeLessThanOrEqual(width);
            expect(geometry.scroll).toBeLessThanOrEqual(geometry.client + 1);
            expect(geometry.switchWidth).toBe(34);
            expect(geometry.switchHeight).toBe(20);
          }
          await visit(page, `${panel.origin}/workspace/bart/settings`, {
            ready: '[aria-label="Configuration file sync"]',
          });
          await card(page).getByText('Needs attention', { exact: true }).waitFor();
          await page.evaluate((value) => {
            document.documentElement.dataset.theme = value;
          }, theme);
          await capture(page, `workspace-missingAccess-${theme}-${width}`);
          await switchGeometry(page);
          expect(crashes).toEqual([]);
        } finally {
          await page.close();
        }
      });
    }
  }

  it.each(
    ['light', 'dark'].flatMap((theme) => [375, 768, 1024, 1440].map((width) => ({ theme, width }))),
  )(
    'keeps Root read-only status inspectable and recovers a failed read in $theme at $width',
    async ({ theme, width }) => {
      const page = await panel.browser.newPage({
        viewport: { width, height: 1100 },
        reducedMotion: 'reduce',
      });
      let fail = true;
      try {
        await page.route('**/root/workspaces/*/config-file', (route) =>
          fail
            ? route.fulfill({
                status: 503,
                json: { error: { code: 'unavailable', message: 'Status storage unavailable' } },
              })
            : route.fulfill({
                json: mockConfigFileStatus(
                  'ready',
                  'team-01/.github',
                  true,
                  1,
                  false,
                  true,
                  new Date(Date.now() - 5 * 60_000).toISOString(),
                ),
              }),
        );
        await visit(page, `${panel.origin}/root/workspaces/team-01/settings`, {
          ready: '[aria-label="Configuration file sync"]',
        });
        await card(page).getByRole('button', { name: 'Try again' }).waitFor();
        await page.evaluate((value) => {
          document.documentElement.dataset.theme = value;
        }, theme);
        expect(await card(page).getByRole('checkbox').isDisabled()).toBe(true);
        await capture(page, `root-read-only-error-${theme}-${width}`);
        fail = false;
        await card(page).getByRole('button', { name: 'Try again' }).click();
        await card(page).getByText('In sync', { exact: true }).waitFor();
        await capture(page, `root-read-only-ready-${theme}-${width}`);
        await switchGeometry(page);
      } finally {
        await page.close();
      }
    },
  );
});
