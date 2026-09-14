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
