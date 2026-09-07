import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { MOCK_BYPASS_ACTORS, MOCK_UNRESOLVED_BYPASS_ACTORS } from '../../dev/bypass-actors';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('unavailable bypass identities [Browser]', () => {
  it('saves and reloads an authored int64 role through the workspace API', async () => {
    const page = await panel.browser.newPage();
    page.setDefaultTimeout(10_000);
    try {
      await visit(page, addressOf(panel, 'workspace/settings'));
      await page.getByRole('combobox', { name: 'Bypass exception policy' }).click();
      await page.getByRole('option', { name: 'Allow listed actors', exact: true }).click();
      const card = page
        .locator('.card')
        .filter({ has: page.getByRole('heading', { name: 'Merge exceptions', exact: true }) });
      await card.getByRole('button', { name: 'Add an actor', exact: true }).click();
      await card.getByRole('combobox', { name: 'Who' }).click();
      await page.getByRole('option', { name: 'Repository role', exact: true }).click();
      await card.getByRole('combobox', { name: 'Role', exact: true }).click();
      await page.getByRole('option', { name: 'Custom repository role', exact: true }).click();
      await card
        .getByRole('textbox', { name: 'Custom repository role ID' })
        .fill('9223372036854775807');
      await card.getByRole('button', { name: 'Add actor', exact: true }).click();
      const saved = page.waitForResponse(
        (response) =>
          response.request().method() === 'PUT' &&
          new URL(response.url()).pathname.endsWith('/settings'),
      );
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const response = await saved;
      expect(response.request().postData()).toContain('"actor_id":9223372036854775807');
      expect(response.status(), await response.text()).toBe(200);
      expect(await response.text()).toContain('"actor_id":9223372036854775807');
      const checkpointId = (await response.json()).checkpoint_id;
      expect(checkpointId).toBeTruthy();
      const checkpoint = await page.request.get(
        `${panel.origin}/api/v1/targets/2001/settings/checkpoints/${checkpointId}`,
      );
      expect(checkpoint.status(), await checkpoint.text()).toBe(200);
      expect(await checkpoint.text()).toContain('"actor_id":9223372036854775807');
      await page.reload({ waitUntil: 'domcontentloaded' });
      await page
        .getByRole('combobox', {
          name: 'Bypass mode for Custom repository role, Repository role ID 9223372036854775807',
        })
        .waitFor();
      expect(await page.locator('.settings-composer').count()).toBe(0);
      await page
        .getByRole('combobox', {
          name: 'Bypass mode for Custom repository role, Repository role ID 9223372036854775807',
        })
        .click();
      await page.getByRole('option', { name: 'Pull requests only', exact: true }).click();
      const savedAgain = page.waitForResponse(
        (candidate) =>
          candidate.request().method() === 'PUT' &&
          new URL(candidate.url()).pathname.endsWith('/settings'),
      );
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const second = await savedAgain;
      expect(second.status(), await second.text()).toBe(200);
      expect(second.request().postData()).toContain('"actor_id":9223372036854775807');
    } finally {
      await page.close();
    }
  });

  it.each([
    { colorScheme: 'light', width: 1440 },
    { colorScheme: 'dark', width: 1440 },
    { colorScheme: 'light', width: 1024 },
    { colorScheme: 'dark', width: 1024 },
    { colorScheme: 'light', width: 768 },
    { colorScheme: 'dark', width: 768 },
    { colorScheme: 'light', width: 375 },
    { colorScheme: 'dark', width: 375 },
  ] as const)(
    'distinguishes saved actors and recovers names at $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: width === 375 ? 1500 : 1100 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      let recovered = false;
      let lookups = 0;
      try {
        await page.route('**/api/v1/targets/*/sync/config/rulesets', async (route) => {
          const response = await route.fetch();
          const config = await response.json();
          config.document.rulesets[0].bypass_actors = [
            { actor_type: 'Integration', actor_id: 1197525, bypass_mode: 'always' },
            ...MOCK_UNRESOLVED_BYPASS_ACTORS.map((actor) =>
              actor.actor_type === 'RepositoryRole' && actor.actor_id === 902
                ? { ...actor, actor_id: JSON.rawJSON('9223372036854775807') }
                : actor,
            ),
          ];
          await route.fulfill({ response, json: config });
        });
        await page.route('**/api/v1/targets/*/bypass-actors?*', async (route) => {
          lookups++;
          await route.fulfill({
            json: {
              items: MOCK_BYPASS_ACTORS.filter(
                (actor) => actor.actor_id === 1197525 || (recovered && actor.actor_id === 2740),
              ),
            },
          });
        });
        await visit(page, addressOf(panel, 'workspace/sync/rulesets/main-protection'));
        const card = page
          .locator('.card')
          .filter({ has: page.getByRole('heading', { name: 'Bypass list', exact: true }) });
        await card.getByRole('button', { name: 'Retry names', exact: true }).waitFor();
        expect(await card.locator('.actor-row').count()).toBe(9);
        expect(
          await card.getByRole('button', { name: 'Remove smyklot', exact: true }).count(),
        ).toBe(1);
        for (const [name, reference] of [
          ['Unavailable app', 'App ID 2740'],
          ['Unavailable app', 'App ID 254'],
          ['Unavailable team', 'Team ID 64120'],
          ['Unavailable team', 'Team ID 64121'],
          ['Unavailable person', 'Person ID 583231'],
          ['Unavailable person', 'Person ID 583232'],
          ['Custom repository role', 'Repository role ID 901'],
          ['Custom repository role', 'Repository role ID 9223372036854775807'],
        ]) {
          const row = card.locator('.actor-row').filter({ hasText: reference });
          expect(await row.locator('.setting-name').innerText()).toBe(name);
          expect(
            await row
              .getByRole('combobox', { name: `Bypass mode for ${name}, ${reference}` })
              .count(),
          ).toBe(1);
          expect(
            await row.getByRole('button', { name: `Remove ${name}, ${reference}` }).count(),
          ).toBe(1);
        }
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        async function assertRowLayout() {
          const rows = await card.locator('.actor-row').evaluateAll((items) =>
            items.map((row) => {
              const identity = row.querySelector('.actor-identity')!.getBoundingClientRect();
              const actions = row.querySelector('.actor-actions')!.getBoundingClientRect();
              const picker = row.querySelector('[role="combobox"]')!.getBoundingClientRect();
              const remove = row
                .querySelector('button[aria-label^="Remove "]')!
                .getBoundingClientRect();
              return {
                stacked: actions.top >= identity.bottom,
                gap: actions.top - identity.bottom,
                midpoint: (actions.top + actions.bottom - identity.top - identity.bottom) / 2,
                pickerHeight: picker.height,
                removeWidth: remove.width,
                removeHeight: remove.height,
              };
            }),
          );
          expect(new Set(rows.map((row) => row.stacked)).size).toBe(1);
          for (const row of rows) {
            expect(row.stacked).toBe(width <= 768);
            if (row.stacked) expect(row.gap).toBe(12);
            else expect(Math.abs(row.midpoint)).toBeLessThanOrEqual(1);
            expect(row.pickerHeight).toBe(34);
            expect(row.removeWidth).toBe(34);
            expect(row.removeHeight).toBe(34);
          }
        }
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.mouse.move(0, 0);
          await card.screenshot({
            path: join(directory, `unresolved-${colorScheme}-${width}.png`),
          });
        }
        const geometry = await card.evaluate((node) => {
          const bounds = node.getBoundingClientRect();
          return Array.from(node.querySelectorAll('.actor-row')).map((row) => {
            const controls = row.querySelectorAll<HTMLElement>('[role="combobox"],button');
            return Array.from(controls).every((control) => {
              const rect = control.getBoundingClientRect();
              return rect.left >= bounds.left && rect.right <= bounds.right;
            });
          });
        });
        expect(geometry.every(Boolean)).toBe(true);
        await assertRowLayout();
        expect(await page.locator('.settings-composer').count()).toBe(0);
        recovered = true;
        await card.getByRole('button', { name: 'Retry names', exact: true }).click();
        const resolved = card.getByRole('button', { name: 'Remove Renovate', exact: true });
        await resolved.waitFor();
        expect(
          await card
            .locator('.actor-row')
            .filter({ hasText: 'Renovate' })
            .locator('img')
            .getAttribute('src'),
        ).toBe(MOCK_BYPASS_ACTORS.find((actor) => actor.actor_id === 2740)!.avatar_url);
        expect(await card.getByText('App ID 2740', { exact: true }).count()).toBe(0);
        expect(await page.locator('.settings-composer').count()).toBe(0);
        expect(lookups).toBe(2);
        if (directory) await page.mouse.move(0, 0);
        if (directory)
          await card.screenshot({ path: join(directory, `recovered-${colorScheme}-${width}.png`) });
        await assertRowLayout();
        const mode = card.getByRole('combobox', {
          name: 'Bypass mode for Unavailable team, Team ID 64121',
        });
        await mode.click();
        await page.getByRole('option', { name: 'Always allow', exact: true }).click();
        await expect.poll(() => card.locator('.actor-row.is-unsaved').count()).toBe(1);
        expect(await card.locator('.actor-row.is-unsaved').innerText()).toContain('Team ID 64121');
        await assertRowLayout();
        await card
          .getByRole('button', { name: 'Remove Unavailable person, Person ID 583232' })
          .click();
        expect(await card.locator('.actor-row').count()).toBe(8);
        expect(
          await card
            .getByRole('button', { name: 'Remove Unavailable person, Person ID 583231' })
            .count(),
        ).toBe(1);
        expect(errors).toEqual([]);
      } finally {
        await page.close();
      }
    },
  );
});

