import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop shared-file lifecycle', () => {
  it.each(['light', 'dark'] as const)(
    'keeps a new %s template unsaved until its save succeeds',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      page.setDefaultTimeout(5000);
      const path = `audit-draft-${colorScheme}.json`;
      const saves: unknown[] = [];
      let refuse = true;
      await page.route('**/api/v1/targets/*/settings', async (route) => {
        if (route.request().method() !== 'PUT') return route.continue();
        saves.push(route.request().postDataJSON());
        if (refuse) {
          refuse = false;
          return route.fulfill({
            status: 503,
            json: { error: { code: 'unavailable', message: 'Save is temporarily unavailable' } },
          });
        }
        return route.continue();
      });
      try {
        await visit(page, addressOf(panel, 'workspace/sync/files'));
        await page.getByRole('button', { name: 'Add a file', exact: true }).click();
        await page.getByPlaceholder('renovate.json, or a path no repository has yet').fill(path);
        await page
          .getByRole('option')
          .filter({ hasText: `Start ${path}` })
          .click();
        await page.getByRole('heading', { name: path, exact: true }).waitFor();
        await expect
          .poll(() => page.locator('.page-head').innerText())
          .toContain('Unsaved new file');
        expect(await page.locator('.page-head').innerText()).not.toContain('updated');
        expect(
          await page
            .getByText(`Draft template created for ${path}. Add content, then save.`, {
              exact: true,
            })
            .count(),
        ).toBe(1);
        expect(saves).toHaveLength(0);
        await page.locator('.cm-content').first().fill('{"enabled": true}');
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        expect(saves).toHaveLength(0);
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Settings were not saved', { exact: true }).waitFor();
        expect(await page.locator('.page-head').innerText()).toContain('Unsaved new file');
        expect(await page.locator('.cm-content').first().innerText()).toContain('"enabled": true');
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Settings saved', { exact: true }).waitFor();
        await expect
          .poll(() => page.locator('.page-head').innerText())
          .not.toContain('Unsaved new file');
        expect(await page.locator('.page-head').innerText()).toContain(
          'shared-file settings updated',
        );
        expect(saves).toHaveLength(2);
      } finally {
        await page.close();
      }
    },
  );
});
