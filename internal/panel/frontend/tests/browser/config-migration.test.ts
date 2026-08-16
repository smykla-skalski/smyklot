import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { SETTLE_MS, startPanel, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

async function resetMigration(path: string, repository: string): Promise<void> {
  const page: Page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
  const crashes: string[] = [];
  page.on('pageerror', (error) => crashes.push(error.message));

  try {
    await page.goto(`${panel.origin}${path}`, { waitUntil: 'domcontentloaded' });
    const dialog = page.getByRole('dialog', { name: repository });
    await dialog.waitFor({ state: 'visible', timeout: 30_000 });

    expect(await dialog.getByRole('button', { name: 'Close dialog' }).count()).toBe(0);
    const reset = dialog.getByRole('button', { name: 'Let it ask' });
    await reset.waitFor({ state: 'visible' });
    const response = page.waitForResponse(
      (candidate) =>
        candidate.request().method() === 'POST' &&
        new URL(candidate.url()).pathname.endsWith('/config-migration'),
    );
    await reset.click();
    expect((await response).status()).toBe(200);
    await reset.waitFor({ state: 'detached' });

    // Closing and reopening inside the query stale window must not resurrect
    // the refused state from a cached detail response.
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'detached' });
    await page.getByRole('button', { name: `Configure smykla-skalski/${repository}` }).click();
    await dialog.waitFor({ state: 'visible' });
    await page.waitForTimeout(SETTLE_MS);
    expect(await dialog.getByRole('button', { name: 'Let it ask' }).count()).toBe(0);
    expect(crashes).toEqual([]);
  } finally {
    await page.close();
  }
}

describe('the TOML migration reset in the development panel', () => {
  it('works in an installation and keeps the modal presentation', async () => {
    await resetMigration(`/i/${panel.account}/repositories/migration-demo`, 'migration-demo');
  });

  it('works through the Root installation API', async () => {
    await resetMigration(
      `/root/installations/${panel.account}/repositories/search-indexer`,
      'search-indexer',
    );
  });
});