describe('merge exception hierarchy [Browser]', () => {
  it.each(
    [375, 1440].flatMap((width) =>
      (['light', 'dark'] as const).flatMap((colorScheme) =>
        (['workspace/settings', 'workspace/repositories/api-gateway'] as const).map((path) => ({
          width,
          colorScheme,
          path,
        })),
      ),
    ),
  )(
    'keeps policy and actor controls coherent at $width $colorScheme $path',
    async ({ width, colorScheme, path }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1100 },
        reducedMotion: 'reduce',
      });
      try {
        await visit(page, addressOf(panel, path));
        const card = page.getByRole('region', { name: 'Merge exceptions', exact: true });
        await card.getByRole('combobox', { name: 'Bypass exception policy' }).click();
        await page.getByRole('option', { name: 'Allow listed actors', exact: true }).click();
        const add = card.getByRole('button', { name: 'Add an actor', exact: true });
        await add.waitFor();
        await add.click();
        await card.getByRole('button', { name: 'Add smyklot', exact: true }).click();
        await card.getByRole('button', { name: 'Remove smyklot', exact: true }).waitFor();
        await card.locator('img').evaluateAll(async (images) => {
          await Promise.all(
            images.map((image) => (image as HTMLImageElement).decode().catch(() => {})),
          );
        });
        expect(await card.locator('.card').count()).toBe(0);
        expect(await card.locator('.actor-header').count()).toBe(0);
        expect(
          await card.locator('.card-head').getByRole('button', { name: 'Add an actor' }).count(),
        ).toBe(1);
        await card.evaluate((node) => node.scrollIntoView({ block: 'center' }));
        await page.evaluate(() => document.fonts.ready);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.mouse.move(0, 0);
          await card.screenshot({
            path: join(
              directory,
              `exceptions-${path.includes('repositories') ? 'repository' : 'workspace'}-${colorScheme}-${width}.png`,
            ),
          });
        }
        const geometry = await card.evaluate((node) => {
          const frame = node.getBoundingClientRect();
          return Array.from(node.querySelectorAll('button,[role="combobox"]')).every((control) => {
            const bounds = control.getBoundingClientRect();
            return bounds.left >= frame.left && bounds.right <= frame.right;
          });
        });
        expect(geometry).toBe(true);
        await add.click();
        await card.getByRole('textbox', { name: 'App name or slug' }).waitFor();
        expect(await add.getAttribute('aria-expanded')).toBe('true');
        await add.click();
        expect(await add.getAttribute('aria-expanded')).toBe('false');
        expect(await card.getByRole('textbox', { name: 'App name or slug' }).count()).toBe(0);
      } finally {
        await page.close();
      }
    },
  );
});
