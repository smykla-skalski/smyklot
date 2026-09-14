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
