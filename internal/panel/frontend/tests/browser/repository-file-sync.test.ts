import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

/**
 * The File sync card of a repository's own page, read for real.
 *
 * What it shows is server state, held by the shared query client so the stream
 * saying something changed refetches it. That wiring has no unit test that can
 * reach it: the card's own specs mount the presentational component with a
 * value handed to it, so they pass whether or not anything ever fetches one.
 * This opens the page and asks the browser.
 *
 * It used to be a PANE, behind a radio, and these specs pressed it. The page is
 * one scroll now and the card is simply there, so what was "open it and check"
 * is "check", and the two specs that were about the pane's own address are
 * `repository-page.test.ts`'s - which asserts those addresses answer 404.
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
  await visit(page, `${panel.origin}/workspace/${panel.account}/repositories/${name}`, {
    ready: '.repository-page',
  });
  await page
    .getByRole('heading', { name, exact: true })
    .waitFor({ state: 'visible', timeout: 30_000 });

  return page.locator('.repository-page');
}

describe('the repository file sync card in the development panel', () => {
  it('reads what a repository adjusts, and renders both grammars', async () => {
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

      // The adjustments the mock seeds for this repository, which nothing but
      // a real read produces: the card renders an entry per merge, and an empty
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
   * A segment after the repository is somebody else's text, and `constructor`
   * is the shape that catches a guard written as `value in LABELS`: `in` walks
   * the prototype chain, so a key every object has reads as a section.
   *
   * Nothing follows a repository now - the page is one scroll - so the address
   * resolves to no route at either end and never reaches a guard at all.
   * Asserted here rather than trusted, because two copies of a list drifting is
   * exactly how the sync view came to 404.
   */
  it('refuses an address naming anything after the repository', async () => {
    // No crash assertion here, unlike its neighbours: the dev server serves the
    // bundle from the root and the worker registers relative to the address, so
    // an address the server refuses also has no worker under it. That 404 is
    // the harness, not the panel.
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });

    try {
      await page.goto(
        `${panel.origin}/workspace/${panel.account}/repositories/smyklot/constructor`,
        {
          waitUntil: 'domcontentloaded',
        },
      );
      await page.locator('.error-body').waitFor({ state: 'visible', timeout: 30_000 });

      expect(await page.locator('body').innerText()).toContain('Not found');
    } finally {
      await page.close();
    }
  });

  /**
   * Every box in the card is one of the panel's own control heights.
   *
   * `control-heights.test.ts` sweeps the routes and cannot reach this: the card
   * is on a repository's own page, which that sweep does not open, so its six
   * boxes kept the user agent's `2px inset` face and stood at 34px beside the
   * 23.8px chip they replace. The same rule, asked where the sweep cannot go.
   *
   * The seeded repository adjusts two templates, one JSON and one Markdown, so
   * every box the card can draw is on the page: a path, a list rule, a heading
   * and its ordinal, and a find/replace pair.
   */
  it('gives every box in the card a declared height', async () => {
    const page: Page = await panel.browser.newPage({
      viewport: { width: 1280, height: 900 },
    });

    try {
      const repository = await repositoryPage(page, 'smyklot');
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

      // Every shape the card draws, so a box added without a class fails here
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
   * Everything about that lives on a second row the card reads beside the
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
      /* platform-infra, because this notice is the OVERRIDE's own problem - the
         repository's stored answer carries why the planner stood down there -
         and not the fleet's refusal, which is a different repository and a
         different reason. */
      const repository = await repositoryPage(page, 'platform-infra');

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
