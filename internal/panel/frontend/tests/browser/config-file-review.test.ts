import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { mockConfigFilePreview, type ConfigFileReviewVariant } from '../../dev/config-file-review';
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
const dialog = (page: Page) =>
  page.getByRole('dialog', { name: 'Review configuration file', exact: true });
const repo = (name: string, root = false) =>
  `${panel.origin}/${root ? 'root/workspaces' : 'workspace'}/${panel.account}/repositories/${name}`;
async function open(page: Page, url: string) {
  await visit(page, url, { ready: '[aria-label="Configuration file sync"]' });
  await card(page)
    .getByRole('button', { name: /^Review (conflicts|file)$/ })
    .click();
  await dialog(page).waitFor();
  await expect
    .poll(() => dialog(page).getByText('Comparing saved settings with the file').count())
    .toBe(0);
}
async function choose(page: Page, side: 'panel' | 'file') {
  await dialog(page)
    .getByRole('radio', { name: side === 'panel' ? 'Panel values' : 'File values', exact: true })
    .locator('..')
    .click();
  const result = dialog(page).getByRole('radio', { name: 'Full result', exact: true });
  if (await result.count()) await result.locator('..').click();
  await dialog(page).locator('.cm-editor').waitFor();
}
async function capture(page: Page, name: string) {
  const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
  if (!directory) return;
  await mkdir(directory, { recursive: true });
  await dialog(page).evaluate((node) => {
    node.querySelector('.modal-body')?.scrollTo({ top: 0, left: 0, behavior: 'instant' });
  });
  await page.evaluate(() => document.fonts.ready);
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  await page.mouse.move(0, 0);
  await dialog(page).screenshot({ path: join(directory, `${name}.png`) });
}
async function save(page: Page) {
  const response = page.waitForResponse(
    (response) => response.request().method() === 'PUT' && response.url().endsWith('/settings'),
  );
  const button = page.getByRole('button', { name: 'Save', exact: true });
  await expect.poll(() => button.isEnabled()).toBe(true);
  await button.click();
  expect((await response).status()).toBe(200);
  await expect.poll(() => page.locator('[data-unsaved]').count()).toBe(0);
}

