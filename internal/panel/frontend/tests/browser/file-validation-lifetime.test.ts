import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';
import type { SyncConfig } from '../../src/lib/types';
import { startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

async function capture(page: Page, name: string) {
  const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
  if (!directory) return;
  await mkdir(directory, { recursive: true });
  await page.mouse.move(0, 0);
  await page.screenshot({ path: join(directory, `${name}.png`) });
}

const configUrl = () => `${panel.origin}/api/v1/targets/2001/sync/config/files`;
async function restoreConfig(page: Page, original: SyncConfig) {
  const latest = await (await page.request.get(configUrl())).json();
  const result = await page.request.put(`${panel.origin}/api/v1/targets/2001/settings`, {
    data: {
      sync_configs: [
        {
          kind: 'files',
          enabled: original.enabled,
          document: original.document,
          expected_revision: latest.revision,
        },
      ],
    },
  });
  expect(result.ok(), await result.text()).toBe(true);
}

async function repositories(page: Page) {
  const mobileMenu = page.getByRole('button', { name: 'Show the pages', exact: true });
  if (await mobileMenu.isVisible()) await mobileMenu.click();
  await page.locator(`a[href="/workspace/${panel.account}/repositories"]`).first().click();
  await expect
    .poll(() => new URL(page.url()).pathname)
    .toBe(`/workspace/${panel.account}/repositories`);
}

describe('file validation lifetime [Browser]', () => {
  it('returns to the invalid repository adjustment instead of an older unrelated change', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    page.setDefaultTimeout(8000);
    await page.route('**/sync/files/render', async (route) => {
      if (route.request().postDataJSON()?.repository?.merge?.overrides?.retries !== 8)
        return route.continue();
      await route.fulfill({
        json: {
          valid: false,
          final_content: '',
          matches_formatting: false,
          diagnostics: [
            {
              stage: 'merge',
              code: 'invalid_override',
              message: 'Check this repository adjustment',
            },
          ],
        },
      });
    });
    try {
      await visit(page, `${panel.origin}/workspace/${panel.account}/repositories/api-gateway`, {
        ready: 'input[aria-label="Quiet period after checks pass"]',
      });
      const quiet = page.getByRole('textbox', { name: 'Quiet period after checks pass' });
      const saved = await quiet.inputValue();
      await quiet.fill(saved === '45' ? '46' : '45');
      await quiet.blur();
      await visit(
        page,
        `${panel.origin}/workspace/${panel.account}/sync/files/.config/quality.jsonc`,
        { ready: '.file-editor' },
      );
      await page.getByRole('searchbox', { name: 'Find a repository output' }).fill('smyklot');
      await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'smyklot', exact: true });
      await dialog.locator('.cm-content').fill('{"enabled":true,"retries":8}');
      await dialog.getByRole('button', { name: 'Done', exact: true }).click();
      await repositories(page);
      await page.reload();
      const composer = page.getByRole('complementary', { name: 'Settings draft' });
      await composer
        .getByText('.config/quality.jsonc: Check this repository adjustment', { exact: true })
        .waitFor();
      await composer.getByRole('link', { name: 'Open .config/quality.jsonc', exact: true }).click();
      const adjustment = page.getByRole('article', {
        name: 'Adjustment for .config/quality.jsonc',
        exact: true,
      });
      await adjustment.waitFor();
      expect(new URL(page.url()).hash).toBe('#file-sync=.config%2Fquality.jsonc');
      expect(await adjustment.locator('.cm-content').innerText()).toContain('8');
      expect(await composer.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
        true,
      );
      await capture(page, 'repository-invalid-return');
    } finally {
      await page.close();
    }
  });

  it.each(['before debounce', 'in flight'] as const)(
    'settles and saves after leaving %s without returning',
    async (phase) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1100 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(8000);
      const content = 'enabled: true\nretries: 9\n';
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      let requests = 0;
      await page.route('**/sync/files/render', async (route) => {
        if (route.request().postDataJSON()?.draft_content?.trimEnd() !== content.trimEnd())
          return route.continue();
        requests += 1;
        await gate;
        await route.continue();
      });
      let original: SyncConfig | undefined;
      try {
        await visit(
          page,
          `${panel.origin}/workspace/${panel.account}/sync/files/.config/quality.yaml`,
          { ready: '.file-editor' },
        );
        original = await (await page.request.get(configUrl())).json();
        await page.evaluate(() => {
          (window as unknown as { lifetimeMarker: number }).lifetimeMarker = 23;
        });
        if (phase === 'before debounce') {
          await page.clock.install();
          await page.clock.pauseAt(new Date(Date.now() + 1000));
        }
        await page.locator('.file-editor .cm-content').first().fill(content.trimEnd());
        if (phase === 'in flight') await expect.poll(() => requests).toBeGreaterThan(0);
        else expect(requests).toBe(0);
        await page
          .locator(`a[href="/workspace/${panel.account}/repositories"]`)
          .first()
          .click({ force: phase === 'before debounce' });
        if (phase === 'before debounce') {
          expect(requests).toBe(0);
          await page.clock.resume();
        }
        await expect
          .poll(() => new URL(page.url()).pathname)
          .toBe(`/workspace/${panel.account}/repositories`);
        release();
        const save = page.getByRole('button', { name: 'Save', exact: true });
        await expect.poll(() => save.isEnabled(), { timeout: 5000 }).toBe(true);
        expect(new URL(page.url()).pathname).toBe(`/workspace/${panel.account}/repositories`);
        expect(
          await page.evaluate(
            () => (window as unknown as { lifetimeMarker: number }).lifetimeMarker,
          ),
        ).toBe(23);
        const saved = page.waitForResponse(
          (response) =>
            response.request().method() === 'PUT' && response.url().endsWith('/settings'),
        );
        await save.click();
        const response = await saved;
        expect(response.ok(), await response.text()).toBe(true);
        expect(
          response
            .request()
            .postDataJSON()
            .sync_configs[0].document.files.find(
              (file: { path: string }) => file.path === '.config/quality.yaml',
            ).content,
        ).toBe(content);
        const readback = await (await page.request.get(configUrl())).json();
        expect(
          readback.document.files.find(
            (file: { path: string }) => file.path === '.config/quality.yaml',
          ).content,
        ).toBe(content);
      } finally {
        release();
        if (original) await restoreConfig(page, original);
        await page.close();
      }
    },
  );
  it.each(
    (['light', 'dark'] as const).flatMap((colorScheme) =>
      [375, 768, 1024, 1440].map((width) => ({ colorScheme, width })),
    ),
  )(
    'keeps an invalid check blocking after navigation and reload $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({
        reducedMotion: 'reduce',
        colorScheme,
        viewport: { width, height: 1100 },
      });
      page.setDefaultTimeout(8000);
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      let received = 0;
      await page.route('**/sync/files/render', async (route) => {
        if (!route.request().postDataJSON()?.draft_content?.includes('unfinished-lifetime'))
          return route.continue();
        received += 1;
        await gate;
        await route.fulfill({
          json: {
            valid: false,
            final_content: '',
            matches_formatting: false,
            diagnostics: [
              { stage: 'parse', code: 'invalid_template', message: 'Finish the YAML list' },
            ],
          },
        });
      });
      try {
        await visit(
          page,
          `${panel.origin}/workspace/${panel.account}/sync/files/.config/quality.yaml`,
          { ready: '.file-editor' },
        );
        await page.locator('.file-editor .cm-content').first().fill('unfinished-lifetime: [');
        await expect.poll(() => received).toBeGreaterThan(0);
        await repositories(page);
        release();
        await page
          .getByText('.config/quality.yaml: Finish the YAML list', { exact: true })
          .waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        const beforeReload = received;
        await page.reload();
        await expect.poll(() => received).toBeGreaterThan(beforeReload);
        await page
          .getByText('.config/quality.yaml: Finish the YAML list', { exact: true })
          .waitFor();
        expect(new URL(page.url()).pathname).toBe(`/workspace/${panel.account}/repositories`);
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await capture(page, `invalid-after-navigation-${colorScheme}-${width}`);
        await page.getByRole('link', { name: 'Open .config/quality.yaml', exact: true }).click();
        await expect
          .poll(() => new URL(page.url()).pathname)
          .toBe(`/workspace/${panel.account}/sync/files/.config/quality.yaml`);
        await expect
          .poll(() => page.locator('.file-editor .cm-content').first().innerText())
          .toContain('unfinished-lifetime: [');
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
      } finally {
        release();
        await page.close();
      }
    },
  );

  it('recovers a failed same-input check from a fresh preview', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    page.setDefaultTimeout(8000);
    let attempts = 0;
    const content = 'enabled: true\nretries: 7';
    await page.route('**/sync/files/render', async (route) => {
      if (route.request().postDataJSON()?.draft_content?.trimEnd() !== content)
        return route.continue();
      attempts += 1;
      if (attempts === 1) return route.abort('failed');
      return route.continue();
    });
    try {
      await visit(
        page,
        `${panel.origin}/workspace/${panel.account}/sync/files/.config/quality.yaml`,
        { ready: '.file-editor' },
      );
      await page.locator('.file-editor .cm-content').first().fill(content);
      await expect.poll(() => attempts).toBe(1);
      const save = page.getByRole('button', { name: 'Save', exact: true });
      await page
        .getByText('.config/quality.yaml: the service could not be reached', { exact: true })
        .waitFor();
      expect(await save.isDisabled()).toBe(true);
      await repositories(page);
      await page.locator(`a[href="/workspace/${panel.account}/sync/files"]`).first().click();
      await page
        .locator(`a[href="/workspace/${panel.account}/sync/files/.config/quality.yaml"]`)
        .click();
      await expect.poll(() => attempts).toBe(2);
      await expect.poll(() => save.isEnabled()).toBe(true);
      expect(await page.locator('.file-editor .cm-content').first().innerText()).toBe(content);
      expect(
        await page
          .getByText('.config/quality.yaml: the service could not be reached', { exact: true })
          .count(),
      ).toBe(0);
      await capture(page, 'retry-recovered-light-1280');
    } finally {
      await page.close();
    }
  });

  it('discards pending validation without a late error leaking into the next draft', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    let received = 0;
    let completed = 0;
    await page.route('**/sync/files/render', async (route) => {
      if (!route.request().postDataJSON()?.draft_content?.includes('old-lifetime'))
        return route.continue();
      received += 1;
      await gate;
      await route.fulfill({
        json: {
          valid: false,
          final_content: '',
          matches_formatting: false,
          diagnostics: [{ stage: 'parse', code: 'invalid_template', message: 'Discarded failure' }],
        },
      });
      completed += 1;
    });
    try {
      await visit(
        page,
        `${panel.origin}/workspace/${panel.account}/sync/files/.config/quality.yaml`,
        { ready: '.file-editor' },
      );
      await page.locator('.file-editor .cm-content').first().fill('old-lifetime: [');
      await expect.poll(() => received).toBeGreaterThan(0);
      await repositories(page);
      await page.getByRole('button', { name: 'Discard', exact: true }).click();
      await page.locator(`a[href="/workspace/${panel.account}/sync/files"]`).first().click();
      await page
        .locator(`a[href="/workspace/${panel.account}/sync/files/.config/quality.yaml"]`)
        .click();
      await page.locator('.file-editor .cm-content').first().fill('enabled: true\nretries: 8');
      await repositories(page);
      const save = page.getByRole('button', { name: 'Save', exact: true });
      await expect.poll(() => save.isEnabled()).toBe(true);
      release();
      await expect.poll(() => completed).toBe(received);
      await page.evaluate(
        () =>
          new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          ),
      );
      expect(await save.isEnabled()).toBe(true);
      expect(await page.getByText(/Discarded failure/).count()).toBe(0);
    } finally {
      release();
      await page.close();
    }
  });

  it('settles a dirty repository check after its inspector closes', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    let received = 0;
    await page.route('**/sync/files/render', async (route) => {
      if (route.request().postDataJSON()?.repository?.merge?.overrides?.retries !== 8)
        return route.continue();
      received += 1;
      await gate;
      await route.continue();
    });
    try {
      await visit(
        page,
        `${panel.origin}/workspace/${panel.account}/sync/files/.config/quality.jsonc`,
        { ready: '.file-editor' },
      );
      await page.getByRole('searchbox', { name: 'Find a repository output' }).fill('smyklot');
      await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'smyklot', exact: true });
      const code = dialog.locator('.cm-content');
      await code.waitFor();
      await code.fill('{"enabled":true,"retries":8}');
      await expect.poll(() => received).toBeGreaterThan(0);
      await dialog.getByRole('button', { name: 'Done', exact: true }).click();
      release();
      const save = page.getByRole('button', { name: 'Save', exact: true });
      await expect.poll(() => save.isEnabled()).toBe(true);
      expect(await dialog.count()).toBe(0);
    } finally {
      release();
      await page.close();
    }
  });
});
