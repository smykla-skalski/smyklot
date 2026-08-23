import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { startPanel, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

async function openRepositories(page: Page): Promise<void> {
  await page.goto(`${panel.origin}/i/${panel.account}/repositories`, {
    waitUntil: 'domcontentloaded',
  });
  await page.locator('.repository-row').first().waitFor({ state: 'visible', timeout: 30_000 });
}

describe('settings draft destinations [Integration]', () => {
  it('identifies the dirty installation, section, repository, and exact setting', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await openRepositories(page);
      const repositoryRow = page.locator('.repository-row').first();
      const repositoryName =
        (await repositoryRow.locator('.repo-copy strong').textContent())?.trim() ?? '';
      expect(repositoryName).not.toBe('');
      await repositoryRow.locator('a.repo-copy').click();

      const quietPeriod = page.getByRole('textbox', { name: 'Stable passing window' });
      await quietPeriod.waitFor({ state: 'visible', timeout: 15_000 });
      const savedValue = await quietPeriod.inputValue();
      await quietPeriod.fill(savedValue === '45' ? '46' : '45');
      await quietPeriod.blur();
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      const settingRow = page.locator('.policy-row').filter({ has: quietPeriod });
      expect(await settingRow.getAttribute('data-unsaved')).toBe('true');

      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByText('Unsaved settings restored').waitFor({ state: 'visible' });
      expect(await page.getByRole('link', { name: 'Review' }).getAttribute('href')).toBe(
        `/i/${panel.account}/repositories`,
      );

      await page.goto(`${panel.origin}/root/installations`, { waitUntil: 'domcontentloaded' });
      const rootRepositoryHref = `/root/installations/${panel.account}/repositories`;
      const installationLink = page.locator(`a.installation-link[href="${rootRepositoryHref}"]`);
      await installationLink.waitFor({ state: 'visible', timeout: 30_000 });
      const installationRow = installationLink.locator('xpath=ancestor::tr');
      expect(await installationRow.getAttribute('data-unsaved')).toBe('true');
      expect(await installationRow.innerText()).toContain('Unsaved changes');

      await installationLink.click();
      await page.waitForURL((url) => url.pathname === rootRepositoryHref);
      const repositoryLeaf = page.locator(`a.tree-kid[href="${rootRepositoryHref}"]`);
      expect(await repositoryLeaf.innerText()).toContain('Unsaved changes');

      const markedRepository = page
        .locator('.repository-row')
        .filter({ has: page.locator('.repo-copy strong', { hasText: repositoryName }) })
        .first();
      await markedRepository.waitFor({ state: 'visible', timeout: 15_000 });
      expect(await markedRepository.getAttribute('data-unsaved')).toBe('true');
      expect(await markedRepository.innerText()).toContain('Unsaved changes');

      await page.getByRole('button', { name: 'Discard', exact: true }).click();
      await markedRepository.waitFor({ state: 'visible' });
      expect(await markedRepository.getAttribute('data-unsaved')).toBeNull();
    } finally {
      await page.close();
    }
  });
});
