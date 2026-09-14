import axe from 'axe-core';
import { mkdir, writeFile } from 'node:fs/promises';
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
for (const timezoneId of ['Europe/Warsaw', 'Asia/Tokyo']) {
  it.each(['light', 'dark'] as const)(
    `resolves exact times in ${timezoneId} and %s desktop`,
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        timezoneId,
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(5000);
      const capture = async (scene: string) => {
        await page.evaluate(axe.source);
        const accessibility = await page.evaluate(async () => {
          const roots = [...document.querySelectorAll('[role="dialog"], [role="listbox"]')];
          return (window as unknown as { axe: typeof axe }).axe.run({ include: roots });
        });
        expect(
          accessibility.violations.map(({ id, nodes }) => ({
            id,
            targets: nodes.map(({ target, html, failureSummary }) => ({
              target,
              html,
              failureSummary,
            })),
          })),
        ).toEqual([]);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
        await writeFile(
          join(
            directory,
            '..',
            'evidence',
            `F14-${timezoneId.split('/')[1]}-${scene}-${colorScheme}-axe.json`,
          ),
          JSON.stringify(accessibility, null, 2),
        );
        await page.screenshot({
          path: join(directory, `F14-${timezoneId.split('/')[1]}-${scene}-${colorScheme}.png`),
          animations: 'disabled',
        });
      };
      try {
        const requested: string[] = [];
        await page.route('**/queue/*/actions/preview', async (route) => {
          requested.push(route.request().postDataJSON().at);
          await route.continue();
        });
        await page.goto(`${panel.origin}/root/queue`);
        await page
          .getByRole('button', { name: 'Actions for Scan for new commands', exact: true })
          .click();
        await page.getByRole('menuitem', { name: /^Schedule exact time/ }).click();
        const dialog = page.getByRole('dialog', { name: 'Schedule exact time', exact: true });
        const input = dialog.getByLabel('Not before', { exact: true });
        const preview = dialog.getByRole('button', { name: 'Preview earliest start', exact: true });
        await input.fill('');
        await dialog
          .getByText(`Required. Enter a time in ${timezoneId}, your browser's timezone.`, {
            exact: true,
          })
          .waitFor();
        expect(await preview.isDisabled()).toBe(true);
        await capture('empty');
        if (timezoneId === 'Europe/Warsaw') {
          await input.fill('2026-03-29T02:30');
          await preview.click();
          await dialog.getByRole('alert').waitFor();
          expect(await input.getAttribute('aria-invalid')).toBe('true');
          expect(await input.getAttribute('aria-describedby')).toContain('queue-action-time-error');
          expect(requested).toEqual([]);
          expect(
            await dialog.getByRole('button', { name: 'Apply', exact: true }).isDisabled(),
          ).toBe(true);
          await capture('gap');
        }
        await input.fill('2026-10-25T02:30');
        await preview.click();
        if (timezoneId === 'Europe/Warsaw') {
          const occurrence = dialog.getByRole('combobox', { name: 'Occurrence', exact: true });
          await occurrence.waitFor();
          expect(await occurrence.textContent()).toContain('Choose an occurrence');
          expect(requested).toEqual([]);
          expect(
            await dialog.getByRole('button', { name: 'Apply', exact: true }).isDisabled(),
          ).toBe(true);
          await capture('repeated');
          await occurrence.focus();
          await page.keyboard.press('ArrowDown');
          await page.getByRole('listbox', { name: 'Occurrence', exact: true }).waitFor();
          const controlled = await occurrence.getAttribute('aria-controls');
          expect(await page.locator(`[id="${controlled}"]`).count()).toBe(1);
          await capture('occurrence-menu');
          await page.keyboard.press('End');
          await page.keyboard.press('Enter');
          expect(await occurrence.textContent()).toContain('UTC+01:00');
          expect(await occurrence.evaluate((node) => node === document.activeElement)).toBe(true);
          await preview.click();
        }
        await dialog.getByText('Can start from', { exact: true }).waitFor();
        expect(requested).toEqual([
          timezoneId === 'Europe/Warsaw' ? '2026-10-25T01:30:00Z' : '2026-10-24T17:30:00Z',
        ]);
        expect(await dialog.getByRole('button', { name: 'Apply', exact: true }).isDisabled()).toBe(
          false,
        );
        await capture('preview');
        await input.fill('2026-10-26T02:30');
        expect(await dialog.getByRole('button', { name: 'Apply', exact: true }).isDisabled()).toBe(
          true,
        );
        expect(await dialog.getByText('Can start from', { exact: true }).count()).toBe(0);
        await preview.click();
        await dialog.getByText('Can start from', { exact: true }).waitFor();
        const submitted: string[] = [];
        await page.route('**/queue/*/actions', async (route) => {
          submitted.push(route.request().postDataJSON().at);
          await route.fulfill({
            status: 400,
            json: {
              error: { code: 'test_rejection', message: 'Keep this time for another attempt.' },
            },
          });
        });
        await dialog.getByRole('button', { name: 'Apply', exact: true }).click();
        await dialog.getByText('Keep this time for another attempt.', { exact: true }).waitFor();
        expect(submitted).toEqual([
          timezoneId === 'Europe/Warsaw' ? '2026-10-26T01:30:00Z' : '2026-10-25T17:30:00Z',
        ]);
        expect(await input.inputValue()).toBe('2026-10-26T02:30');
      } finally {
        await page.close();
      }
    },
  );
}

