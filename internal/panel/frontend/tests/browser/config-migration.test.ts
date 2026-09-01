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
    // A page now, reached by the address it had as a dialog. What proves it is a
    // page is that nothing stands over the list: no dialog role, and the list is
    // not underneath it.
    const title = page.getByRole('heading', { name: repository, exact: true });
    await title.waitFor({ state: 'visible', timeout: 30_000 });
    expect(await page.getByRole('dialog').count()).toBe(0);

    const reset = page.getByRole('button', { name: 'Let it ask' });
    await reset.waitFor({ state: 'visible' });
    const response = page.waitForResponse(
      (candidate) =>
        candidate.request().method() === 'POST' &&
        new URL(candidate.url()).pathname.endsWith('/config-migration'),
    );
    await reset.click();
    expect((await response).status()).toBe(200);
    await reset.waitFor({ state: 'detached' });

    // Leaving and coming back inside the query stale window must not resurrect
    // the refused state from a cached detail response.
    await page.getByRole('link', { name: 'Repositories', exact: true }).first().click();
    await title.waitFor({ state: 'detached' });

    /* The list is virtualised and renders only the rows in view, so a repository
       further down is not in the page to be pressed. Narrow to it the way a
       reader would.

       Typed straight after coming back, which is the part worth keeping: the
       list is the same component the page was drawn inside, so it is still
       mounted with the reader's place in it - it used to be torn down and built
       again from the last stored search, and what had just been typed went with
       it. */
    await page.getByPlaceholder('Find a repository').fill(repository);
    /* Not `exact`: the row's link is the name AND the override count beside it,
       so its accessible name is "search-indexer 2 overrides" for any repository
       that has overrides - which is most of the ones worth opening. */
    await page.getByRole('link', { name: repository }).click();
    await title.waitFor({ state: 'visible' });
    await page.waitForTimeout(SETTLE_MS);
    expect(await page.getByRole('button', { name: 'Let it ask' }).count()).toBe(0);
    expect(crashes).toEqual([]);
  } finally {
    await page.close();
  }
}

describe('the TOML migration reset in the development panel', () => {
  it('works in a workspace and keeps the page presentation', async () => {
    await resetMigration(
      `/workspace/${panel.account}/repositories/migration-demo`,
      'migration-demo',
    );
  });

  it('works through the Root workspace API', async () => {
    await resetMigration(
      `/root/workspaces/${panel.account}/repositories/search-indexer`,
      'search-indexer',
    );
  });
});
