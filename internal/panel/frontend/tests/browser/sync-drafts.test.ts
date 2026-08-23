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
    /\/api\/v1\/targets\/[^/]+\/settings\/batch$/u.test(new URL(request.url()).pathname)
  );
}

async function flip(page: Page, name: string): Promise<void> {
  const input = page.getByRole('checkbox', { name });
  await page.locator('label.switch').filter({ has: input }).click();
}

describe('installation Sync drafts', () => {
  it('persists across routes and workspaces, then saves every setting once', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    const saves: Request[] = [];
    let dialogs = 0;
    page.on('request', (request) => {
      if (batchSave(request)) saves.push(request);
    });
    page.on('dialog', (dialog) => {
      dialogs += 1;
      void dialog.dismiss();
    });

    try {
      await visit(page, addressOf(panel, 'i/sync/labels'), { ready: 'h2' });
      await flip(page, 'Remove labels this list does not name');
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(0);

      await page.locator(`a[href="/i/${panel.account}/sync/settings"]`).click();
      await page.getByRole('heading', { name: 'Settings' }).waitFor({ state: 'visible' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(0);

      await flip(page, 'Settings sync');
      await page.getByText('2 changed settings').waitFor({ state: 'visible' });
      const saved = page.waitForRequest(batchSave);
      await page.getByRole('button', { name: 'Save' }).click();
      const request = await saved;
      expect((request.postDataJSON() as { sync_configs: unknown[] }).sync_configs).toHaveLength(2);
      await page.getByText('Settings saved').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(1);
      const answer = (await (await request.response())?.json()) as { checkpoint_id?: string };
      expect(answer.checkpoint_id).toBeTruthy();
      const audit = await page.evaluate(async () => {
        const history = (await (await fetch('/api/v1/targets/2001/audit?limit=1')).json()) as {
          items: { action: string; settings_checkpoint_id?: string }[];
        };
        return history.items[0];
      });
      expect(audit).toMatchObject({
        action: 'installation.settings.updated',
        settings_checkpoint_id: answer.checkpoint_id,
      });

      await page.locator(`a[href="/i/${panel.account}/sync/labels"]`).click();
      await flip(page, 'Remove labels this list does not name');
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByText('Unsaved settings restored').waitFor({ state: 'visible' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      const workspaces = page.locator('nav[aria-label="Consoles"] a[href^="/i/"]');
      expect(await workspaces.count()).toBeGreaterThan(1);
      const originalWorkspace = page.locator(
        `nav[aria-label="Consoles"] a[href^="/i/${panel.account}/"]`,
      );
      const other = page
        .locator(`nav[aria-label="Consoles"] a[href^="/i/"]:not([href^="/i/${panel.account}/"])`)
        .first();
      const originalPath = new URL(page.url()).pathname;
      await other.click();
      await page.waitForURL((url) => url.pathname !== originalPath);
      expect(await originalWorkspace.getAttribute('aria-label')).toContain('unsaved changes');
      await originalWorkspace.click();
      await page.waitForURL((url) => url.pathname === `/i/${panel.account}/sync`);
      await page.locator(`a[href="${originalPath}"]`).click();
      await page.waitForURL((url) => url.pathname === originalPath);
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(dialogs).toBe(0);
    } finally {
      await page.close();
    }
  });
});
