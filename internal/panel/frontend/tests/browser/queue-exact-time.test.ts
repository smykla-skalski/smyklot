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
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (!directory) return;
        await mkdir(directory, { recursive: true });
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
          await occurrence.click();
          await page.getByRole('option', { name: /UTC\+01:00/ }).click();
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
