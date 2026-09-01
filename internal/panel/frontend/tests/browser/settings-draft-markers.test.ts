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
  await page.goto(`${panel.origin}/workspace/${panel.account}/repositories`, {
    waitUntil: 'domcontentloaded',
  });
  await page.locator('.repository-row').first().waitFor({ state: 'visible', timeout: 30_000 });
}

describe('settings draft destinations [Integration]', () => {
  it('identifies the dirty workspace, section, repository, and exact setting', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await openRepositories(page);
      const repositoryRow = page.locator('.repository-row').first();
      const repositoryName =
        (await repositoryRow.locator('.object-name').textContent())?.trim() ?? '';
      expect(repositoryName).not.toBe('');
      await repositoryRow.locator('a.row-hit').click();

      const quietPeriod = page.getByRole('textbox', { name: 'Quiet period after checks pass' });
      await quietPeriod.waitFor({ state: 'visible', timeout: 15_000 });
      const savedValue = await quietPeriod.inputValue();
      await quietPeriod.fill(savedValue === '45' ? '46' : '45');
      await quietPeriod.blur();
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      const settingRow = page.locator('.policy-row').filter({ has: quietPeriod });
      expect(await settingRow.getAttribute('data-unsaved')).toBe('true');

      /* The draft survives the reload, and what says so is the page it is on plus the
         tree's mark on every scope holding one. Nothing announces it: a notice would
         report a thing the reader had not just done, and cover the page saying it. */
      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible', timeout: 15_000 });
      expect(await page.locator('.tree-row.has-dirty').count()).toBeGreaterThan(0);

      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
      await page.getByRole('button', { name: 'Override the deployment session lifetime' }).click();
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      await page.goto(`${panel.origin}/root/workspaces`, { waitUntil: 'domcontentloaded' });
      const rootRepositoryHref = `/root/workspaces/${panel.account}/repositories`;
      /* The console's catalog is a list of sentences now, and a workspace is opened by
         name rather than by pressing its row - so the link is the row's one act, and it
         still carries where the unsaved work is. */
      const workspaceLink = page.locator(`a[href="${rootRepositoryHref}"]`);
      await workspaceLink.waitFor({ state: 'visible', timeout: 30_000 });
      const workspaceRow = page.locator('.object-row', { has: workspaceLink });
      expect(await workspaceRow.getAttribute('data-unsaved')).toBe('true');
      expect(await workspaceRow.innerText()).toContain('1 unsaved setting');

      await workspaceLink.click();
      await page.waitForURL((url) => url.pathname === rootRepositoryHref);
      const repositoryLeaf = page.locator(`a.tree-row[href="${rootRepositoryHref}"]`);
      expect(await repositoryLeaf.innerText()).toContain('Unsaved changes');

      const markedRepository = page
        .locator('.repository-row')
        .filter({ has: page.locator('.object-name', { hasText: repositoryName }) })
        .first();
      await markedRepository.waitFor({ state: 'visible', timeout: 15_000 });
      expect(await markedRepository.getAttribute('data-unsaved')).toBe('true');
      expect(await markedRepository.innerText()).toContain('Unsaved changes');
      expect(await page.locator('.settings-composer').count()).toBe(1);

      await page.getByRole('button', { name: 'Discard', exact: true }).click();
      await markedRepository.waitFor({ state: 'visible' });
      expect(await markedRepository.getAttribute('data-unsaved')).toBeNull();
      expect(await page.locator('.settings-composer').count()).toBe(0);

      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
      await page.getByRole('button', { name: 'Discard', exact: true }).click();
    } finally {
      await page.close();
    }
  });
});