describe('fresh configuration-file review [Browser]', () => {
  it('shows both competing values before a choice and keeps view separate from the outcome', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    page.setDefaultTimeout(8000);
    let writes = 0;
    try {
      await page.route('**/config-file/preview', (route) =>
        route.fulfill({
          json: mockConfigFilePreview('conflict', 'smykla-skalski/edge-proxy', 'a'.repeat(64)),
        }),
      );
      page.on('request', (request) => {
        if (request.url().endsWith('/resolution')) writes++;
      });
      await open(page, repo('edge-proxy'));
      const comparison = dialog(page).getByRole('region', {
        name: 'Conflicting settings',
        exact: true,
      });
      await comparison.waitFor();
      expect(await comparison.innerText()).toContain('/panel');
      expect(await comparison.innerText()).toContain('/file');
      expect(await comparison.locator('.is-del').innerText()).toContain('/panel');
      expect(await comparison.locator('.is-add').innerText()).toContain('/file');
      const beforeLines = await comparison.locator('.ln').count();
      const reveal = comparison
        .getByRole('button', { name: /^Show \d+ unchanged lines?$/ })
        .first();
      await reveal.click();
      expect(await comparison.locator('.ln').count()).toBeGreaterThan(beforeLines);
      expect((await comparison.locator('.ln .src').last().innerText()).trim()).not.toBe('');
      const hide = comparison.getByRole('button', { name: /^Hide \d+ unchanged lines?$/ }).first();
      expect(await hide.evaluate((node) => document.activeElement === node)).toBe(true);
      expect(await hide.getAttribute('aria-expanded')).toBe('true');
      await hide.click();
      expect(await comparison.locator('.ln').count()).toBe(beforeLines);
      expect(await reveal.evaluate((node) => document.activeElement === node)).toBe(true);
      expect(await reveal.getAttribute('aria-expanded')).toBe('false');
      expect(
        await dialog(page).getByRole('radio', { name: 'Conflicts', exact: true }).isChecked(),
      ).toBe(true);
      expect(
        await dialog(page).getByRole('radio', { name: 'Full result', exact: true }).isDisabled(),
      ).toBe(true);
      expect(
        await dialog(page).getByRole('radio', { name: 'Panel values', exact: true }).isChecked(),
      ).toBe(false);
      expect(
        await dialog(page).getByRole('radio', { name: 'File values', exact: true }).isChecked(),
      ).toBe(false);
      expect(
        await dialog(page).getByRole('button', { name: 'Apply choice', exact: true }).isDisabled(),
      ).toBe(true);
      expect(await dialog(page).locator('.cm-editor').count()).toBe(0);
      await dialog(page).getByText('/command_prefix', { exact: true }).waitFor();
      await dialog(page)
        .getByRole('radio', { name: 'File values', exact: true })
        .locator('..')
        .click();
      expect(
        await dialog(page).getByRole('radio', { name: 'Conflicts', exact: true }).isChecked(),
      ).toBe(true);
      expect(
        await dialog(page)
          .getByRole('button', { name: 'Use file values', exact: true })
          .isEnabled(),
      ).toBe(true);
      expect(writes).toBe(0);
      await dialog(page)
        .getByRole('radio', { name: 'Full result', exact: true })
        .locator('..')
        .click();
      await dialog(page).locator('.cm-editor').waitFor();
      expect(await comparison.count()).toBe(0);
      expect(await dialog(page).locator('.cm-content').innerText()).toContain(
        '"command_prefix": "/file "',
      );
      await dialog(page)
        .getByRole('radio', { name: 'Conflicts', exact: true })
        .locator('..')
        .click();
      await comparison.waitFor();
      expect(await dialog(page).locator('.cm-editor').count()).toBe(0);
      expect(
        await dialog(page).getByRole('radio', { name: 'Conflicts', exact: true }).isChecked(),
      ).toBe(true);
      expect(
        await dialog(page).getByRole('radio', { name: 'File values', exact: true }).isChecked(),
      ).toBe(true);
      expect(writes).toBe(0);
    } finally {
      await page.close();
    }
  });
  it.each([
    { side: 'panel' as const, repository: 'edge-proxy' },
    { side: 'file' as const, repository: 'feature-flags' },
  ])(
    'submits $side with independent edits and exact numbers, then remains pending',
    async ({ side, repository }) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1000 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(8000);
      const writes: string[] = [];
      page.on('request', (request) => {
        if (request.url().endsWith('/resolution')) writes.push(request.postData() ?? '');
      });
      try {
        await open(page, repo(repository));
        expect(await dialog(page).locator('.cm-editor').count()).toBe(0);
        expect(writes).toHaveLength(0);
        await choose(page, side);
        const editor = dialog(page).locator('.cm-content');
        const text = await editor.innerText();
        expect(text).toContain('9007199254740993');
        expect(text).toContain('1e-400');
        expect(text).toContain('"quiet_success": false');
        expect(text).toContain('"allow_self_approval": true');
        expect(await editor.getAttribute('contenteditable')).toBe('false');
        expect(await dialog(page).locator('.cm-editor').count()).toBe(1);
        expect(writes).toHaveLength(0);
        const response = page.waitForResponse((response) => response.url().endsWith('/resolution'));
        await dialog(page)
          .getByRole('button', {
            name: side === 'panel' ? 'Use panel values' : 'Use file values',
            exact: true,
          })
          .click();
        expect((await response).status()).toBe(202);
        await dialog(page)
          .getByText('Your choice was accepted · changes are waiting to sync')
          .waitFor();
        expect(JSON.parse(writes[0]!)).toEqual({
          review_token: expect.stringMatching(/^[a-f0-9]{64}$/u),
          side,
        });
        expect(writes).toHaveLength(1);
        expect(
          await dialog(page)
            .getByRole('button', { name: /^Use .* values$/ })
            .count(),
        ).toBe(0);
        await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
        await expect.poll(() => dialog(page).count()).toBe(0);
        await card(page).getByText('Sync pending', { exact: true }).waitFor();
        await page.reload();
        await card(page).getByText('Sync pending', { exact: true }).waitFor();
      } finally {
        await page.close();
      }
    },
  );

  it('refreshes a409 comparison and requires another explicit selection', async () => {
    const page = await panel.browser.newPage();
    page.setDefaultTimeout(8000);
    let reads = 0;
    const writes: { side: string; review_token: string }[] = [];
    try {
      await page.route('**/config-file/preview', (route) =>
        route.fulfill({
          json: mockConfigFilePreview(
            'conflict',
            'smykla-skalski/edge-proxy',
            (++reads === 1 ? 'a' : 'b').repeat(64),
          ),
        }),
      );
      await page.route('**/config-file/resolution', (route) => {
        writes.push(route.request().postDataJSON());
        return route.fulfill(
          writes.length === 1
            ? {
                status: 409,
                json: {
                  error: { code: 'config_file_changed', message: 'Settings or the file changed' },
                },
              }
            : { status: 202, json: { status: 'pending' } },
        );
      });
      await open(page, repo('edge-proxy'));
      await choose(page, 'file');
      await dialog(page).getByRole('button', { name: 'Use file values', exact: true }).click();
      await expect.poll(() => reads).toBe(2);
      await dialog(page)
        .getByText('Settings or the file changed · review the new comparison and choose again')
        .waitFor();
      expect(
        await dialog(page).getByRole('radio', { name: 'Conflicts', exact: true }).isChecked(),
      ).toBe(true);
      expect(
        await dialog(page).getByRole('radio', { name: 'Full result', exact: true }).isDisabled(),
      ).toBe(true);
      await dialog(page)
        .getByRole('region', { name: 'Conflicting settings', exact: true })
        .waitFor();
      expect(await dialog(page).locator('.cm-editor').count()).toBe(0);
      expect(
        await dialog(page).getByRole('radio', { name: 'File values', exact: true }).isChecked(),
      ).toBe(false);
      expect(writes).toHaveLength(1);
      await choose(page, 'panel');
      await dialog(page).getByRole('button', { name: 'Use panel values', exact: true }).click();
      await dialog(page)
        .getByText('Your choice was accepted · changes are waiting to sync')
        .waitFor();
      expect(writes[1]).toEqual({ side: 'panel', review_token: 'b'.repeat(64) });
    } finally {
      await page.close();
    }
  });

  it('shows recreation immediately but writes only after Recreate file is pressed', async () => {
    const page = await panel.browser.newPage();
    page.setDefaultTimeout(8000);
    let writes = 0;
    page.on('request', (request) => {
      if (request.url().endsWith('/resolution')) writes++;
    });
    try {
      await visit(page, repo('event-consumer'), {
        ready: '[aria-label="Configuration file sync"]',
      });
      const useFile = page.getByRole('checkbox', { name: 'Use file settings', exact: true });
      expect(await useFile.isChecked()).toBe(false);
      await card(page).getByRole('button', { name: 'Review file', exact: true }).click();
      await dialog(page)
        .getByText('Turn on Use file settings and save before reviewing changes')
        .waitFor();
      expect(await dialog(page).locator('.cm-editor').count()).toBe(0);
      await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
      await useFile.locator('..').click();
      await save(page);
      await card(page).getByRole('button', { name: 'Review file', exact: true }).click();
      await dialog(page).locator('.cm-editor').waitFor();
      expect(await dialog(page).getByRole('group', { name: 'Keep values from' }).count()).toBe(0);
      expect(writes).toBe(0);
      await dialog(page).getByRole('button', { name: 'Recreate file', exact: true }).click();
      await dialog(page)
        .getByText('Your choice was accepted · changes are waiting to sync')
        .waitFor();
      expect(writes).toBe(1);
      await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
      await useFile.locator('..').click();
      await save(page);
    } finally {
      await page.close();
    }
  });

  it.each([false, true])(
    'uses saved workspace opt-in and the correct API in Root=%s',
    async (root) => {
      const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
      page.setDefaultTimeout(8000);
      const writes: string[] = [];
      page.on('request', (request) => {
        if (request.url().endsWith('/resolution')) writes.push(request.url());
      });
      try {
        await visit(
          page,
          `${panel.origin}/${root ? 'root/workspaces' : 'workspace'}/${panel.account}/settings`,
          { ready: '[aria-label="Configuration file sync"]' },
        );
        const enabled = card(page).getByRole('checkbox', { name: 'Sync settings with file' });
        expect(await enabled.isChecked()).toBe(false);
        expect(
          await card(page)
            .getByRole('button', { name: /^Review / })
            .count(),
        ).toBe(0);
        await enabled.locator('..').click();
        expect(
          await card(page)
            .getByRole('button', { name: /^Review / })
            .count(),
        ).toBe(0);
        await save(page);
        await card(page)
          .getByRole('button', { name: /^Review / })
          .click();
        await choose(page, root ? 'file' : 'panel');
        expect(await dialog(page).locator('.cm-content').innerText()).toContain(
          '"scope": "workspace"',
        );
        await dialog(page)
          .getByRole('button', { name: root ? 'Use file values' : 'Use panel values', exact: true })
          .click();
        await dialog(page)
          .getByText('Your choice was accepted · changes are waiting to sync')
          .waitFor();
        expect(writes).toHaveLength(1);
        expect(writes[0]).toContain(root ? '/root/workspaces/' : '/targets/');
        expect(writes[0]).not.toContain('/repositories/');
        await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
        await enabled.locator('..').click();
        await save(page);
      } finally {
        await page.close();
      }
    },
  );

  it('blocks a fresh comparison when another repository has a draft in the same workspace', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    page.setDefaultTimeout(8000);
    let reads = 0;
    page.on('request', (request) => {
      if (request.url().endsWith('/preview')) reads++;
    });
    try {
      await visit(page, repo('auth-service'), { ready: '[aria-label="Configuration file sync"]' });
      await card(page)
        .getByRole('checkbox', { name: 'Sync settings with file' })
        .locator('..')
        .click();
      await page
        .getByRole('navigation', { name: 'Pages', exact: true })
        .getByRole('link', { name: /^Repositories(?:$|\s)/ })
        .click();
      await page.locator(`a[href="/workspace/${panel.account}/repositories/edge-proxy"]`).click();
      await card(page)
        .getByRole('button', { name: /^Review / })
        .click();
      await dialog(page)
        .getByText('Save or discard this workspace’s changes before comparing saved settings')
        .waitFor();
      expect(reads).toBe(0);
      expect(await dialog(page).locator('.cm-editor').count()).toBe(0);
    } finally {
      await page.close();
    }
  });

  it.each([410, 422])(
    'clears write authority after HTTP %s without repeating a resolution',
    async (status) => {
      const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
      page.setDefaultTimeout(8000);
      let writes = 0;
      try {
        await page.route('**/config-file/preview', (route) =>
          route.fulfill({
            json: mockConfigFilePreview('conflict', 'smykla-skalski/edge-proxy', 'd'.repeat(64)),
          }),
        );
        await page.route('**/config-file/resolution', (route) => {
          writes++;
          return route.fulfill({
            status,
            json: {
              error: {
                code: 'review_blocked',
                message:
                  status === 410
                    ? 'The operator visit expired'
                    : 'The file no longer accepts this choice',
              },
            },
          });
        });
        await open(page, repo('edge-proxy', true));
        await choose(page, 'panel');
        await dialog(page).getByRole('button', { name: 'Use panel values', exact: true }).click();
        await dialog(page)
          .getByRole('alert')
          .filter({
            hasText:
              status === 410
                ? 'The operator visit expired'
                : 'The file no longer accepts this choice',
          })
          .waitFor();
        expect(
          await dialog(page)
            .getByRole('button', { name: /^Use .* values$/ })
            .count(),
        ).toBe(0);
        expect(await dialog(page).locator('.cm-editor').count()).toBe(0);
        expect(writes).toBe(1);
      } finally {
        await page.close();
      }
    },
  );

  it.each(['close', 'navigate'] as const)(
    'ignores an old response after %s and another comparison opens',
    async (action) => {
      const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
      page.setDefaultTimeout(8000);
      let release = () => {};
      const pending = new Promise<void>((resolve) => {
        release = resolve;
      });
      let reads = 0;
      try {
        await page.route('**/config-file/preview', async (route) => {
          const old = ++reads === 1;
          if (old) await pending;
          await route.fulfill({
            json: mockConfigFilePreview(
              old ? 'invalid' : 'conflict',
              'smykla-skalski/edge-proxy',
              (old ? 'a' : 'b').repeat(64),
            ),
          });
        });
        await visit(page, `${panel.origin}/workspace/${panel.account}/repositories`, {
          ready: 'a[href$="/repositories/edge-proxy"]',
        });
        await page.locator(`a[href="/workspace/${panel.account}/repositories/edge-proxy"]`).click();
        await card(page)
          .getByRole('button', { name: /^Review / })
          .click();
        await expect.poll(() => reads).toBe(1);
        if (action === 'close') {
          await dialog(page)
            .getByRole('button', { name: 'Close configuration file review', exact: true })
            .click();
        } else {
          // Browser Back dismisses the current repository route while its comparison is pending.
          await page.goBack();
          await page.goForward();
          await card(page).waitFor();
        }
        await card(page)
          .getByRole('button', { name: /^Review / })
          .click();
        await expect.poll(() => reads).toBe(2);
        await choose(page, 'file');
        const oldResponse = page.waitForResponse(
          async (response) =>
            response.url().endsWith('/preview') &&
            (await response.json()).problem === 'invalid_file',
        );
        release();
        await oldResponse;
        await page.evaluate(
          () =>
            new Promise<void>((resolve) =>
              requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
            ),
        );
        await expect
          .poll(() =>
            dialog(page).getByRole('radio', { name: 'File values', exact: true }).isChecked(),
          )
          .toBe(true);
        expect(
          await dialog(page).getByText('The configuration file contains invalid TOML').count(),
        ).toBe(0);
        expect(
          await dialog(page)
            .getByRole('button', { name: 'Use file values', exact: true })
            .isEnabled(),
        ).toBe(true);
      } finally {
        release();
        await page.close();
      }
    },
  );

  it('keeps viewer inspection read-only and uses existing Root visit authority', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    page.setDefaultTimeout(8000);
    let writes = 0;
    try {
      await page.route('**/config-file/preview', (route) =>
        route.fulfill({
          json: mockConfigFilePreview('conflict', 'team-01/.github', 'f'.repeat(64), true),
        }),
      );
      await page.route('**/config-file/resolution', (route) => {
        writes++;
        return route.fulfill({ status: 202, json: { status: 'pending' } });
      });
      await open(page, `${panel.origin}/root/workspaces/team-01/settings`);
      await choose(page, 'panel');
      await dialog(page)
        .getByText('Read only · write access is required to apply a choice')
        .waitFor();
      expect(
        await dialog(page)
          .getByRole('button', { name: /^Use .* values$/ })
          .count(),
      ).toBe(0);
      expect(writes).toBe(0);
      await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
      // team-01 deliberately has stale Owners. A fresh-owner workspace is required
      // for a real operator visit; do not bypass that existing authorization guard.
      await visit(page, `${panel.origin}/root/workspaces/team-11/settings`, {
        ready: '[aria-label="Configuration file sync"]',
      });
      await page.getByRole('button', { name: 'Visit as an operator', exact: true }).click();
      const consent = page.getByRole('dialog', { name: /^Visit .* as an operator$/ });
      await consent.getByRole('checkbox').locator('..').click();
      await consent.getByRole('button', { name: 'Start a 15-minute visit', exact: true }).click();
      await expect.poll(() => consent.count()).toBe(0);
      const enabled = card(page).getByRole('checkbox', { name: 'Sync settings with file' });
      await enabled.locator('..').click();
      await save(page);
      await card(page)
        .getByRole('button', { name: /^Review / })
        .click();
      await choose(page, 'file');
      await dialog(page).getByRole('button', { name: 'Use file values', exact: true }).click();
      await dialog(page)
        .getByText('Your choice was accepted · changes are waiting to sync')
        .waitFor();
      expect(writes).toBe(1);
      await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
      await enabled.locator('..').click();
      await save(page);
      await page.getByRole('button', { name: 'End the visit', exact: true }).click();
      await expect.poll(() => card(page).getByRole('checkbox').isDisabled()).toBe(true);
    } finally {
      await page.close();
    }
  });

  it('offers a normal workspace viewer the preview without write controls', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    page.setDefaultTimeout(8000);
    let writes = 0;
    try {
      await page.route('**/api/v1/targets', async (route) => {
        const response = await route.fetch();
        const body = await response.json();
        for (const target of body.targets) {
          target.capabilities.write = false;
          target.effective_role = 'viewer';
          target.access_source = 'explicit';
        }
        await route.fulfill({ response, json: body });
      });
      await page.route('**/config-file/preview', (route) =>
        route.fulfill({
          json: mockConfigFilePreview('conflict', 'smykla-skalski/edge-proxy', 'a'.repeat(64)),
        }),
      );
      page.on('request', (request) => {
        if (request.url().endsWith('/resolution')) writes++;
      });
      await open(page, repo('edge-proxy'));
      await choose(page, 'file');
      await dialog(page)
        .getByText('Read only · write access is required to apply a choice')
        .waitFor();
      expect(
        await dialog(page)
          .getByRole('button', { name: /^Use .* values$/ })
          .count(),
      ).toBe(0);
      expect(writes).toBe(0);
    } finally {
      await page.close();
    }
  });

  it.each(['close', 'navigate'] as const)(
    'ignores a delayed accepted resolution after %s',
    async (action) => {
      const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
      page.setDefaultTimeout(8000);
      let release = () => {};
      const pending = new Promise<void>((resolve) => {
        release = resolve;
      });
      let writes = 0;
      try {
        await page.route('**/config-file/preview', (route) =>
          route.fulfill({
            json: mockConfigFilePreview('conflict', 'smykla-skalski/edge-proxy', 'a'.repeat(64)),
          }),
        );
        await page.route('**/config-file/resolution', async (route) => {
          writes++;
          await pending;
          await route.fulfill({ status: 202, json: { status: 'pending' } });
        });
        await open(page, repo('edge-proxy'));
        await choose(page, 'panel');
        await dialog(page).getByRole('button', { name: 'Use panel values', exact: true }).click();
        await expect.poll(() => writes).toBe(1);
        expect(
          await dialog(page)
            .getByRole('button', { name: 'Applying choice', exact: true })
            .isDisabled(),
        ).toBe(true);
        await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
        if (action === 'navigate') {
          await page
            .getByRole('navigation', { name: 'Pages', exact: true })
            .getByRole('link', { name: 'Repositories', exact: true })
            .click();
          await page
            .locator(`a[href="/workspace/${panel.account}/repositories/feature-flags"]`)
            .click();
        }
        await card(page)
          .getByRole('button', { name: /^Review / })
          .click();
        await choose(page, 'file');
        const response = page.waitForResponse((response) => response.url().endsWith('/resolution'));
        release();
        await response;
        await page.evaluate(
          () =>
            new Promise<void>((resolve) =>
              requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
            ),
        );
        expect(
          await dialog(page)
            .getByText('Your choice was accepted · changes are waiting to sync')
            .count(),
        ).toBe(0);
        expect(
          await dialog(page)
            .getByRole('button', { name: 'Use file values', exact: true })
            .isEnabled(),
        ).toBe(true);
        expect(writes).toBe(1);
      } finally {
        release();
        await page.close();
      }
    },
  );

  it.each(
    ['light', 'dark'].flatMap((theme) => [375, 768, 1024, 1440].map((width) => ({ theme, width }))),
  )('keeps one readable inspector surface in $theme at $width', async ({ theme, width }) => {
    const page = await panel.browser.newPage({
      viewport: { width, height: 1000 },
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(8000);
    const errors: string[] = [];
    let variant: ConfigFileReviewVariant = 'conflict';
    page.on('pageerror', (error) => errors.push(error.message));
    try {
      await page.route('**/config-file/preview', (route) =>
        route.fulfill({
          json: mockConfigFilePreview(
            variant,
            route.request().url().includes('/repositories/')
              ? 'smykla-skalski/edge-proxy'
              : 'team-01/.github',
            'c'.repeat(64),
            !route.request().url().includes('/repositories/'),
          ),
        }),
      );
      await open(page, repo('edge-proxy'));
      await page.evaluate((theme) => {
        document.documentElement.dataset.theme = theme;
      }, theme);
      expect(await page.locator('html').getAttribute('data-theme')).toBe(theme);
      await dialog(page)
        .getByRole('region', { name: 'Conflicting settings', exact: true })
        .waitFor();
      await capture(page, `conflicts-${theme}-${width}`);
      await choose(page, 'panel');
      await capture(page, `panel-${theme}-${width}`);
      await choose(page, 'file');
      await capture(page, `file-${theme}-${width}`);
      const geometry = await dialog(page).evaluate((node) => {
        const rect = node.getBoundingClientRect();
        const picker = node.querySelector('fieldset')!.getBoundingClientRect();
        const close = node
          .querySelector('[aria-label="Close configuration file review"]')!
          .getBoundingClientRect();
        const body = node.querySelector('.modal-body')!;
        const view = node.querySelector('.review-code fieldset')!.getBoundingClientRect();
        const metadata = node.querySelector('.review-code .card-meta')!.getBoundingClientRect();
        const path = node.querySelector('.review-path')!;
        const name = path.querySelector('span')!.getBoundingClientRect();
        const pointer = path.querySelector('code')!.getBoundingClientRect();
        return {
          left: rect.left,
          right: rect.right,
          viewport: innerWidth,
          scroll: body.scrollWidth,
          width: body.clientWidth,
          pickerWidth: picker.width,
          pickerHeight: picker.height,
          closeWidth: close.width,
          closeHeight: close.height,
          viewCenter: (view.top + view.bottom) / 2,
          metadataCenter: (metadata.top + metadata.bottom) / 2,
          pathGap: pointer.left - name.right,
        };
      });
      expect(geometry.left).toBeGreaterThanOrEqual(0);
      expect(geometry.right).toBeLessThanOrEqual(width);
      expect(geometry.scroll).toBeLessThanOrEqual(geometry.width + 1);
      expect(geometry.pickerHeight).toBe(34);
      expect(geometry.pickerWidth).toBeLessThan(geometry.width);
      expect(geometry.closeWidth).toBe(geometry.closeHeight);
      expect(Math.abs(geometry.viewCenter - geometry.metadataCenter)).toBeLessThanOrEqual(1);
      expect(geometry.pathGap).toBe(8);
      expect(await dialog(page).locator('.cm-editor').count()).toBe(1);
      expect((await dialog(page).locator('.cm-line').last().innerText()).trim()).not.toBe('');
      await page.keyboard.press('Escape');
      await expect.poll(() => dialog(page).count()).toBe(0);
      await expect
        .poll(() =>
          card(page)
            .getByRole('button', { name: /^Review (conflicts|file)$/ })
            .evaluate((node) => document.activeElement === node),
        )
        .toBe(true);
      for (const state of ['removed', 'invalid', 'schema', 'scope', 'outstanding'] as const) {
        variant = state;
        await card(page)
          .getByRole('button', { name: /^Review / })
          .click();
        await expect
          .poll(() => dialog(page).getByText('Comparing saved settings with the file').count())
          .toBe(0);
        if (state === 'removed') {
          await dialog(page).locator('.cm-editor').waitFor();
          expect(await dialog(page).getByRole('group', { name: 'Keep values from' }).count()).toBe(
            0,
          );
        } else {
          expect(await dialog(page).locator('.cm-editor').count()).toBe(0);
          expect(
            await dialog(page)
              .getByRole('button', { name: /^Use .* values$/ })
              .count(),
          ).toBe(0);
        }
        if (state === 'outstanding')
          expect(
            await dialog(page)
              .getByRole('link', { name: 'Review pull request #78' })
              .getAttribute('href'),
          ).toBe('https://github.com/smykla-skalski/edge-proxy/pull/78');
        await capture(page, `${state}-${theme}-${width}`);
        await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
      }
      variant = 'missingAccess';
      await open(page, `${panel.origin}/workspace/bart/settings`);
      await page.evaluate((theme) => {
        document.documentElement.dataset.theme = theme;
      }, theme);
      await dialog(page)
        .getByText('Give Smyklot access to the .github repository to sync workspace settings')
        .waitFor();
      await capture(page, `missingAccess-${theme}-${width}`);
      await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
      variant = 'conflict';
      await open(page, `${panel.origin}/root/workspaces/team-01/settings`);
      await page.evaluate((theme) => {
        document.documentElement.dataset.theme = theme;
      }, theme);
      await choose(page, 'panel');
      await dialog(page)
        .getByText('Read only · write access is required to apply a choice')
        .waitFor();
      await capture(page, `root-read-only-${theme}-${width}`);
      await dialog(page).getByRole('button', { name: 'Done', exact: true }).click();
      await card(page).scrollIntoViewIfNeeded();
      const header = await card(page)
        .locator('.card-head')
        .evaluate((node) => {
          const action = node.querySelector('button')!.getBoundingClientRect();
          const metadata = node.querySelector('.card-meta')!.getBoundingClientRect();
          return {
            action: (action.top + action.bottom) / 2,
            metadata: (metadata.top + metadata.bottom) / 2,
          };
        });
      expect(Math.abs(header.action - header.metadata)).toBeLessThanOrEqual(1);
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (directory)
        await card(page).screenshot({ path: join(directory, `root-card-${theme}-${width}.png`) });
      expect(errors).toEqual([]);
    } finally {
      await page.close();
    }
  });
});
