import type { Locator, Route } from 'playwright-core';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;

describe('desktop hours draft protection', () => {
  const fields = [
    ['Profile name', 'Release hours'],
    ['Timezone', 'UTC'],
  ] as const;

  it.each(['light', 'dark'] as const)(
    'checks searchable timezones and recovers failed previews in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(5000);
      try {
        await visit(page, addressOf(panel, 'root/schedules'));
        await page.getByRole('button', { name: 'New hours profile', exact: true }).click();
        const dialog = page.getByRole('dialog', { name: 'New hours profile', exact: true });
        await dialog.getByLabel('Profile name', { exact: true }).fill('Timezone recovery');
        for (const day of ['Tuesday', 'Wednesday', 'Thursday', 'Friday']) {
          await dialog.getByRole('button', { name: new RegExp(`Remove ${day} hours`) }).click();
        }
        const timezone = dialog.getByRole('combobox', { name: 'Timezone', exact: true });
        await timezone.fill('Mars/Olympus');
        await timezone.press('Tab');
        await dialog
          .getByText('Choose a timezone supported by the scheduler', { exact: true })
          .waitFor();
        expect(
          await dialog.getByRole('button', { name: 'Save profile', exact: true }).isDisabled(),
        ).toBe(true);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({
            path: join(directory, `F08-timezone-invalid-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        let fail = true;
        await page.route('**/api/v1/schedule-timezone?**', async (route) => {
          if (fail && new URL(route.request().url()).searchParams.get('timezone') === 'UTC') {
            fail = false;
            await route.fulfill({
              status: 503,
              contentType: 'application/json',
              body: JSON.stringify({ error: { code: 'unavailable', message: 'Unavailable' } }),
            });
          } else await route.continue();
        });
        await timezone.fill('UTC');
        await timezone.press('Tab');
        await dialog.getByRole('button', { name: 'Retry timezone check', exact: true }).waitFor();
        if (directory)
          await page.screenshot({
            path: join(directory, `F08-timezone-retry-${colorScheme}.png`),
            animations: 'disabled',
          });
        await dialog.getByRole('button', { name: 'Retry timezone check', exact: true }).click();
        await expect
          .poll(() => dialog.getByRole('button', { name: 'Save profile', exact: true }).isEnabled())
          .toBe(true);
        let releaseOld: (() => void) | undefined;
        let oldRequested = false;
        const oldGate = new Promise<void>((resolve) => {
          releaseOld = resolve;
        });
        await page.route('**/api/v1/schedule-timezone?**', async (route) => {
          if (new URL(route.request().url()).searchParams.get('timezone') !== 'Asia/Tokyo')
            return route.fallback();
          oldRequested = true;
          await oldGate;
          await route.continue();
        });
        try {
          await timezone.fill('Asia/Tokyo');
          await timezone.press('Tab');
          await expect.poll(() => oldRequested).toBe(true);
          expect(
            await dialog.getByRole('button', { name: 'Save profile', exact: true }).isDisabled(),
          ).toBe(true);
          await timezone.fill('UTC');
          await timezone.press('Tab');
          await expect
            .poll(() =>
              dialog.getByRole('button', { name: 'Save profile', exact: true }).isEnabled(),
            )
            .toBe(true);
        } finally {
          releaseOld?.();
        }
        await expect.poll(() => timezone.inputValue()).toBe('UTC');
        await timezone.fill('Warsaw');
        await page.getByRole('option', { name: 'Europe/Warsaw', exact: true }).waitFor();
        const optionEdge = await page
          .getByRole('option', { name: 'Europe/Warsaw', exact: true })
          .evaluate((node) => {
            const popup = node.closest('.select-menu')!.getBoundingClientRect();
            return popup.right - node.getBoundingClientRect().right;
          });
        expect(optionEdge).toBeLessThanOrEqual(6);
        if (directory)
          await page.screenshot({
            path: join(directory, `F08-timezone-search-${colorScheme}.png`),
            animations: 'disabled',
          });
        await timezone.press('ArrowDown');
        await timezone.press('Enter');
        await expect.poll(() => timezone.inputValue()).toBe('Europe/Warsaw');
        await expect
          .poll(() => dialog.getByRole('button', { name: 'Save profile', exact: true }).isEnabled())
          .toBe(true);
        await dialog.getByText(/^Local time:/).waitFor();
        if (directory)
          await page.screenshot({
            path: join(directory, `F08-timezone-preview-${colorScheme}.png`),
            animations: 'disabled',
          });
        await dialog.getByRole('button', { name: 'Save profile', exact: true }).click();
        await dialog.waitFor({ state: 'hidden' });
      } finally {
        await page.unrouteAll({ behavior: 'wait' });
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'previews date-specific hours and recovers errors in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      try {
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page.getByRole('button', { name: 'New hours profile' }).click();
        const editor = page.getByRole('dialog', { name: 'New hours profile', exact: true });
        await editor.getByLabel('Profile name', { exact: true }).fill('Clock changes');
        for (const day of ['Tuesday', 'Wednesday', 'Thursday', 'Friday'])
          await editor.getByRole('button', { name: new RegExp(`^Remove ${day} hours`) }).click();
        const timezone = editor.getByRole('combobox', { name: 'Timezone', exact: true });
        await timezone.fill('Europe/Warsaw');
        await timezone.press('Tab');
        const row = editor.locator('.window-row');
        await row.getByRole('combobox', { name: 'Day', exact: true }).click();
        await page.getByRole('option', { name: 'Sunday', exact: true }).click();
        await row.getByLabel('Opens', { exact: true }).fill('02:15');
        await row.getByLabel('Closes', { exact: true }).fill('02:45');
        await editor.locator('summary').filter({ hasText: 'Preview a date' }).click();
        const date = editor.getByLabel('Preview date', { exact: true });
        const section = editor.locator('details').filter({ hasText: 'Preview a date' });
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await section.scrollIntoViewIfNeeded();
          await page.screenshot({
            path: join(directory, `F08-date-preview-${scene}-${colorScheme}.png`),
            animations: 'disabled',
          });
        };
        await date.fill('2026-10-25');
        await section.getByRole('button', { name: 'Preview hours', exact: true }).click();
        await section
          .getByText('Opens 2026-10-25 at 02:15 (UTC+02:00)', { exact: false })
          .waitFor();
        expect(await section.innerText()).toContain('Closes 2026-10-25 at 02:45 (UTC+01:00)');
        await capture('repeated');
        await date.fill('2026-03-29');
        await expect
          .poll(() => section.getByText('Hours for 2026-10-25', { exact: false }).count())
          .toBe(0);
        await section.getByRole('button', { name: 'Preview hours', exact: true }).click();
        await section
          .getByText('This interval does not open because the clocks move forward.', {
            exact: false,
          })
          .waitFor();
        await capture('gap');
        await editor.getByRole('button', { name: 'Add hours', exact: true }).click();
        const additional = editor.locator('.window-row').last();
        await additional.getByRole('combobox', { name: 'Day', exact: true }).click();
        await page.getByRole('option', { name: 'Sunday', exact: true }).click();
        await additional.getByLabel('Opens', { exact: true }).fill('09:00');
        await additional.getByLabel('Closes', { exact: true }).fill('17:00');
        await section.getByRole('button', { name: 'Preview hours', exact: true }).click();
        await section
          .getByText('Opens 2026-03-29 at 09:00 (UTC+02:00)', { exact: false })
          .waitFor();
        const intervals = section.getByRole('listitem');
        expect(await intervals.count()).toBe(2);
        expect(await intervals.nth(0).innerText()).toContain('02:15 to 02:45');
        expect(await intervals.nth(0).innerText()).toContain('does not open');
        expect(await intervals.nth(1).innerText()).toContain('09:00 to 17:00');
        await capture('multiple');
        await additional.getByRole('button', { name: /^Remove Sunday hours/ }).click();
        await page.route('**/api/v1/schedule-preview', (route) =>
          route.fulfill({
            status: 503,
            contentType: 'application/json',
            body: JSON.stringify({ error: { code: 'unavailable', message: 'Unavailable' } }),
          }),
        );
        await date.fill('2026-12-27');
        await section.getByRole('button', { name: 'Preview hours', exact: true }).click();
        await section.getByRole('alert').waitFor();
        await capture('retry');
        await page.unroute('**/api/v1/schedule-preview');
        await section.getByRole('button', { name: 'Retry preview', exact: true }).click();
        await section
          .getByText('Opens 2026-12-27 at 02:15 (UTC+01:00)', { exact: false })
          .waitFor();
        await editor.getByRole('button', { name: 'Add date', exact: true }).click();
        await editor.getByLabel('Date', { exact: true }).fill('2026-12-27');
        await section.getByRole('button', { name: 'Preview hours', exact: true }).click();
        await section.getByText('Closed on this date.', { exact: true }).waitFor();
        await capture('closed');
      } finally {
        await page.unrouteAll({ behavior: 'wait' });
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'keeps workspace date previews current through cancelled reads in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(5000);
      const held: Route[] = [];
      let release: () => void = () => {};
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      try {
        await visit(page, addressOf(panel, 'workspace/settings'), { ready: '#ws-timing' });
        const timing = page.locator('#ws-timing');
        await timing.locator('summary').click();
        await timing.getByRole('button', { name: 'Request a change' }).click();
        const dialog = page.getByRole('dialog', {
          name: 'Request a change to when Smyklot acts',
          exact: true,
        });
        await dialog.getByRole('combobox', { name: 'Hours', exact: true }).click();
        await page.getByRole('option', { name: 'Hours of your own', exact: true }).click();
        for (const day of ['Tuesday', 'Wednesday', 'Thursday', 'Friday'])
          await dialog.getByRole('button', { name: new RegExp(`^Remove ${day} hours`) }).click();
        const timezone = dialog.getByRole('combobox', { name: 'Timezone', exact: true });
        await timezone.fill('UTC');
        await timezone.press('Tab');
        const row = dialog.locator('.window-row');
        await row.getByLabel('Opens', { exact: true }).fill('09:00');
        await row
          .getByRole('checkbox', { name: 'Close at end of Monday (24:00)', exact: true })
          .check();
        const section = dialog.locator('details').filter({ hasText: 'Preview a date' });
        await section.locator('summary').click();
        const date = section.getByLabel('Preview date', { exact: true });
        await date.fill('2026-12-28');
        await page.route('**/api/v1/schedule-preview', async (route) => {
          held.push(route);
          await gate;
          await route.continue();
        });
        await section.getByRole('button', { name: 'Preview hours', exact: true }).click();
        await expect.poll(() => held.length).toBe(1);
        const cancelled = page.waitForEvent('requestfailed', {
          predicate: (request) => request.url().endsWith('/api/v1/schedule-preview'),
        });
        await date.fill('2026-12-29');
        await cancelled;
        await date.fill('2026-12-28');
        await section.getByRole('button', { name: 'Preview hours', exact: true }).click();
        await expect.poll(() => held.length).toBe(2);
        expect(
          await section.getByRole('button', { name: 'Checking hours…', exact: true }).isDisabled(),
        ).toBe(true);
        release();
        await section
          .getByText('Opens 2026-12-28 at 09:00 (UTC+00:00)', { exact: false })
          .waitFor();
        expect(await section.innerText()).toContain('Closes 2026-12-29 at 00:00 (UTC+00:00)');
        expect(await section.getByRole('alert').count()).toBe(0);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await section.scrollIntoViewIfNeeded();
          await page.screenshot({
            path: join(directory, `F08-workspace-date-preview-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await row.getByLabel('Opens', { exact: true }).fill('10:00');
        await expect.poll(() => section.getByText('Hours for', { exact: false }).count()).toBe(0);
        await section.getByRole('button', { name: 'Preview hours', exact: true }).click();
        await section
          .getByText('Opens 2026-12-28 at 10:00 (UTC+00:00)', { exact: false })
          .waitFor();
      } finally {
        release();
        await page.unrouteAll({ behavior: 'wait' });
        await page.close();
      }
    },
  );

  it('previews unsaved schedule dates without changing mock profiles', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    try {
      const profiles = addressOf(panel, 'api/v1/root/schedule-profiles');
      const before = await (await page.request.get(profiles)).json();
      const input = {
        date: '2026-10-25',
        profile: {
          name: 'Unsaved',
          timezone: 'Europe/Warsaw',
          windows: [],
          exceptions: [{ date: '2026-10-25', closed: false, start_minute: 135, end_minute: 165 }],
        },
      };
      const endpoint = addressOf(panel, 'api/v1/schedule-preview');
      const response = await page.request.post(endpoint, { data: input });
      expect(response.status()).toBe(200);
      expect((await response.json()).windows).toEqual([
        {
          start_minute: 135,
          end_minute: 165,
          available: true,
          opens_at: '2026-10-25T02:15:00+02:00',
          closes_at: '2026-10-25T02:45:00+01:00',
        },
      ]);
      expect(
        (await page.request.post(endpoint, { data: { ...input, date: '2026-02-30' } })).status(),
      ).toBe(400);
      expect(await (await page.request.get(profiles)).json()).toEqual(before);
    } finally {
      await page.close();
    }
  });

  it('previews schedule timezones with the authoritative Go database', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    try {
      const endpoint = addressOf(panel, 'api/v1/schedule-timezone');
      const preview = await page.request.get(endpoint, {
        params: { timezone: 'Europe/Warsaw', at: '2026-03-29T01:00:00Z' },
      });
      expect(preview.status()).toBe(200);
      expect(await preview.json()).toMatchObject({
        local_time: '2026-03-29T03:00:00+02:00',
        offset_seconds: 7200,
      });
      const invalid = await page.request.get(endpoint, {
        params: { timezone: 'Mars/Olympus', at: '2026-03-29T01:00:00Z' },
      });
      expect(invalid.status()).toBe(400);
      expect((await invalid.json()).error.code).toBe('invalid_timezone');
    } finally {
      await page.close();
    }
  });

  it('rejects invalid calendar rules without changing mock profiles', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    try {
      const url = addressOf(panel, 'api/v1/root/schedule-profiles');
      const before = await (await page.request.get(url)).json();
      const base = {
        name: 'Calendar validation',
        timezone: 'UTC',
        expected_revision: 0,
        windows: [],
        exceptions: [],
      };
      for (const rules of [
        { timezone: 'Mars/Olympus', windows: [{ weekday: 1, start_minute: 0, end_minute: 1440 }] },
        { exceptions: [{ date: '2026-02-30', closed: true }] },
        { windows: [{ weekday: 1, start_minute: 600, end_minute: 500 }] },
        {
          windows: [
            { weekday: 1, start_minute: 0, end_minute: 1440 },
            { weekday: 1, start_minute: 600, end_minute: 700 },
          ],
        },
        {
          exceptions: [
            { date: '2026-12-25', closed: true },
            { date: '2026-12-25', closed: false, start_minute: 600, end_minute: 700 },
          ],
        },
      ]) {
        const response = await page.request.post(url, { data: { ...base, ...rules } });
        expect(response.status()).toBe(400);
        expect((await response.json()).error.code).toBe('invalid_schedule');
      }
      expect(await (await page.request.get(url)).json()).toEqual(before);
      const valid = await page.request.post(url, {
        data: {
          ...base,
          exceptions: [{ date: '2028-02-29', closed: false, start_minute: 0, end_minute: 1440 }],
        },
      });
      expect(valid.status()).toBe(201);
      const saved = await valid.json();
      expect(saved.windows).toEqual([]);
      expect(saved.exceptions).toEqual([
        { date: '2028-02-29', closed: false, start_minute: 0, end_minute: 1440 },
      ]);
    } finally {
      await page.close();
    }
  });

  it.each(['light', 'dark'] as const)(
    'edits structured exception modes without losing hours in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      try {
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page.getByRole('button', { name: 'New hours profile' }).click();
        const editor = page.getByRole('dialog', { name: 'New hours profile', exact: true });
        await editor
          .getByLabel('Profile name', { exact: true })
          .fill(`Structured hours ${colorScheme}`);
        for (const day of ['Tuesday', 'Wednesday', 'Thursday', 'Friday']) {
          await editor.getByRole('button', { name: new RegExp(`^Remove ${day} hours`) }).click();
        }
        await editor.getByRole('button', { name: 'Add date', exact: true }).click();
        const first = editor.getByRole('group', { name: 'Date exception 1', exact: true });
        await first.getByLabel('Date', { exact: true }).fill('2026-12-25');
        const mode = first.getByRole('combobox', { name: 'Hours', exact: true });
        await mode.click();
        await page.getByRole('option', { name: 'Custom hours', exact: true }).click();
        const opens = first.getByLabel('Opens', { exact: true });
        const closes = first.getByLabel('Closes', { exact: true });
        await closes.fill('08:00');
        await closes.press('Tab');
        await first.getByRole('alert').waitFor();
        expect(
          await first.getByLabel('Date', { exact: true }).getAttribute('aria-invalid'),
        ).toBeNull();
        expect(await mode.getAttribute('aria-invalid')).toBeNull();
        expect(await opens.getAttribute('aria-invalid')).toBe('true');
        expect(await closes.getAttribute('aria-invalid')).toBe('true');
        const problemId = await closes.getAttribute('aria-describedby');
        expect(await first.locator(`[id="${problemId}"]`).innerText()).toContain('Closing time');
        const auditDirectory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (auditDirectory) {
          await mkdir(auditDirectory, { recursive: true });
          await page.screenshot({
            path: join(auditDirectory, `F08-exception-range-targets-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await closes.fill('13:00');
        await closes.press('Tab');
        await expect.poll(() => first.getByRole('alert').count()).toBe(0);
        expect(await opens.getAttribute('aria-invalid')).toBeNull();
        expect(await closes.getAttribute('aria-invalid')).toBeNull();
        await mode.click();
        await page.getByRole('option', { name: 'Closed all day', exact: true }).click();
        await mode.click();
        await page.getByRole('option', { name: 'Custom hours', exact: true }).click();
        expect(await first.getByLabel('Closes', { exact: true }).inputValue()).toBe('13:00');
        const endOfDay = first.getByRole('checkbox', {
          name: 'Close at end of day (24:00)',
          exact: true,
        });
        await endOfDay.check();
        await endOfDay.uncheck();
        expect(await first.getByLabel('Closes', { exact: true }).inputValue()).toBe('13:00');
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await first.scrollIntoViewIfNeeded();
          await page.screenshot({
            path: join(directory, `F08-structured-custom-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await editor.getByRole('button', { name: 'Add date', exact: true }).click();
        const second = editor.getByRole('group', { name: 'Date exception 2', exact: true });
        await second.getByLabel('Date', { exact: true }).fill('2026-12-25');
        await editor.getByRole('button', { name: 'Save profile' }).click();
        await expect.poll(() => editor.getByRole('alert').count()).toBe(2);
        expect(await mode.getAttribute('aria-invalid')).toBe('true');
        expect(
          await second
            .getByRole('combobox', { name: 'Hours', exact: true })
            .getAttribute('aria-invalid'),
        ).toBe('true');
        if (directory) {
          await second.scrollIntoViewIfNeeded();
          await page.screenshot({
            path: join(directory, `F08-structured-conflict-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await second.getByRole('button', { name: 'Remove date exception 2', exact: true }).click();
        await expect.poll(() => editor.getByRole('alert').count()).toBe(0);
        await editor.getByRole('button', { name: 'Save profile' }).click();
        await editor.waitFor({ state: 'hidden' });
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'keeps workspace schedule errors inside the request dialog in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      page.setDefaultTimeout(5000);
      const submitted: Array<{ custom_profile: { windows: unknown[]; exceptions: unknown[] } }> =
        [];
      try {
        await page.route('**/api/v1/targets/*/schedule-requests', async (route) => {
          if (route.request().method() === 'POST') submitted.push(route.request().postDataJSON());
          await route.continue();
        });
        await visit(page, addressOf(panel, 'workspace/settings'), { ready: '#ws-timing' });
        const timing = page.locator('#ws-timing');
        await timing.locator('summary').click();
        await timing.getByRole('button', { name: 'Request a change' }).click();
        const dialog = page.getByRole('dialog', {
          name: 'Request a change to when Smyklot acts',
          exact: true,
        });
        await dialog.getByRole('combobox', { name: 'Hours', exact: true }).click();
        await page.getByRole('option', { name: 'Hours of your own', exact: true }).click();
        for (const day of ['Tuesday', 'Wednesday', 'Thursday', 'Friday']) {
          await dialog.evaluate(async (node) => {
            await Promise.allSettled(
              node.getAnimations({ subtree: true }).map((animation) => animation.finished),
            );
          });
          const count = await dialog.locator('.window-row').count();
          await dialog.getByRole('button', { name: new RegExp(`^Remove ${day} hours`) }).click();
          await expect.poll(() => dialog.locator('.window-row').count()).toBe(count - 1);
        }
        const reason = `Keep release work within the agreed hours (${colorScheme})`;
        await dialog.getByLabel('Reason', { exact: true }).fill(reason);
        await dialog.getByRole('button', { name: 'Add date', exact: true }).click();
        const exceptions = dialog.getByLabel('Date', { exact: true });
        await dialog.getByRole('button', { name: 'Send request', exact: true }).click();
        const error = dialog.getByRole('alert').filter({ hasText: 'Choose a valid calendar date' });
        await error.waitFor();
        expect(submitted).toHaveLength(0);
        expect(await exceptions.inputValue()).toBe('');
        await expect
          .poll(() => exceptions.evaluate((node) => node === document.activeElement))
          .toBe(true);
        await error.scrollIntoViewIfNeeded();
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({
            path: join(directory, `F08-workspace-date-error-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await exceptions.fill('2026-12-25');
        await dialog.getByLabel('Opens', { exact: true }).fill('18:00');
        await dialog.getByRole('button', { name: 'Send request', exact: true }).click();
        expect(submitted).toHaveLength(0);
        await expect
          .poll(() =>
            dialog
              .getByLabel('Opens', { exact: true })
              .evaluate((node) => node === document.activeElement),
          )
          .toBe(true);
        await dialog.locator('.window-problem').scrollIntoViewIfNeeded();
        if (directory)
          await page.screenshot({
            path: join(directory, `F08-workspace-range-error-${colorScheme}.png`),
            animations: 'disabled',
          });
        await dialog.getByRole('checkbox', { name: 'Close at end of Monday (24:00)' }).check();
        await dialog.getByRole('button', { name: 'Send request', exact: true }).click();
        await dialog.waitFor({ state: 'hidden' });
        expect(submitted).toHaveLength(1);
        expect(submitted[0]?.custom_profile.windows).toEqual([
          { weekday: 1, start_minute: 1080, end_minute: 1440 },
        ]);
        expect(submitted[0]?.custom_profile.exceptions).toEqual([
          { date: '2026-12-25', closed: true },
        ]);
        await timing.getByText(reason, { exact: false }).waitFor();
      } finally {
        await page.unrouteAll({ behavior: 'wait' });
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'preserves end-of-day closing times in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      try {
        await page.route('**/api/v1/root/schedule-profiles', async (route) => {
          const response = await route.fetch();
          const document = await response.json();
          const profile = document.profiles.find((entry: { system: boolean }) => !entry.system);
          profile.name = 'Full Monday';
          profile.windows = [{ weekday: 1, start_minute: 0, end_minute: 1440 }];
          profile.exceptions = [];
          await route.fulfill({ response, json: document });
        });
        let submitted: { windows: unknown[] } | undefined;
        await page.route('**/api/v1/root/schedule-profiles/*', async (route) => {
          if (route.request().method() === 'PUT') submitted = route.request().postDataJSON();
          await route.continue();
        });
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page
          .getByRole('button', { name: 'Edit - the Full Monday profile', exact: true })
          .click();
        const editor = page.getByRole('dialog', { name: 'Edit hours profile', exact: true });
        const endOfDay = editor.getByRole('checkbox', { name: 'Close at end of Monday (24:00)' });
        expect(await endOfDay.isChecked()).toBe(true);
        expect(await editor.getByLabel('Closes', { exact: true }).inputValue()).toBe('End of day');
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({
            path: join(directory, `F08-end-of-day-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await endOfDay.uncheck();
        await editor.getByLabel('Closes', { exact: true }).fill('17:00');
        await endOfDay.check();
        await endOfDay.uncheck();
        expect(await editor.getByLabel('Closes', { exact: true }).inputValue()).toBe('17:00');
        await endOfDay.check();
        await editor.getByLabel('Profile name', { exact: true }).fill('Full Monday revised');
        await editor.getByRole('button', { name: 'Save profile' }).click();
        await editor.waitFor({ state: 'hidden' });
        expect(submitted?.windows).toEqual([{ weekday: 1, start_minute: 0, end_minute: 1440 }]);
      } finally {
        await page.unrouteAll({ behavior: 'wait' });
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'explains invalid weekly rows and their recovery in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      try {
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page.getByRole('button', { name: 'New hours profile' }).click();
        const editor = page.getByRole('dialog', { name: 'New hours profile', exact: true });
        await editor.getByLabel('Profile name', { exact: true }).fill('Weekly validation');
        for (const day of ['Tuesday', 'Wednesday', 'Thursday', 'Friday']) {
          const count = await editor.locator('.window-row').count();
          await editor.getByRole('button', { name: new RegExp(`^Remove ${day} hours`) }).click();
          await expect.poll(() => editor.locator('.window-row').count()).toBe(count - 1);
        }
        const first = editor.locator('.window-row').first();
        const opens = first.getByLabel('Opens', { exact: true });
        await opens.fill('18:00');
        await first.getByRole('alert').waitFor();
        expect(await opens.getAttribute('aria-invalid')).toBe('true');
        expect(
          await first
            .getByRole('combobox', { name: 'Day', exact: true })
            .getAttribute('aria-invalid'),
        ).toBeNull();
        const errorId = await opens.getAttribute('aria-describedby');
        expect(await editor.locator(`[id="${errorId}"]`).innerText()).toContain(
          'Closing time must be after opening time',
        );
        expect(await editor.getByRole('button', { name: 'Save profile' }).isDisabled()).toBe(true);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({
            path: join(directory, `F08-weekly-range-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await opens.fill('09:00');
        await expect.poll(() => first.getByRole('alert').count()).toBe(0);
        await editor.getByRole('button', { name: 'Add hours', exact: true }).click();
        await expect.poll(() => editor.locator('.window-problem').count()).toBe(2);
        expect(await editor.getByRole('button', { name: 'Save profile' }).isDisabled()).toBe(true);
        if (directory)
          await page.screenshot({
            path: join(directory, `F08-weekly-overlap-${colorScheme}.png`),
            animations: 'disabled',
          });
        const second = editor.locator('.window-row').last();
        await second.getByLabel('Closes', { exact: true }).fill('18:00');
        await second.getByLabel('Opens', { exact: true }).fill('17:00');
        await expect.poll(() => editor.locator('.window-problem').count()).toBe(0);
        expect(await opens.getAttribute('aria-invalid')).toBeNull();
        await editor.getByRole('button', { name: 'Save profile' }).click();
        await editor.waitFor({ state: 'hidden' });
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'preserves unfinished exception rows for correction in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      try {
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page.getByRole('button', { name: 'New hours profile' }).click();
        const editor = page.getByRole('dialog', { name: 'New hours profile', exact: true });
        await editor.getByLabel('Profile name', { exact: true }).fill('Strict exception input');
        await editor.getByRole('button', { name: 'Add date', exact: true }).click();
        const field = editor.getByLabel('Date', { exact: true });
        await editor.getByRole('button', { name: 'Save profile' }).click();
        const dateError = editor
          .getByRole('alert')
          .filter({ hasText: 'Choose a valid calendar date' });
        await dateError.waitFor();
        expect(await field.inputValue()).toBe('');
        await expect
          .poll(() => field.evaluate((node) => node === document.activeElement))
          .toBe(true);
        await page.keyboard.press('Escape');
        const guard = page.getByRole('dialog', { name: 'Discard hours changes?', exact: true });
        await guard.getByRole('button', { name: 'Keep editing', exact: true }).click();
        expect(await field.count()).toBe(1);
        await dateError.scrollIntoViewIfNeeded();
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({
            path: join(directory, `F08-structured-date-error-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await field.fill('2026-12-25');
        await editor.getByRole('button', { name: 'Save profile' }).click();
        await editor.waitFor({ state: 'hidden' });
      } finally {
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'preserves an exception-only profile in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      page.setDefaultTimeout(5000);
      const exceptions = [
        { date: '2026-12-25', closed: true },
        { date: '2026-12-31', closed: false, start_minute: 540, end_minute: 780 },
      ];
      let submitted: { windows: unknown[]; exceptions: unknown[] } | undefined;
      try {
        await page.route('**/api/v1/root/schedule-profiles', async (route) => {
          const response = await route.fetch();
          const document = (await response.json()) as {
            profiles: Array<{
              system: boolean;
              name: string;
              windows: unknown[];
              exceptions: unknown[];
            }>;
          };
          const profile = document.profiles.find((entry) => !entry.system)!;
          profile.name = 'Holiday releases';
          profile.windows = [];
          profile.exceptions = exceptions;
          await route.fulfill({ response, json: document });
        });
        await page.route('**/api/v1/root/schedule-profiles/*', async (route) => {
          if (route.request().method() !== 'PUT') return route.continue();
          submitted = route.request().postDataJSON() as typeof submitted;
          await route.continue();
        });
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page
          .getByRole('button', { name: 'Edit - the Holiday releases profile', exact: true })
          .click();
        const editor = page.getByRole('dialog', { name: 'Edit hours profile', exact: true });
        expect(await editor.locator('.window-row').count()).toBe(0);
        expect(
          await editor
            .getByLabel('Date', { exact: true })
            .evaluateAll((nodes) => nodes.map((node) => (node as HTMLInputElement).value)),
        ).toEqual(['2026-12-25', '2026-12-31']);
        const custom = editor.getByRole('group', { name: 'Date exception 2', exact: true });
        expect(await custom.getByLabel('Opens', { exact: true }).inputValue()).toBe('09:00');
        expect(await custom.getByLabel('Closes', { exact: true }).inputValue()).toBe('13:00');
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({
            path: join(directory, `F08-exception-only-${colorScheme}.png`),
            animations: 'disabled',
          });
        }
        await editor.getByLabel('Profile name', { exact: true }).fill('Holiday release hours');
        await editor.getByRole('button', { name: 'Save profile' }).click();
        await editor.waitFor({ state: 'hidden' });
        expect(submitted?.windows).toEqual([]);
        expect(submitted?.exceptions).toEqual(exceptions);
      } finally {
        await page.close();
      }
    },
  );

  it('keeps an existing profile mounted during save and retains edits after rejection', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    page.setDefaultTimeout(5000);
    let release = (): void => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    let submitted = false;
    try {
      await page.route('**/api/v1/root/schedule-profiles/*', async (route) => {
        if (route.request().method() !== 'PUT') return route.continue();
        submitted = true;
        await held;
        await route.fulfill({
          status: 503,
          json: { error: { code: 'unavailable', message: 'Try saving again' } },
        });
      });
      await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
      const edit = page.getByRole('button', { name: /^Edit - the .+ profile$/ }).first();
      await edit.click();
      const editor = page.getByRole('dialog', { name: 'Edit hours profile', exact: true });
      const name = editor.getByLabel('Profile name', { exact: true });
      const original = await name.inputValue();
      await name.fill('Temporary changed name');
      await name.fill(original);
      await page.keyboard.press('Escape');
      await editor.waitFor({ state: 'hidden' });
      await edit.click();
      await name.fill('Release working hours');
      await editor.getByRole('button', { name: 'Save profile' }).click();
      await expect.poll(() => submitted).toBe(true);
      await page.keyboard.press('Escape');
      await editor.getByRole('button', { name: 'Cancel', exact: true }).click();
      expect(await editor.isVisible()).toBe(true);
      expect(
        await page.getByRole('dialog', { name: 'Discard hours changes?', exact: true }).count(),
      ).toBe(0);
      release();
      await editor.getByText('Try saving again', { exact: true }).waitFor();
      await expect.poll(() => name.inputValue()).toBe('Release working hours');
      await page.keyboard.press('Escape');
      const guard = page.getByRole('dialog', { name: 'Discard hours changes?', exact: true });
      await guard.getByRole('button', { name: 'Discard changes' }).click();
      await editor.waitFor({ state: 'hidden' });
    } finally {
      release();
      await page.close();
    }
  });

  it.each(fields)(
    'retains changed %s through incidental and explicit dismissal',
    async (label, value) => {
      const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
      page.setDefaultTimeout(5000);
      try {
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page.getByRole('button', { name: 'New hours profile' }).click();
        const editor = page.getByRole('dialog', { name: 'New hours profile', exact: true });
        const field = editor.getByLabel(label, { exact: true });
        await field.fill(value);
        await page.keyboard.press('Escape');
        const guard = page.getByRole('dialog', { name: 'Discard hours changes?', exact: true });
        await guard.waitFor();
        await guard.getByRole('button', { name: 'Keep editing' }).click();
        await expect.poll(() => field.inputValue()).toBe(value);
        await expect
          .poll(() => field.evaluate((node) => node === document.activeElement))
          .toBe(true);
        await editor.getByRole('button', { name: 'Cancel', exact: true }).click();
        await guard.waitFor();
        await guard.getByRole('button', { name: 'Discard changes' }).click();
        await editor.waitFor({ state: 'hidden' });
        await expect
          .poll(() =>
            page
              .getByRole('button', { name: 'New hours profile' })
              .evaluate((node) => node === document.activeElement),
          )
          .toBe(true);
      } finally {
        await page.close();
      }
    },
  );

  it('protects weekly windows on outside dismissal and closes restored drafts without a guard', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    page.setDefaultTimeout(5000);
    try {
      await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
      await page.getByRole('button', { name: 'New hours profile' }).click();
      const editor = page.getByRole('dialog', { name: 'New hours profile', exact: true });
      const start = editor.locator('.window-row input[type=time]').first();
      const original = await start.inputValue();
      await start.fill('10:00');
      await page.mouse.click(400, 300);
      const guard = page.getByRole('dialog', { name: 'Discard hours changes?', exact: true });
      await guard.waitFor();
      await page.keyboard.press('Escape');
      await guard.waitFor({ state: 'hidden' });
      await expect.poll(() => start.inputValue()).toBe('10:00');
      await start.fill(original);
      await page.keyboard.press('Escape');
      await editor.waitFor({ state: 'hidden' });
      expect(await guard.count()).toBe(0);
    } finally {
      await page.close();
    }
  });
});

async function expectSharedModalControls(dialog: Locator): Promise<void> {
  const geometry = await dialog.evaluate((node) => {
    const durations = Array.from(node.querySelectorAll('.duration-field')).map((field) => {
      const box = field.getBoundingClientRect();
      const controls = field.querySelector('.duration-controls')!.getBoundingClientRect();
      const parent = field.closest('.form-field')!;
      const label = parent.querySelector('.form-label')!;
      const labelBox = label.getBoundingClientRect();
      const input = field.querySelector('input')!;
      const select = field.querySelector('[role=combobox]')!;
      const reference = document.createElement('input');
      reference.className = 'text-input';
      parent.append(reference);
      const paint = (element: Element) => {
        const style = getComputedStyle(element);
        return [
          style.backgroundColor,
          style.borderTopColor,
          style.borderTopWidth,
          style.borderRadius,
          style.fontSize,
          style.height,
        ];
      };
      const result = {
        width: box.width,
        controlsWidth: controls.width,
        x: controls.x,
        labelX: labelBox.x,
        gap: controls.y - labelBox.bottom,
        labelSize: getComputedStyle(label).fontSize,
        expectedLabelSize: getComputedStyle(reference).fontSize,
        labelWeight: getComputedStyle(label).fontWeight,
        input: paint(input),
        reference: paint(reference),
        inputHeight: input.getBoundingClientRect().height,
        selectHeight: select.getBoundingClientRect().height,
      };
      reference.remove();
      return result;
    });
    return {
      durations,
      nativeChecks: Array.from(node.querySelectorAll('input[type="checkbox"]')).filter(
        (input) => !input.closest('.switch,.check-item'),
      ).length,
      nativeSelects: node.querySelectorAll('select').length,
      overflow: node.scrollWidth - node.clientWidth,
    };
  });
  expect(geometry.durations.length).toBeGreaterThan(0);
  for (const field of geometry.durations) {
    expect(field.width).toBeCloseTo(field.controlsWidth, 1);
    expect(field.x).toBeCloseTo(field.labelX, 1);
    expect(field.gap).toBeCloseTo(8, 1);
    expect(field.labelSize).toBe(field.expectedLabelSize);
    expect(field.labelWeight).toBe('600');
    expect(field.input).toEqual(field.reference);
    expect(field.inputHeight).toBe(34);
    expect(field.selectHeight).toBe(34);
  }
  expect(geometry.nativeChecks).toBe(0);
  expect(geometry.nativeSelects).toBe(0);
  expect(geometry.overflow).toBeLessThanOrEqual(1);
}

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

describe('background work schedules [Integration]', () => {
  it.each([
    { colorScheme: 'light', width: 1440 },
    { colorScheme: 'dark', width: 1440 },
    { colorScheme: 'light', width: 375 },
    { colorScheme: 'dark', width: 375 },
  ] as const)(
    'recognizes configuration file sync in service and workspace timing at $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({ colorScheme, viewport: { width, height: 1000 } });
      page.setDefaultTimeout(10_000);
      try {
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page.getByRole('button', { name: /Show all \d+ jobs/ }).click();
        const job = page.locator('.object-row', {
          has: page.getByText('Configuration file sync', { exact: true }),
        });
        await job.getByText(/every 15 minutes around the clock/).waitFor();
        await job
          .getByRole('button', { name: 'Edit schedule - Configuration file sync', exact: true })
          .click();
        const editor = page.getByRole('dialog', { name: 'Configure job', exact: true });
        await editor.waitFor();
        expect(
          await editor.getByRole('textbox', { name: 'How often', exact: true }).inputValue(),
        ).toBe('15');
        await editor.getByText(/Checks connected configuration files for changes/).waitFor();
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await editor.screenshot({
            path: join(directory, `service-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        }
        await expectSharedModalControls(editor);
        const overflow = await editor.evaluate(
          (element) => element.scrollWidth - element.clientWidth,
        );
        expect(overflow).toBeLessThanOrEqual(1);
        await page.keyboard.press('Escape');

        let submitted: Record<string, unknown> | undefined;
        await page.route('**/api/v1/targets/*/schedule-requests', async (route) => {
          if (route.request().method() !== 'POST') {
            await route.continue();
            return;
          }
          submitted = route.request().postDataJSON() as Record<string, unknown>;
          await route.fulfill({ status: 200, json: {} });
        });
        await visit(page, addressOf(panel, 'workspace/settings'), { ready: '#ws-timing' });
        const timing = page.locator('#ws-timing');
        await timing.locator('summary').click();
        await timing.getByRole('button', { name: 'Request a change' }).click();
        const request = page.getByRole('dialog', { name: 'Request a change to when Smyklot acts' });
        await request.getByLabel('Job', { exact: true }).click();
        await page.getByRole('option', { name: 'Configuration file sync', exact: true }).click();
        await request.getByText(/Checks connected configuration files for changes/).waitFor();
        expect(
          await request.getByRole('textbox', { name: 'How often', exact: true }).inputValue(),
        ).toBe('15');
        await request.getByRole('textbox', { name: 'How often', exact: true }).fill('20');
        await request.getByLabel('Reason').fill('Check missed file changes every twenty minutes');
        if (directory)
          await request.screenshot({
            path: join(directory, `workspace-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        expect(
          await request.evaluate((element) => element.scrollWidth - element.clientWidth),
        ).toBeLessThanOrEqual(1);
        await expectSharedModalControls(request);
        await request.getByRole('button', { name: 'Send request' }).click();
        await expect
          .poll(() => submitted)
          .toEqual(
            expect.objectContaining({
              kind: 'config_file_sync',
              cadence_seconds: 1200,
              base_revision: 1,
              default_priority: 'normal',
            }),
          );
      } finally {
        await page.close();
      }
    },
  );

  it.each(
    [375, 768, 1024, 1440].flatMap((width) =>
      (['light', 'dark'] as const).map((colorScheme) => ({ width, colorScheme })),
    ),
  )('keeps hours profile forms coherent at $colorScheme $width', async ({ width, colorScheme }) => {
    const page = await panel.browser.newPage({ colorScheme, viewport: { width, height: 1000 } });
    page.setDefaultTimeout(10_000);
    try {
      await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
      await page.getByRole('button', { name: 'New hours profile', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'New hours profile', exact: true });
      await dialog.waitFor();
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (directory) {
        await mkdir(directory, { recursive: true });
        await dialog.screenshot({
          path: join(directory, `profile-new-${colorScheme}-${width}.png`),
          animations: 'disabled',
        });
      }
      const geometry = await dialog.evaluate((node) => {
        const name = node.querySelector<HTMLInputElement>('#profile-name')!;
        const timezone = node.querySelector<HTMLInputElement>('#profile-timezone')!;
        const controls = Array.from(
          node.querySelectorAll('.window-row input, .window-row [role=combobox]'),
        );
        return {
          nameHeight: name.getBoundingClientRect().height,
          timezoneHeight: timezone.getBoundingClientRect().height,
          controlHeights: controls.map((control) => control.getBoundingClientRect().height),
          removeButtons: Array.from(node.querySelectorAll('.window-remove button')).map(
            (button) => ({
              width: button.getBoundingClientRect().width,
              height: button.getBoundingClientRect().height,
              glyphs: button.querySelectorAll('svg path').length,
            }),
          ),
          overflow: node.scrollWidth - node.clientWidth,
        };
      });
      expect.soft(geometry.nameHeight).toBe(34);
      expect.soft(geometry.timezoneHeight).toBe(34);
      expect.soft(geometry.controlHeights.every((height) => height === 34)).toBe(true);
      expect(geometry.removeButtons).toHaveLength(5);
      for (const button of geometry.removeButtons) {
        expect(button.width).toBe(34);
        expect(button.height).toBe(34);
        expect(button.glyphs).toBeGreaterThan(0);
      }
      expect(geometry.overflow).toBeLessThanOrEqual(1);
      if (width === 375) {
        const exceptions = dialog.getByRole('group', { name: 'Date exceptions', exact: true });
        await exceptions.scrollIntoViewIfNeeded();
        if (directory)
          await dialog.screenshot({
            path: join(directory, `profile-new-end-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        const control = await exceptions.boundingBox();
        const save = await dialog.getByRole('button', { name: 'Save profile' }).boundingBox();
        expect(control!.y + control!.height).toBeLessThan(save!.y);
      }
      await page.keyboard.press('Escape');
      await page
        .getByRole('button', { name: /^Edit - the .+ profile$/ })
        .first()
        .click();
      const edit = page.getByRole('dialog', { name: 'Edit hours profile', exact: true });
      await edit.waitFor();
      if (directory)
        await edit.screenshot({
          path: join(directory, `profile-edit-${colorScheme}-${width}.png`),
          animations: 'disabled',
        });
      expect(
        await edit.evaluate((node) => node.scrollWidth - node.clientWidth),
      ).toBeLessThanOrEqual(1);
      if (width === 375) {
        await edit
          .getByRole('group', { name: 'Date exceptions', exact: true })
          .scrollIntoViewIfNeeded();
        if (directory)
          await edit.screenshot({
            path: join(directory, `profile-edit-end-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
      }
      await edit.getByRole('button', { name: /^Remove Friday hours/ }).click();
      expect(await edit.locator('.window-row').count()).toBe(4);
      await edit.getByRole('button', { name: 'Add hours', exact: true }).click();
      expect(await edit.locator('.window-row').count()).toBe(5);
    } finally {
      await page.close();
    }
  });

  it('announces the initial schedule load until every response arrives', async () => {
    const page = await panel.browser.newPage();
    let releaseResponse = (): void => {};
    const heldResponse = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });
    try {
      await page.route('**/api/v1/root/job-policies', async (route) => {
        await heldResponse;
        await route.continue();
      });
      await page.goto(addressOf(panel, 'root/schedules'), { waitUntil: 'domcontentloaded' });

      const view = page.locator('.view-frame[aria-busy]');
      await view.waitFor();
      await expect.poll(() => view.getAttribute('aria-busy')).toBe('true');

      releaseResponse();
      await expect.poll(() => view.getAttribute('aria-busy')).toBe('false');
      await view.locator('.object-row').first().waitFor();
    } finally {
      releaseResponse();
      await page.close();
    }
  });

  it('renders empty policy overrides from older servers', async () => {
    const page = await panel.browser.newPage();
    const emptyOverrides = async (route: Route) => {
      const response = await route.fetch();
      const document = (await response.json()) as {
        policy_set?: { overrides?: unknown };
        policies?: { overrides?: unknown };
      };
      if (document.policy_set !== undefined) document.policy_set.overrides = null;
      if (document.policies !== undefined) document.policies.overrides = null;
      await route.fulfill({ response, json: document });
    };
    try {
      await page.route('**/api/v1/root/job-policies', emptyOverrides);

      await visit(page, addressOf(panel, 'root/schedules'), {
        ready: '.view-frame .object-row',
      });
      await page.getByRole('heading', { name: 'Schedules', level: 1 }).waitFor();
    } finally {
      await page.close();
    }
  });

  /**
   * A job is a sentence, and the page opens on the four that ran most recently rather
   * than on all twelve: a console opens on what is happening. What each card owes a
   * reader is checked by its words, because that is the whole of what changed - a
   * cadence is said in human units and the hours are said as a week.
   */
  it('says what every job does, how often, and in whose hours', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });

      await page.getByRole('heading', { name: 'Schedules', level: 1 }).waitFor();

      const jobs = page.locator('.card', { has: page.getByRole('heading', { name: 'Jobs' }) });
      await expect.poll(() => jobs.locator('.object-row').count()).toBe(4);
      await jobs.getByText('Showing 4 of 12 jobs', { exact: false }).waitFor();
      // The cadence in words, and the hours the job runs in - never 21600 seconds.
      await jobs
        .getByText(/every 5 minutes around the clock/)
        .first()
        .waitFor();

      await jobs.getByRole('button', { name: 'Show all 12 jobs' }).click();
      await expect.poll(() => jobs.locator('.object-row').count()).toBe(12);

      const hours = page.locator('.card', { has: page.getByRole('heading', { name: 'Hours' }) });
      await hours.getByText('Always Open', { exact: true }).waitFor();
      await hours.getByText(/Europe\/Warsaw · Mon to Fri/).waitFor();

      // A request waiting on somebody leads the page, worded as the ask it is.
      const decide = page.locator('.card', {
        has: page.getByRole('heading', { name: 'Needs a decision' }),
      });
      await decide.getByText(/asks: File indexing every 30 minutes/).waitFor();
      await decide
        .getByText('Refresh which paths are watched during the release preparation window', {
          exact: false,
        })
        .waitFor();
      await decide.getByRole('button', { name: 'Approve' }).waitFor();
      await decide.getByRole('button', { name: 'Decline' }).waitFor();
    } finally {
      await page.close();
    }
  });

  /**
   * A workspace has no Schedules page any more: timing is the service's to set, so what a
   * workspace gets is one row on its settings page saying when Smyklot acts and the way to
   * ask for that to change. This walks the row, opens the ask and sends it.
   */
  it('gives a workspace when Smyklot acts, and the way to ask for a change', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, addressOf(panel, 'workspace/settings'), { ready: '#ws-timing' });

      const timing = page.locator('#ws-timing');
      await timing.locator('summary').click();
      await timing.getByText('When Smyklot acts', { exact: true }).waitFor();
      // Read from the windows, not from a name: the fixture's policies name two profiles.
      await timing.locator('.setting-fact').waitFor();

      const dialog = page.locator('#workspace-timing-request');
      await timing.getByRole('button', { name: 'Request a change' }).click();
      await dialog.waitFor({ state: 'visible' });

      const send = dialog.getByRole('button', { name: 'Send request' });
      await expect.poll(() => send.isDisabled()).toBe(true);
      await dialog.getByLabel('Reason').fill('Keep the sync inside the release window');
      await expect.poll(() => send.isEnabled()).toBe(true);

      await send.click();
      await timing.getByText('A change to', { exact: false }).first().waitFor();
      await page.getByText('Sent to the operators for a decision').waitFor();
    } finally {
      await page.close();
    }
  });

  it('keeps every schedule row inside a phone viewport', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 390, height: 844 } });
    try {
      await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });

      const overflow = await page.evaluate(() => {
        const rows = [...document.querySelectorAll<HTMLElement>('.object-row')];
        if (rows.length === 0) return Number.POSITIVE_INFINITY;
        const width = document.documentElement.clientWidth;

        return Math.max(...rows.map((row) => row.getBoundingClientRect().right - width));
      });
      expect(overflow).toBeLessThanOrEqual(1);

      const scroll = await page.evaluate(() => {
        const before = window.scrollY;
        window.scrollTo({ top: document.documentElement.scrollHeight });
        return { before, after: window.scrollY };
      });
      expect(scroll.after).toBeGreaterThan(scroll.before);
    } finally {
      await page.close();
    }
  });
});
