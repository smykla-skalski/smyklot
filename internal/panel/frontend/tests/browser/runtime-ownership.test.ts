import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';
import type { RootRuntimeSettings, RootRuntimeSettingsInput } from '../../src/lib/types';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop runtime override ownership', () => {
  it.each(['light', 'dark'] as const)(
    'recovers rejected YAML formatting with keyboard focus in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        if (scene === 'rejected') return;
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        if (scene !== 'returned')
          await page
            .locator('[data-settings-field="formatting.yaml.quote_style"]')
            .evaluate((node) => node.scrollIntoView({ block: 'center' }));
        await page.mouse.move(1, 1);
        await page.screenshot({
          path: join(directory, `F04-formatting-choice-server-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: null,
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                field: 'bot_config.formatting.yaml.quote_style',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const yaml = page
          .getByRole('group', { name: 'Formatting file type' })
          .getByRole('radio', { name: 'YAML', exact: true });
        await yaml.locator('..').click();
        const choices = page.getByRole('group', { name: 'Quote Style', exact: true });
        const prefix = choices.getByRole('radio', { name: 'Prefer Single', exact: true });
        await prefix.locator('..').click();
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        await expect
          .poll(() => prefix.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(await prefix.isChecked()).toBe(true);
        const description = await prefix.getAttribute('aria-describedby');
        expect(description).toBeTruthy();
        expect(await page.locator(`[id="${description?.split(' ').at(-1)}"]`).textContent()).toBe(
          'Runtime settings were rejected by the service',
        );
        const rowBounds = await page
          .locator('[data-settings-field="formatting.yaml.quote_style"]')
          .boundingBox();
        const composerBounds = await page.locator('.settings-composer').boundingBox();
        expect(rowBounds!.y + rowBounds!.height).toBeLessThan(composerBounds!.y);
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        expect(await prefix.isChecked()).toBe(true);
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await capture('restored');
        await choices
          .getByRole('radio', { name: 'Prefer Double', exact: true })
          .locator('..')
          .click();
        await expect.poll(() => prefix.getAttribute('aria-invalid')).toBeNull();
        expect(await prefix.getAttribute('aria-describedby')).toBeNull();
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await yaml.locator('..').click();
        await prefix.waitFor();
        expect(
          await choices.getByRole('radio', { name: 'Prefer Double', exact: true }).isChecked(),
        ).toBe(true);
        expect((await read()).behavior_defaults.intent.overrides.formatting.yaml.quote_style).toBe(
          'prefer_double',
        );
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'recovers rejected numeric formatting with keyboard focus in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        if (scene === 'rejected') return;
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        if (scene !== 'returned')
          await page
            .locator('[data-settings-field="formatting.common.indent_width"]')
            .evaluate((node) => node.scrollIntoView({ block: 'center' }));
        await page.mouse.move(1, 1);
        await page.screenshot({
          path: join(directory, `F04-formatting-number-server-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: null,
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                field: 'bot_config.formatting.common.indent_width',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const prefix = page.getByLabel('Indent Width', { exact: true });
        await prefix.fill('4');
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        await expect
          .poll(() => prefix.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(await prefix.inputValue()).toBe('4');
        expect(await prefix.getAttribute('aria-invalid')).toBe('true');
        const description = await prefix.getAttribute('aria-describedby');
        expect(description).toBeTruthy();
        expect(await page.locator(`[id="${description?.split(' ').at(-1)}"]`).textContent()).toBe(
          'Runtime settings were rejected by the service',
        );
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('4');
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await capture('restored');
        await prefix.fill('6');
        await expect.poll(() => prefix.getAttribute('aria-invalid')).toBeNull();
        expect(await prefix.getAttribute('aria-describedby')).not.toContain('server-problem');
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('6');
        expect(
          (await read()).behavior_defaults.intent.overrides.formatting.common.indent_width,
        ).toBe(6);
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'recovers rejected alias collection with keyboard focus in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        if (scene === 'rejected') return;
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(directory, `F04-alias-server-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: { version: 1, overrides: { command_aliases: { old: 'merge' } } },
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                field: 'bot_config.command_aliases',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const prefix = page.getByRole('textbox', { name: /^Alias / });
        await prefix.fill('ship');
        await prefix.press('Enter');
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        await expect
          .poll(() => prefix.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(await prefix.inputValue()).toBe('ship');
        const description = await prefix.getAttribute('aria-describedby');
        expect(description).toBeTruthy();
        expect(await page.locator(`[id="${description}"]`).textContent()).toBe(
          'Runtime settings were rejected by the service',
        );
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('ship');
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await capture('restored');
        await prefix.fill('deploy');
        await prefix.press('Enter');
        await expect.poll(() => prefix.getAttribute('aria-invalid')).toBeNull();
        expect(await prefix.getAttribute('aria-describedby')).toBeNull();
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('deploy');
        expect((await read()).behavior_defaults.intent.overrides.command_aliases).toEqual({
          deploy: 'merge',
        });
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'recovers rejected log level with keyboard focus in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        if (scene === 'rejected') return;
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(
            directory,
            `${process.env.SMYKLOT_VISUAL_AUDIT_PREFIX ?? 'F04-log-server'}-${scene}-${colorScheme}.png`,
          ),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: null,
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                field: 'log_level',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        expect(
          await page
            .getByRole('region', { name: 'Service settings', exact: true })
            .getByText('Unsaved changes', { exact: true })
            .count(),
        ).toBe(0);
        await page.getByRole('button', { name: 'Override the deployment log level' }).click();
        await page
          .getByRole('region', { name: 'Service settings', exact: true })
          .getByText('Unsaved changes', { exact: true })
          .waitFor();
        const prefix = page.getByRole('button', { name: 'Runtime log level', exact: true });
        await prefix.click();
        await page.getByRole('option', { name: 'Debug', exact: true }).click();
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        await expect
          .poll(() => prefix.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(await prefix.textContent()).toContain('Debug');
        const description = await prefix.getAttribute('aria-describedby');
        expect(description).toBeTruthy();
        expect(await page.locator(`[id="${description}"]`).textContent()).toBe(
          'Runtime settings were rejected by the service',
        );
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.textContent()).toContain('Debug');
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await capture('restored');
        await prefix.click();
        await page.getByRole('option', { name: 'Warn', exact: true }).click();
        await expect.poll(() => prefix.getAttribute('aria-invalid')).toBeNull();
        expect(await prefix.getAttribute('aria-describedby')).toBeNull();
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.textContent()).toContain('Warn');
        expect((await read()).log_level.override).toBe('warn');
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        expect(
          await page
            .getByRole('region', { name: 'Service settings', exact: true })
            .getByText('Unsaved changes', { exact: true })
            .count(),
        ).toBe(0);
        expect(await page.getByText('Changes wait for Save', { exact: true }).count()).toBe(0);
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'recovers rejected boolean override with keyboard focus in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        if (scene === 'rejected') return;
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(directory, `F04-boolean-server-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: null,
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                field: 'bot_config.allow_draft_merges',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        expect(
          await page
            .getByRole('region', { name: 'Service settings', exact: true })
            .getByText('Unsaved changes', { exact: true })
            .count(),
        ).toBe(0);
        await page.getByRole('button', { name: 'Override another', exact: true }).click();
        await page.getByRole('button', { name: 'Merge draft pull requests', exact: true }).click();
        const prefix = page.getByRole('checkbox', {
          name: 'Merge draft pull requests',
          exact: true,
        });
        await prefix.locator('..').click();
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        await expect
          .poll(() => prefix.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(await prefix.isChecked()).toBe(true);
        const description = await prefix.getAttribute('aria-describedby');
        expect(description).toBeTruthy();
        expect(await page.locator(`[id="${description}"]`).textContent()).toBe(
          'Runtime settings were rejected by the service',
        );
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.isChecked()).toBe(true);
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await capture('restored');
        await prefix.locator('..').click();
        await expect.poll(() => prefix.getAttribute('aria-invalid')).toBeNull();
        expect(await prefix.getAttribute('aria-describedby')).toBeNull();
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.isChecked()).toBe(false);
        expect((await read()).behavior_defaults.intent.overrides.allow_draft_merges).toBe(false);
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        expect(
          await page
            .getByRole('region', { name: 'Service settings', exact: true })
            .getByText('Unsaved changes', { exact: true })
            .count(),
        ).toBe(0);
        expect(await page.getByText('Changes wait for Save', { exact: true }).count()).toBe(0);
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'recovers rejected command selection with keyboard focus in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        if (scene === 'rejected') return;
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(directory, `F04-command-server-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: {
            version: 1,
            overrides: { allowed_commands: ['approve', 'merge', 'help'] },
          },
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                field: 'bot_config.allowed_commands',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const row = page.locator('[data-settings-field="allowed_commands"]');
        const prefix = row.getByRole('checkbox', { name: 'approve', exact: true });
        await prefix.locator('..').click();
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        await expect
          .poll(() => prefix.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(await prefix.isChecked()).toBe(false);
        const description = await prefix.getAttribute('aria-describedby');
        expect(description).toBeTruthy();
        expect(await page.locator(`[id="${description}"]`).textContent()).toBe(
          'Runtime settings were rejected by the service',
        );
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.isChecked()).toBe(false);
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await row.evaluate((node) => node.scrollIntoView({ block: 'center' }));
        await capture('restored');
        await row.getByRole('checkbox', { name: 'squash', exact: true }).locator('..').click();
        await expect.poll(() => prefix.getAttribute('aria-invalid')).toBeNull();
        expect(await prefix.getAttribute('aria-describedby')).toBeNull();
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.isChecked()).toBe(false);
        expect((await read()).behavior_defaults.intent.overrides.allowed_commands).toEqual([
          'merge',
          'squash',
          'help',
        ]);
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        await row.evaluate((node) => node.scrollIntoView({ block: 'center' }));
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'recovers rejected session duration with keyboard focus in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        if (scene === 'rejected') return;
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(directory, `F04-duration-server-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: null,
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                field: 'session_ttl_seconds',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        await page
          .getByRole('button', { name: 'Override the deployment session lifetime' })
          .click();
        const prefix = page.getByLabel('Session lifetime amount', { exact: true });
        await prefix.fill('2');
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        await expect
          .poll(() => prefix.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(await prefix.inputValue()).toBe('2');
        expect(await prefix.getAttribute('aria-invalid')).toBe('true');
        const description = await prefix.getAttribute('aria-describedby');
        expect(description).toBeTruthy();
        expect(await page.locator(`[id="${description}"]`).textContent()).toBe(
          'Runtime settings were rejected by the service',
        );
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('2');
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await capture('restored');
        await prefix.fill('3');
        await expect.poll(() => prefix.getAttribute('aria-invalid')).toBe('false');
        expect(await prefix.getAttribute('aria-describedby')).toBeNull();
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('3');
        expect((await read()).session_lifetime.override_seconds).toBe(259200);
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'recovers persisted server field errors with keyboard focus in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(directory, `F04-inline-recovery-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: null,
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                field: 'bot_config.command_prefix',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const prefix = page.getByLabel('Prefix', { exact: true });
        await prefix.fill('/server-recovery');
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        await expect
          .poll(() => prefix.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(await prefix.inputValue()).toBe('/server-recovery');
        expect(await prefix.getAttribute('aria-invalid')).toBe('true');
        const description = await prefix.getAttribute('aria-describedby');
        expect(description).toBeTruthy();
        expect(await page.locator(`[id="${description}"]`).textContent()).toBe(
          'Runtime settings were rejected by the service',
        );
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('/server-recovery');
        await page.getByText('Fix the invalid setting before saving', { exact: true }).waitFor();
        expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
          true,
        );
        await capture('restored');
        await prefix.fill('/corrected');
        await expect.poll(() => prefix.getAttribute('aria-invalid')).toBeNull();
        expect(await prefix.getAttribute('aria-describedby')).toBeNull();
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('/corrected');
        expect((await read()).behavior_defaults.intent).toEqual({
          version: 1,
          overrides: { command_prefix: '/corrected' },
        });
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'preserves runtime drafts after server rejection and retries in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(directory, `F04-server-recovery-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await read();
        const input = {
          bot_config: null,
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: input })).status()).toBe(200);
        const baseline = await read();
        let reject = true;
        await page.route(endpoint, async (route) => {
          if (route.request().method() !== 'PUT' || !reject) return route.continue();
          await route.fulfill({
            status: 400,
            json: {
              error: {
                code: 'invalid_runtime_settings',
                message: 'Runtime settings were rejected by the service',
              },
            },
          });
        });
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const prefix = page.getByLabel('Prefix', { exact: true });
        await prefix.fill('/server-recovery');
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Settings were not saved', { exact: true }).waitFor();
        await page
          .getByText('Runtime settings were rejected by the service', { exact: true })
          .waitFor();
        const rejected = await read();
        expect(rejected).toEqual({
          ...baseline,
          service: { ...baseline.service, uptime_seconds: rejected.service.uptime_seconds },
        });
        await capture('rejected');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('/server-recovery');
        await capture('returned');
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('/server-recovery');
        reject = false;
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await prefix.waitFor();
        expect(await prefix.inputValue()).toBe('/server-recovery');
        expect((await read()).behavior_defaults.intent).toEqual({
          version: 1,
          overrides: { command_prefix: '/server-recovery' },
        });
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
        await capture('saved');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'pins an equal formatting number and resets only that field in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async () => (await page.request.get(endpoint)).json();
      const capture = async (scene: string) => {
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(directory, `F04-equal-formatting-${scene}-${colorScheme}.png`),
        });
      };
      const save = async () => {
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
      };
      try {
        const initial = await read();
        expect(
          (
            await page.request.put(endpoint, {
              data: {
                bot_config: null,
                log_level: null,
                reaction_poll_interval_seconds: null,
                merge_after_ci_quiet_period_seconds: null,
                path_index_interval_seconds: null,
                session_ttl_seconds: null,
                expected_revision: initial.revision,
              },
            })
          ).status(),
        ).toBe(200);
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        await page.getByLabel('Prefix', { exact: true }).fill('/pin');
        const width = page.getByLabel('Indent Width', { exact: true });
        await width.scrollIntoViewIfNeeded();
        await page.getByRole('button', { name: 'Override Indent Width at 2', exact: true }).click();
        expect(await width.inputValue()).toBe('2');
        await page
          .getByRole('button', { name: 'Stop overriding Indent Width', exact: true })
          .waitFor();
        await capture('draft');
        await save();
        await page.reload({ waitUntil: 'domcontentloaded' });
        await page
          .getByRole('button', { name: 'Stop overriding Indent Width', exact: true })
          .waitFor();
        expect((await read()).behavior_defaults.intent.overrides).toEqual({
          command_prefix: '/pin',
          formatting: { common: { indent_width: 2 } },
        });
        await width.scrollIntoViewIfNeeded();
        await capture('saved');
        await page
          .getByRole('button', { name: 'Stop overriding Indent Width', exact: true })
          .click();
        await save();
        await page.reload({ waitUntil: 'domcontentloaded' });
        await page
          .getByRole('button', { name: 'Override Indent Width at 2', exact: true })
          .waitFor();
        expect((await read()).behavior_defaults.intent.overrides).toEqual({
          command_prefix: '/pin',
        });
        await width.scrollIntoViewIfNeeded();
        await capture('reset');
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'retains invalid duration edits across navigation and focuses recovery in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      const capture = async (scene: string) => {
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({
          path: join(directory, `F04-validation-${scene}-${colorScheme}.png`),
        });
      };
      try {
        const initial = await (await page.request.get(endpoint)).json();
        expect(
          (
            await page.request.put(endpoint, {
              data: {
                bot_config: null,
                log_level: null,
                reaction_poll_interval_seconds: null,
                merge_after_ci_quiet_period_seconds: null,
                path_index_interval_seconds: null,
                session_ttl_seconds: null,
                expected_revision: initial.revision,
              },
            })
          ).status(),
        ).toBe(200);
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        await page.getByLabel('Prefix', { exact: true }).fill('/validation');
        await page
          .getByRole('button', { name: 'Override the deployment session lifetime' })
          .click();
        const amount = page.getByLabel('Session lifetime amount', { exact: true });
        await amount.fill('1e');
        await expect.poll(() => amount.getAttribute('aria-invalid')).toBe('true');
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isDisabled())
          .toBe(true);
        await capture('invalid');
        await visit(page, addressOf(panel, 'root/history/audit'));
        await page.reload({ waitUntil: 'domcontentloaded' });
        await page
          .getByText('Session lifetime must be between 1 minute and 30 days', { exact: true })
          .waitFor();
        await capture('away');
        await page.getByRole('link', { name: 'Open Service settings', exact: true }).click();
        await expect
          .poll(() => amount.evaluate((element) => element === document.activeElement))
          .toBe(true);
        expect(await amount.inputValue()).toBe('1e');
        expect(await page.getByLabel('Prefix', { exact: true }).inputValue()).toBe('/validation');
        await capture('recovery');
        await amount.fill('2');
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        const saved = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await saved).status()).toBe(200);
        await page.reload({ waitUntil: 'domcontentloaded' });
        expect(await amount.inputValue()).toBe('2');
        expect(await page.getByLabel('Prefix', { exact: true }).inputValue()).toBe('/validation');
        await capture('saved');
        expect(errors).toEqual([]);
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'restores checkpoint ownership and resets individual formatting fields in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async (): Promise<RootRuntimeSettings> =>
        (await page.request.get(endpoint)).json();
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      const capture = async (scene: string) => {
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await page.evaluate(() => document.fonts.ready);
        await page.mouse.move(0, 0);
        await page.screenshot({ path: join(directory, `F04-history-${scene}-${colorScheme}.png`) });
      };
      const save = async () => {
        const response = page.waitForResponse(
          (r) => r.url() === endpoint && r.request().method() === 'PUT',
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        expect((await response).status()).toBe(200);
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
      };
      try {
        const initial = await read();
        expect(
          (
            await page.request.put(endpoint, {
              data: {
                bot_config: null,
                background_work_paused: false,
                log_level: null,
                reaction_poll_interval_seconds: null,
                merge_after_ci_quiet_period_seconds: null,
                path_index_interval_seconds: null,
                session_ttl_seconds: null,
                expected_revision: initial.revision,
              },
            })
          ).ok(),
        ).toBe(true);
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const prefix = page.getByLabel('Prefix', { exact: true });
        const width = page.getByLabel('Indent Width', { exact: true });
        await prefix.fill('/history-test');
        await width.fill('4');
        await save();
        await page.reload({ waitUntil: 'domcontentloaded' });
        await expect.poll(() => width.inputValue()).toBe('4');
        expect((await read()).behavior_defaults.intent?.overrides).toEqual({
          command_prefix: '/history-test',
          formatting: { common: { indent_width: 4 } },
        });
        await width.scrollIntoViewIfNeeded();
        await capture('formatting-saved');
        for (const phase of ['clear', 'restore'] as const) {
          await visit(page, addressOf(panel, 'root/history/audit'));
          await page
            .getByRole('button', { name: /inspect the settings snapshot/ })
            .first()
            .click();
          const dialog = page.getByRole('dialog', { name: 'Settings history', exact: true });
          await dialog.getByText('After · Selected', { exact: true }).waitFor();
          await capture(`${phase}-after-preview`);
          await dialog
            .getByRole('radio', { name: 'Before change', exact: true })
            .locator('..')
            .click();
          expect(await dialog.getByText('Before · Selected', { exact: true }).count()).toBe(1);
          expect(await dialog.getByText('After · Selected', { exact: true }).count()).toBe(0);
          await capture(`${phase}-preview`);
          await dialog.getByRole('button', { name: 'Restore selected', exact: true }).click();
          await capture(`${phase}-confirm`);
          const response = page.waitForResponse(
            (r) => r.url().endsWith('/restore') && r.request().method() === 'POST',
          );
          await dialog.getByRole('button', { name: 'Confirm restore', exact: true }).click();
          expect((await response).status()).toBe(200);
          await dialog.waitFor({ state: 'hidden' });
          const intent = (await read()).behavior_defaults.intent;
          if (phase === 'clear') expect(intent).toBeNull();
          else
            expect(intent?.overrides).toEqual({
              command_prefix: '/history-test',
              formatting: { common: { indent_width: 4 } },
            });
        }
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        await expect.poll(() => width.inputValue()).toBe('4');
        await page
          .getByRole('button', { name: 'Stop overriding Indent Width', exact: true })
          .click();
        await save();
        expect((await read()).behavior_defaults.intent?.overrides).toEqual({
          command_prefix: '/history-test',
        });
        await page.reload({ waitUntil: 'domcontentloaded' });
        await expect
          .poll(() => width.inputValue())
          .toBe(String(initial.behavior_defaults.deployment.formatting.common.indent_width));
        await width.scrollIntoViewIfNeeded();
        await capture('formatting-reset');
        await prefix.fill(initial.behavior_defaults.deployment.command_prefix);
        await save();
        expect((await read()).behavior_defaults.intent).toBeNull();
        expect(errors).toEqual([]);
      } finally {
        await page.close();
      }
    },
  );
  it.each(['light', 'dark'] as const)(
    'keeps every override reachable above the save bar in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'no-preference',
      });
      page.setDefaultTimeout(10_000);
      try {
        const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
        const current = await (await page.request.get(endpoint)).json();
        expect(
          (
            await page.request.put(endpoint, {
              data: {
                bot_config: null,
                background_work_paused: false,
                log_level: null,
                reaction_poll_interval_seconds: null,
                merge_after_ci_quiet_period_seconds: null,
                path_index_interval_seconds: null,
                session_ttl_seconds: null,
                expected_revision: current.revision,
              },
            })
          ).ok(),
        ).toBe(true);
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const card = page.getByRole('region', { name: 'Behavior', exact: true });
        const trigger = card.getByRole('button', { name: 'Override another', exact: true });
        const menu = page.getByRole('dialog', { name: 'Behavior choices', exact: true });
        let added = 0;
        while (await trigger.count()) {
          if (added > 0) {
            // Put the trigger behind the floating bar to exercise native reveal.
            await trigger.evaluate((node) => {
              const bar = document.querySelector('.settings-composer')!.getBoundingClientRect();
              const box = node.getBoundingClientRect();
              window.scrollBy({ top: box.top - bar.top, behavior: 'instant' });
            });
          }
          await trigger.click();
          const option = menu.locator('.btn-add').first();
          const label = (await option.innerText()).trim();
          await option.click();
          await menu.waitFor({ state: 'hidden' });
          const control = card.getByRole('checkbox', { name: label, exact: true });
          await expect
            .poll(() => control.evaluate((node) => document.activeElement === node))
            .toBe(true);
          added += 1;
        }
        expect(added).toBe(10);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.evaluate(() => document.fonts.ready);
          await page.mouse.move(0, 0);
          await page.screenshot({ path: join(directory, `F04-all-overrides-${colorScheme}.png`) });
        }
        await page.getByRole('button', { name: 'Discard', exact: true }).click();
        await expect
          .poll(() =>
            page.evaluate(() =>
              document.documentElement.style.getPropertyValue('--settings-composer-clearance'),
            ),
          )
          .toBe('');
      } finally {
        await page.close();
      }
    },
  );
  it.each(['light', 'dark'] as const)(
    'saves equal values and resets ownership in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
      const read = async (): Promise<RootRuntimeSettings> =>
        (await page.request.get(endpoint)).json();
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      try {
        const initial = await read();
        const reset: RootRuntimeSettingsInput = {
          bot_config: null,
          background_work_paused: false,
          log_level: null,
          reaction_poll_interval_seconds: null,
          merge_after_ci_quiet_period_seconds: null,
          path_index_interval_seconds: null,
          session_ttl_seconds: null,
          expected_revision: initial.revision,
        };
        expect((await page.request.put(endpoint, { data: reset })).ok()).toBe(true);
        await visit(page, addressOf(panel, 'root/runtime/settings'));
        const behavior = page.getByRole('region', { name: 'Behavior', exact: true });
        const screenshot = async (scene: string) => {
          const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.evaluate(() => document.fonts.ready);
          await page.mouse.move(0, 0);
          await page.screenshot({ path: join(directory, `F04-${scene}-${colorScheme}.png`) });
        };
        const save = async () => {
          const response = page.waitForResponse(
            (response) => response.url() === endpoint && response.request().method() === 'PUT',
          );
          await page.getByRole('button', { name: 'Save', exact: true }).click();
          const result = await response;
          expect(result.status()).toBe(200);
          await expect
            .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
            .toBe(0);
          return result.json() as Promise<RootRuntimeSettings>;
        };
        await behavior.scrollIntoViewIfNeeded();
        await behavior.getByRole('button', { name: 'Override another', exact: true }).click();
        const choices = page.getByRole('dialog', { name: 'Behavior choices', exact: true });
        await choices.waitFor();
        await screenshot('picker');
        await choices.getByRole('button', { name: 'Success replies', exact: true }).click();
        const field = behavior.getByRole('checkbox', { name: 'Success replies', exact: true });
        await field.waitFor();
        await screenshot('equal-draft');
        const first = await save();
        expect(first.behavior_defaults.intent?.overrides).toEqual({
          quiet_success: initial.behavior_defaults.deployment.quiet_success,
        });
        expect(first.checkpoint_id).toBeDefined();
        await page.reload({ waitUntil: 'domcontentloaded' });
        await field.waitFor();
        await behavior.scrollIntoViewIfNeeded();
        await screenshot('equal-saved');
        await field.locator('..').click();
        await screenshot('changed-draft');
        const changed = await save();
        expect(changed.behavior_defaults.intent?.overrides.quiet_success).toBe(
          !first.behavior_defaults.intent!.overrides.quiet_success,
        );
        await page.reload({ waitUntil: 'domcontentloaded' });
        await field.waitFor();
        expect((await read()).behavior_defaults.effective.quiet_success).toBe(
          changed.behavior_defaults.intent!.overrides.quiet_success,
        );
        await behavior.scrollIntoViewIfNeeded();
        await behavior
          .locator('.policy-row')
          .filter({ has: page.getByRole('checkbox', { name: 'Success replies', exact: true }) })
          .getByRole('button', { name: 'Reset', exact: true })
          .click();
        await screenshot('reset-draft');
        const inherited = await save();
        expect(inherited.behavior_defaults.intent).toBeNull();
        await page.reload({ waitUntil: 'domcontentloaded' });
        await behavior.waitFor();
        await behavior.scrollIntoViewIfNeeded();
        expect(await field.count()).toBe(0);
        await screenshot('reset-saved');
        expect(errors).toEqual([]);
      } finally {
        await page.close();
      }
    },
  );
});

