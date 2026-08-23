import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page, Request } from 'playwright-core';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

function batchSave(request: Request): boolean {
  return (
    request.method() === 'PUT' &&
    /\/api\/v1\/targets\/[^/]+\/sync\/config$/u.test(new URL(request.url()).pathname)
  );
}

async function flip(page: Page, name: string): Promise<void> {
  await page.getByRole('checkbox', { name }).click({ force: true });
}

describe('installation Sync drafts', () => {
  it('persists across routes, saves once, and protects every departure', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    const saves: Request[] = [];
    page.on('request', (request) => {
      if (batchSave(request)) saves.push(request);
    });

    try {
      await visit(page, addressOf(panel, 'i/sync/labels'), { ready: 'h2' });
      await flip(page, 'Remove labels this list does not name');
      await page.getByText('1 changed Sync section').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(0);

      await page.locator(`a[href="/i/${panel.account}/sync/settings"]`).click();
      await page.getByRole('heading', { name: 'Settings' }).waitFor({ state: 'visible' });
      await page.getByText('1 changed Sync section').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(0);

      await flip(page, 'Settings sync');
      await page.getByText('2 changed Sync sections').waitFor({ state: 'visible' });
      const saved = page.waitForRequest(batchSave);
      await page.getByRole('button', { name: 'Save' }).click();
      const request = await saved;
      expect((request.postDataJSON() as { changes: unknown[] }).changes).toHaveLength(2);
      await page.getByText('Sync configuration saved').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(1);

      await page.locator(`a[href="/i/${panel.account}/sync/labels"]`).click();
      await flip(page, 'Remove labels this list does not name');
      await page.getByText('1 changed Sync section').waitFor({ state: 'visible' });

      const unloadDialog = page.waitForEvent('dialog');
      const reload = page.reload({ waitUntil: 'domcontentloaded' }).catch(() => null);
      const unload = await unloadDialog;
      expect(unload.type()).toBe('beforeunload');
      await unload.dismiss();
      await reload;
      await page.getByText('1 changed Sync section').waitFor({ state: 'visible' });

      await page.getByRole('button', { name: /^Account menu for/u }).click();
      const signOutDialog = page.waitForEvent('dialog');
      const signOutClick = page.getByRole('menuitem', { name: 'Sign out' }).click();
      const signOut = await signOutDialog;
      expect(signOut.message()).toContain('Discard your unsaved Sync configuration changes?');
      await signOut.dismiss();
      await signOutClick;
      await page.getByText('1 changed Sync section').waitFor({ state: 'visible' });

      const workspaces = page.locator('nav[aria-label="Consoles"] a[href^="/i/"]');
      expect(await workspaces.count()).toBeGreaterThan(1);
      const other = workspaces.nth(1);
      const originalPath = new URL(page.url()).pathname;
      const stayDialog = page.waitForEvent('dialog');
      const stayClick = other.click();
      const stay = await stayDialog;
      await stay.dismiss();
      await stayClick;
      expect(new URL(page.url()).pathname).toBe(originalPath);
      await page.getByText('1 changed Sync section').waitFor({ state: 'visible' });

      const leaveDialog = page.waitForEvent('dialog');
      const leaveClick = other.click();
      const leave = await leaveDialog;
      await leave.accept();
      await leaveClick;
      await page.waitForURL((url) => url.pathname !== originalPath);
      expect(
        await page.getByRole('complementary', { name: 'Sync configuration draft' }).count(),
      ).toBe(0);
    } finally {
      await page.close();
    }
  });
});