it.each(['light', 'dark'] as const)(
  'keeps the latest exact-time preview in %s desktop',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      timezoneId: 'Europe/Warsaw',
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    let releaseOld = () => {};
    const held = new Promise<void>((resolve) => {
      releaseOld = resolve;
    });
    let sawOld = () => {};
    const oldStarted = new Promise<void>((resolve) => {
      sawOld = resolve;
    });
    try {
      let resolutions = 0;
      await page.route('**/schedule-local-time?*', async (route) => {
        resolutions++;
        if (resolutions !== 1) return route.continue();
        await route.fulfill({
          status: 503,
          json: {
            error: { code: 'unavailable', message: 'Time lookup is unavailable. Try again.' },
          },
        });
      });
      let previews = 0;
      await page.route('**/queue/*/actions/preview', async (route) => {
        previews++;
        if (previews !== 1) return route.continue();
        const response = await route.fetch();
        sawOld();
        await held;
        await route.fulfill({ response });
      });
      await page.goto(`${panel.origin}/root/queue`);
      await page
        .getByRole('button', { name: 'Actions for Scan for new commands', exact: true })
        .click();
      await page.getByRole('menuitem', { name: /^Schedule exact time/ }).click();
      const dialog = page.getByRole('dialog', { name: 'Schedule exact time', exact: true });
      const input = dialog.getByLabel('Not before', { exact: true });
      const preview = dialog.getByRole('button', { name: 'Preview earliest start', exact: true });
      const apply = dialog.getByRole('button', { name: 'Apply', exact: true });
      await input.fill('2026-10-26T02:30');
      await preview.focus();
      await page.keyboard.press('Enter');
      await dialog.getByText('Time lookup is unavailable. Try again.', { exact: true }).waitFor();
      expect(await input.inputValue()).toBe('2026-10-26T02:30');
      expect(await apply.isDisabled()).toBe(true);
      expect(await preview.evaluate((element) => element === document.activeElement)).toBe(true);
      await page.keyboard.press('Enter');
      await oldStarted;
      const calculating = dialog.getByRole('button', { name: 'Calculating…', exact: true });
      expect(await calculating.getAttribute('aria-disabled')).toBe('true');
      expect(await calculating.evaluate((element) => element === document.activeElement)).toBe(
        true,
      );
      await page.keyboard.press('Enter');
      expect(previews).toBe(1);
      await input.fill('2026-10-27T02:30');
      await preview.click();
      const time = dialog.locator('.schedule-preview time');
      await time.waitFor();
      expect(Date.parse((await time.getAttribute('datetime'))!)).toBe(
        Date.parse('2026-10-27T01:30:00Z'),
      );
      const oldReturned = page.waitForResponse(
        (response) =>
          response.url().endsWith('/actions/preview') &&
          response.request().postDataJSON().at === '2026-10-26T01:30:00Z',
      );
      releaseOld();
      await oldReturned;
      // Let the returned request's promise handlers and the next render complete.
      await page.evaluate(
        () =>
          new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          ),
      );
      expect(Date.parse((await time.getAttribute('datetime'))!)).toBe(
        Date.parse('2026-10-27T01:30:00Z'),
      );
      expect(await apply.isDisabled()).toBe(false);
      expect(resolutions).toBe(3);
      expect(previews).toBe(2);
    } finally {
      releaseOld();
      await page.close();
    }
  },
);

it.each(['light', 'dark'] as const)(
  'cancels an obsolete time lookup in %s desktop',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      timezoneId: 'Europe/Warsaw',
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    let started = () => {};
    const arrived = new Promise<void>((resolve) => {
      started = resolve;
    });
    try {
      let lookups = 0;
      await page.route('**/schedule-local-time?*', async (route) => {
        if (++lookups !== 1) return route.continue();
        started();
        await held;
        await route.abort();
      });
      await page.goto(`${panel.origin}/root/queue`);
      await page
        .getByRole('button', { name: 'Actions for Scan for new commands', exact: true })
        .click();
      await page.getByRole('menuitem', { name: /^Schedule exact time/ }).click();
      const dialog = page.getByRole('dialog', { name: 'Schedule exact time', exact: true });
      const input = dialog.getByLabel('Not before', { exact: true });
      await input.fill('2026-10-25T02:30');
      await dialog.getByRole('button', { name: 'Preview earliest start', exact: true }).click();
      await arrived;
      const cancelled = page.waitForEvent('requestfailed', {
        predicate: (request) => request.url().includes('/schedule-local-time?'),
      });
      await input.fill('2026-10-26T02:30');
      await cancelled;
      release();
      await dialog.getByRole('button', { name: 'Preview earliest start', exact: true }).click();
      await dialog.locator('.schedule-preview time').waitFor();
      expect(await dialog.getByRole('combobox', { name: 'Occurrence', exact: true }).count()).toBe(
        0,
      );
      expect(await dialog.getByRole('alert').count()).toBe(0);
      expect(await dialog.getByRole('button', { name: 'Apply', exact: true }).isDisabled()).toBe(
        false,
      );
      expect(lookups).toBe(2);
    } finally {
      release();
      await page.close();
    }
  },
);