describe('desktop runtime conflict review', () => {
  it.each(
    (['light', 'dark'] as const).flatMap((colorScheme) =>
      (['draft', 'saved'] as const).map((choice) => ({ colorScheme, choice })),
    ),
  )('chooses $choice values in $colorScheme', async ({ colorScheme, choice }) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(10_000);
    const endpoint = `${panel.origin}/api/v1/root/runtime/settings`;
    const read = async (): Promise<RootRuntimeSettings> =>
      (await page.request.get(endpoint)).json();
    const input = (revision: number): RootRuntimeSettingsInput => ({
      bot_config: null,
      log_level: null,
      background_work_paused: false,
      reaction_poll_interval_seconds: null,
      merge_after_ci_quiet_period_seconds: null,
      path_index_interval_seconds: null,
      session_ttl_seconds: null,
      expected_revision: revision,
    });
    const shot = async (scene: string) => {
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.evaluate(() => document.fonts.ready);
      await page.mouse.move(0, 0);
      await page.screenshot({
        path: join(directory, `F04-conflict-${scene}-${choice}-${colorScheme}.png`),
      });
    };
    try {
      expect(
        (await page.request.put(endpoint, { data: input((await read()).revision) })).ok(),
      ).toBe(true);
      await visit(page, addressOf(panel, 'root/runtime/settings'));
      const prefix = page.getByLabel('Prefix', { exact: true });
      await prefix.fill('/mine');
      await page.getByLabel('Indent Width', { exact: true }).fill('4');
      const remote = input((await read()).revision);
      remote.bot_config = { version: 1, overrides: { command_prefix: '/theirs' } };
      expect((await page.request.put(endpoint, { data: remote })).ok()).toBe(true);
      await page.getByRole('button', { name: 'Update draft', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'Choose which values to keep', exact: true });
      await dialog.waitFor();
      expect(
        await dialog.getByRole('button', { name: 'Update draft', exact: true }).isDisabled(),
      ).toBe(true);
      await shot('unanswered');
      await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
      expect(await prefix.inputValue()).toBe('/mine');
      await page.getByRole('button', { name: 'Update draft', exact: true }).click();
      await dialog.waitFor();
      await dialog.getByRole('combobox', { name: 'Command prefix', exact: true }).click();
      await shot('choices');
      await page
        .getByRole('option', {
          name: choice === 'draft' ? 'My draft: /mine' : 'Saved in another session: /theirs',
          exact: true,
        })
        .click();
      await shot('selected');
      await dialog.getByRole('button', { name: 'Update draft', exact: true }).click();
      await dialog.waitFor({ state: 'hidden' });
      expect((await read()).behavior_defaults.intent?.overrides.command_prefix).toBe('/theirs');
      const response = page.waitForResponse(
        (response) => response.url() === endpoint && response.request().method() === 'PUT',
      );
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      expect((await response).status()).toBe(200);
      const saved = await read();
      expect(saved.behavior_defaults.intent?.overrides.command_prefix).toBe(
        choice === 'draft' ? '/mine' : '/theirs',
      );
      expect(saved.behavior_defaults.intent?.overrides.formatting?.common?.indent_width).toBe(4);
    } finally {
      await page.close();
    }
  });
});
