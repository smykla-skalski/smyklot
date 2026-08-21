import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Locator, Page } from 'playwright-core';

import { settle, startPanel, visit, type Panel } from './harness';

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

/**
 * Opens one repository's page and answers with it, once its heading says it is
 * the repository that was asked for.
 *
 * Through the harness's `visit` rather than a goto and a fixed sleep: the sleep
 * is a guess in both directions, and it is most of what this suite costs.
 */
async function repositoryPage(page: Page, name: string) {
  await visit(page, `${panel.origin}/i/${panel.account}/repositories/${name}`, {
    ready: '.repository-page',
  });
  await page
    .getByRole('heading', { name, exact: true })
    .waitFor({ state: 'visible', timeout: 30_000 });

  return page.locator('.repository-page');
}

/** The pane switch is a radio under a label, and the label is what covers it. */
async function openPane(page: Page, repository: Locator, name: string): Promise<void> {
  await settle(page, async () => {
    await repository.getByRole('radio', { name }).locator('xpath=ancestor::label[1]').click();
  });
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
      const repository = await repositoryPage(page, 'smyklot');

      // The page opens on the repository's own configuration, so nothing has
      // asked what it adjusts yet.
      expect(reads).toEqual([]);

      await openPane(page, repository, 'Sync');

      // The adjustments the mock seeds for this repository, which nothing but
      // a real read produces: the pane renders a card per merge, and an empty
      // answer renders none. Both templates, so both grammars are on the page.
      const json = repository.locator('.entry-card').first();
      await json.waitFor({ state: 'visible', timeout: 30_000 });

      expect(await json.getByRole('textbox', { name: 'File', exact: true }).inputValue()).toBe(
        'renovate.json',
      );
      const markdown = repository.locator('.entry-card').nth(1);
      expect(await markdown.getByRole('textbox', { name: 'File', exact: true }).inputValue()).toBe(
        'CONTRIBUTING.md',
      );
      expect(
        await markdown.getByRole('textbox', { name: 'Heading', exact: true }).first().inputValue(),
      ).toBe('## Commits');
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
      const repository = await repositoryPage(page, 'smyklot');
      await openPane(page, repository, 'Sync');

      // What the panel put in the address bar, reloaded as somebody pasting it
      // would get it.
      const written = page.url();
      expect(written).toContain('/sync');

      await visit(page, written, { ready: '.repository-page' });

      const reopened = page.locator('.repository-page');
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
   * Every box in the pane is one of the panel's own control heights.
   *
   * `control-heights.test.ts` sweeps the routes and cannot reach this: the pane
   * is behind a radio on a repository's page, so its six boxes kept the user
   * agent's `2px inset` face and stood at 34px beside the 23.8px chip they
   * replace - a ten pixel jump on opening a row. The same rule, asked where the
   * sweep cannot go.
   *
   * The seeded repository adjusts two templates, one JSON and one Markdown, so
   * every box the pane can draw is on the page without a press: a path, a list
   * rule, a heading and its ordinal, and a find/replace pair.
   */
  it('gives every box in the pane a declared height', async () => {
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });

    try {
      const repository = await repositoryPage(page, 'smyklot');
      await openPane(page, repository, 'Sync');
      await repository
        .locator('.sync-merge')
        .first()
        .waitFor({ state: 'visible', timeout: 30_000 });

      const boxes = await page.evaluate(() =>
        [...document.querySelectorAll<HTMLInputElement>('.sync-merge input')]
          .filter((box) => !['checkbox', 'radio'].includes(box.type))
          .map((box) => ({
            where: box.getAttribute('placeholder') ?? box.type,
            height: box.getBoundingClientRect().height,
          })),
      );

      // Every shape the pane draws, so a box added without a class fails here
      // rather than passing on an empty list.
      expect(boxes.length).toBeGreaterThanOrEqual(6);
      expect(
        boxes.filter((box) => box.height !== 34).map((box) => `${box.where} ${box.height}px`),
      ).toEqual([]);
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
      const repository = await repositoryPage(page, 'platform-infra');
      await openPane(page, repository, 'Sync');

      /* Scoped to the stand-down line: the merge card's saved receipt is a
         status too, quietly present so a save can announce itself. */
      const notice = repository.locator('.sync-pane-standdown');
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
