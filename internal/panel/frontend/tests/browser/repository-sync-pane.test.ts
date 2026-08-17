import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { SETTLE_MS, startPanel, type Panel } from './harness';

/**
 * The Sync pane of a repository's own page, opened for real.
 *
 * What it reads is server state, held by the shared query client so the stream
 * saying something changed refetches it. That wiring has no unit test that can
 * reach it: the pane's own specs mount the presentational component with a
 * value handed to it, so they pass whether or not anything ever fetches one.
 * This opens the page and asks the browser.
 */
let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

/** The page, once its heading says it is the repository that was asked for. */
async function repositoryPage(page: Page, name: string) {
  await page
    .getByRole('heading', { name, exact: true })
    .waitFor({ state: 'visible', timeout: 30_000 });

  return page.locator('.repository-page');
}

describe('the repository sync pane in the development panel', () => {
  it('reads what a repository adjusts when the pane is opened, and not before', async () => {
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });
    const crashes: string[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));

    const reads: string[] = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname;
      if (request.method() === 'GET' && path.includes('/sync/files')) reads.push(path);
    });

    try {
      await page.goto(`${panel.origin}/i/${panel.account}/repositories/smyklot`, {
        waitUntil: 'domcontentloaded',
      });

      const repository = await repositoryPage(page, 'smyklot');
      await page.waitForTimeout(SETTLE_MS);

      // The page opens on the repository's own configuration, so nothing has
      // asked what it adjusts yet.
      expect(reads).toEqual([]);

      // The label around the radio, which is what a reader clicks. The radio
      // itself is covered by that label, so a click aimed at the input is a
      // click the label intercepts.
      await repository
        .getByRole('radio', { name: 'Sync' })
        .locator('xpath=ancestor::label[1]')
        .click();

      // The adjustment the mock seeds for this repository, which nothing but a
      // real read produces: the pane renders a card per merge, and an empty
      // answer renders none.
      const merge = repository.locator('.sync-merge').first();
      await merge.waitFor({ state: 'visible', timeout: 30_000 });

      expect(await merge.locator('.sync-merge-path input').inputValue()).toBe('renovate.json');
      expect(await merge.locator('.sync-merge-overrides').inputValue()).toContain('Europe/Warsaw');
      expect(reads.length).toBeGreaterThan(0);

      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });

  /**
   * The address the panel writes when somebody opens the pane has to be one it
   * can read back. It was not: the sections the parser accepts were listed
   * without this one, so choosing Sync moved the address to a path the panel's
   * own guard answered 404 for - on a reload, or on a link pasted to somebody
   * else. Nothing caught it because nothing reloaded the address it wrote.
   */
  it('writes an address for the pane that survives a reload', async () => {
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });
    const crashes: string[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));

    try {
      await page.goto(`${panel.origin}/i/${panel.account}/repositories/smyklot`, {
        waitUntil: 'domcontentloaded',
      });

      const repository = await repositoryPage(page, 'smyklot');
      await page.waitForTimeout(SETTLE_MS);

      await repository
        .getByRole('radio', { name: 'Sync' })
        .locator('xpath=ancestor::label[1]')
        .click();
      await page.waitForTimeout(SETTLE_MS);

      // What the panel put in the address bar, reloaded as somebody pasting it
      // would get it.
      const written = page.url();
      expect(written).toContain('/sync');

      await page.goto(written, { waitUntil: 'domcontentloaded' });
      await page.waitForTimeout(SETTLE_MS);

      const reopened = await repositoryPage(page, 'smyklot');
      expect(await reopened.getByRole('radio', { name: 'Sync' }).isChecked()).toBe(true);

      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });

  /**
   * The pane an address names is somebody else's text, and `constructor` is the
   * shape that catches a guard written as `value in LABELS`: `in` walks the
   * prototype chain, so a key every object has reads as a pane, and the page
   * renders the empty box its fallback exists to avoid with `Object` itself in
   * the pane's accessible name.
   *
   * A pane is a path segment now, matched against `REPOSITORY_SECTIONS` by a
   * regex the param matcher derives from that list, and the Go server is handed
   * the same pattern - so the address does not resolve at either end and never
   * reaches a guard at all. Asserted here rather than trusted, because the two
   * copies of the list drifting is exactly how the sync view came to 404.
   */
  it('refuses an address naming a pane that does not exist', async () => {
    // No crash assertion here, unlike its neighbours: the dev server serves the
    // bundle from the root and the worker registers relative to the address, so
    // an address the server refuses also has no worker under it. That 404 is
    // the harness, not the panel.
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });

    try {
      await page.goto(`${panel.origin}/i/${panel.account}/repositories/smyklot/constructor`, {
        waitUntil: 'domcontentloaded',
      });
      await page.locator('.error-body').waitFor({ state: 'visible', timeout: 30_000 });

      expect(await page.locator('body').innerText()).toContain('Not found');
    } finally {
      await page.close();
    }
  });

  /**
   * A repository the planner refuses receives none of the organization's files.
   * Everything about that lives on a second row the pane reads beside the
   * adjustments, so a component spec handed a value proves none of it - this
   * asks the browser, through the real read.
   */
  it('says why a repository is getting none of the files', async () => {
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });
    const crashes: string[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));

    try {
      await page.goto(`${panel.origin}/i/${panel.account}/repositories/platform-infra`, {
        waitUntil: 'domcontentloaded',
      });

      const repository = await repositoryPage(page, 'platform-infra');
      await page.waitForTimeout(SETTLE_MS);

      await repository
        .getByRole('radio', { name: 'Sync' })
        .locator('xpath=ancestor::label[1]')
        .click();

      const notice = repository.getByRole('status');
      await notice.waitFor({ state: 'visible', timeout: 30_000 });

      const said = await notice.textContent();
      expect(said).toContain('are not being synced here');
      expect(said).toContain('docs is not a directory in this repository');
      expect(said).toContain('Last looked at');

      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });
});
