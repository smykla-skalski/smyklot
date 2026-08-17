import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { SETTLE_MS, startPanel, type Panel } from './harness';

/**
 * The Sync pane of the repository dialog, opened for real.
 *
 * What it reads is server state, held by the shared query client so the stream
 * saying something changed refetches it. That wiring has no unit test that can
 * reach it: the pane's own specs mount the presentational component with a
 * value handed to it, so they pass whether or not anything ever fetches one.
 * This opens the dialog and asks the browser.
 */
let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

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

      const dialog = page.getByRole('dialog', { name: 'smyklot' });
      await dialog.waitFor({ state: 'visible', timeout: 30_000 });
      await page.waitForTimeout(SETTLE_MS);

      // The dialog opens on the repository's own configuration, so nothing has
      // asked what it adjusts yet.
      expect(reads).toEqual([]);

      // The label around the radio, which is what a reader clicks. The radio
      // itself is covered by that label, so a click aimed at the input is a
      // click the label intercepts.
      await dialog.getByRole('radio', { name: 'Sync' }).locator('xpath=ancestor::label[1]').click();

      // The adjustment the mock seeds for this repository, which nothing but a
      // real read produces: the pane renders a card per merge, and an empty
      // answer renders none.
      const merge = dialog.locator('.sync-merge').first();
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

      const dialog = page.getByRole('dialog', { name: 'platform-infra' });
      await dialog.waitFor({ state: 'visible', timeout: 30_000 });
      await page.waitForTimeout(SETTLE_MS);

      await dialog.getByRole('radio', { name: 'Sync' }).locator('xpath=ancestor::label[1]').click();

      const notice = dialog.getByRole('status');
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
