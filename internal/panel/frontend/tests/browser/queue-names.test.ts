import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, expect, it } from 'vitest';
import { startPanel, type Panel } from './harness';
let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});
it.each(['light', 'dark'] as const)('names queue filters in %s desktop', async (colorScheme) => {
  const page = await panel.browser.newPage({
    viewport: { width: 1920, height: 1200 },
    colorScheme,
    reducedMotion: 'reduce',
  });
  page.setDefaultTimeout(5000);
  const capture = async (scene: string) => {
    const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
    if (!directory) return;
    await mkdir(directory, { recursive: true });
    await page.screenshot({
      path: join(directory, `F13-${scene}-${colorScheme}.png`),
      animations: 'disabled',
    });
  };
  try {
    for (const scope of ['root', 'workspace/smykla-skalski']) {
      const prefix = scope === 'root' ? 'root' : 'workspace';
      await page.goto(`${panel.origin}/${scope}/queue`);
      await page.getByRole('button', { name: 'Filter queue', exact: true }).click();
      const hours = page.getByRole('listbox', { name: 'Hours', exact: true });
      await hours
        .getByRole('option', { name: 'Always Open', exact: true })
        .scrollIntoViewIfNeeded();
      expect(await hours.getByRole('option', { name: 'always-open', exact: true }).count()).toBe(0);
      await capture(`${prefix}-hours`);
      const repositories = page.getByRole('listbox', { name: 'Repository', exact: true });
      const repository = repositories.getByRole('option', {
        name: 'smykla-skalski/platform-infra',
        exact: true,
      });
      await repository.scrollIntoViewIfNeeded();
      expect(await repositories.getByRole('option', { name: '4002', exact: true }).count()).toBe(0);
      await capture(`${prefix}-repositories`);
      const filtered = page.waitForRequest(
        (request) =>
          request.url().includes('/queue?') &&
          new URL(request.url()).searchParams.get('repository') === '4002',
      );
      await repository.click();
      await filtered;
      expect(await repository.getAttribute('aria-selected')).toBe('true');
      await page.keyboard.press('Escape');
    }
  } finally {
    await page.close();
  }
});

it.each(['light', 'dark'] as const)(
  'recovers missing queue names in %s desktop',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    let missing = true;
    await page.route('**/api/v1/root/queue?*', async (route) => {
      const response = await route.fetch();
      const body = await response.json();
      if (missing && body.facets) {
        body.facets.repository_names = {};
        body.facets.profile_names = {};
        for (const item of body.items ?? []) item.repository_name = '';
      }
      await route.fulfill({ response, json: body });
    });
    const capture = async (scene: string) => {
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.screenshot({
        path: join(directory, `F13-${scene}-${colorScheme}.png`),
        animations: 'disabled',
      });
    };
    try {
      await page.goto(`${panel.origin}/root/queue`);
      await page.getByRole('button', { name: 'Retry names', exact: true }).waitFor();
      await page.getByRole('button', { name: 'Filter queue', exact: true }).click();
      const fallback = page.getByRole('option', {
        name: 'Repository 4002 (name unavailable)',
        exact: true,
      });
      await fallback.scrollIntoViewIfNeeded();
      await capture('missing-names');
      await page.keyboard.press('Escape');
      await page.getByRole('button', { name: 'Retry names', exact: true }).click();
      await page.getByRole('button', { name: 'Retry names', exact: true }).waitFor();
      expect(
        await page.getByText('Deleted records keep their IDs.', { exact: false }).count(),
      ).toBe(1);
      missing = false;
      await page.getByRole('button', { name: 'Retry names', exact: true }).click();
      await page
        .getByRole('button', { name: 'Retry names', exact: true })
        .waitFor({ state: 'hidden' });
      await page.getByRole('button', { name: 'Open Merge after CI', exact: true }).click();
      const detail = page.getByRole('dialog', { name: 'Merge after CI', exact: true });
      await detail.getByText('smykla-skalski/platform-infra', { exact: true }).waitFor();
      await capture('named-inspector');
    } finally {
      await page.close();
    }
  },
);

it.each(['light', 'dark'] as const)(
  'refreshes repository inspector names in %s desktop',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    let missing = true;
    let failRefresh = false;
    await page.route('**/api/v1/**/queue/*', async (route) => {
      if (failRefresh) {
        await route.fulfill({
          status: 503,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'temporary failure' }),
        });
        return;
      }
      const response = await route.fetch();
      const body = await response.json();
      if (missing && body.item) body.item.repository_name = '';
      await route.fulfill({ response, json: body });
    });
    const capture = async (scene: string) => {
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.screenshot({
        path: join(directory, `F13-${scene}-${colorScheme}.png`),
        animations: 'disabled',
      });
    };
    try {
      for (const scope of ['root', 'workspace/smykla-skalski']) {
        missing = true;
        const prefix = scope === 'root' ? 'root' : 'workspace';
        await page.goto(`${panel.origin}/${scope}/queue`);
        await page.getByRole('button', { name: 'Open Merge after CI', exact: true }).click();
        const dialog = page.getByRole('dialog', { name: 'Merge after CI', exact: true });
        await dialog.getByText('Repository 4002 (name unavailable)', { exact: false }).waitFor();
        await capture(`${prefix}-inspector-missing`);
        await dialog.getByRole('button', { name: 'Refresh', exact: true }).click();
        await dialog.getByRole('button', { name: 'Refresh', exact: true }).waitFor();
        expect(await dialog.innerText()).toContain('Deleted repositories keep their IDs');
        failRefresh = true;
        await dialog.getByRole('button', { name: 'Refresh', exact: true }).focus();
        await page.keyboard.press('Enter');
        await dialog.getByRole('alert').waitFor();
        const retry = dialog.getByRole('button', { name: 'Try again', exact: true });
        expect(await retry.evaluate((node) => node === document.activeElement)).toBe(true);
        await capture(`${prefix}-inspector-refresh-error`);
        failRefresh = false;
        missing = false;
        await retry.press('Enter');
        await dialog.getByText('smykla-skalski/platform-infra', { exact: true }).waitFor();
        await dialog.getByRole('button', { name: 'Refresh', exact: true }).focus();
        await page.keyboard.press('Enter');
        await dialog.getByText('smykla-skalski/platform-infra', { exact: true }).waitFor();
        expect(
          await dialog
            .getByRole('button', { name: 'Refresh', exact: true })
            .evaluate((node) => node === document.activeElement),
        ).toBe(true);
        expect(await dialog.getByText('Estimated start', { exact: true }).count()).toBe(0);
        expect(await dialog.getByText('Work ahead', { exact: true }).count()).toBe(0);
        await dialog
          .getByText('Not confirmed. The blocker must clear before this occurrence can start.', {
            exact: true,
          })
          .waitFor();
        await capture(`${prefix}-inspector-recovered`);
        await page.keyboard.press('Escape');
      }
    } finally {
      await page.close();
    }
  },
);
