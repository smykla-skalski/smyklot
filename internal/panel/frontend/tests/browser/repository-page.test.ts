import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { startPanel, type Panel } from './harness';

/**
 * One repository opens as a page, and the row is what opens it.
 *
 * The row is a pointer target laid over controls that are already press targets,
 * which is the whole risk in the change: the enablement switch is a radio and a
 * label per option, so a press on the WORD "Enabled" has the label as its
 * target, and a row handler that only skipped buttons opened the repository and
 * set its enablement from one press. That is measured here rather than reasoned
 * about, because it is invisible - the page opens over the list, so the switch
 * moving underneath is not something a reader would see happen.
 */

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

async function openList(): Promise<Page> {
  const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
  await page.goto(`${panel.origin}/workspace/${panel.account}/repositories`, {
    waitUntil: 'domcontentloaded',
  });
  await page.locator('.repository-row').first().waitFor({ state: 'visible', timeout: 30_000 });
  /* A search stored from another sweep would leave a filtered list, and a row
     index means nothing then. */
  await page.getByPlaceholder('Find a repository').fill('');
  await page.locator('.repository-row').nth(1).waitFor({ state: 'visible' });

  return page;
}

describe('one repository as a page [Integration]', () => {
  it('opens from a press on the row, not only on its name', async () => {
    const page = await openList();
    try {
      const row = page.locator('.repository-row').first();
      const name = (await row.locator('.object-name').textContent())?.trim() ?? '';
      expect(name).not.toBe('');

      /* A real press where the row's sentence is - text belonging to no control.
         What receives it is the layer the row's address is drawn on, which is the
         whole point: everything that is not a control opens the repository. */
      const sentence = await row.locator('.object-sum').boundingBox();
      if (sentence === null) throw new Error('the row has no sentence to press');
      await page.mouse.click(sentence.x + 8, sentence.y + sentence.height / 2);

      await page
        .getByRole('heading', { name, exact: true })
        .waitFor({ state: 'visible', timeout: 15_000 });
      expect(new URL(page.url()).pathname).toBe(
        `/workspace/${panel.account}/repositories/${encodeURIComponent(name)}`,
      );
    } finally {
      await page.close();
    }
  });

  it('leaves the enablement switch to the enablement switch', async () => {
    const page = await openList();
    try {
      const row = page.locator('.repository-row').nth(1);
      const before = new URL(page.url()).pathname;
      /* The track, not the input: the checkbox is visually hidden under it, and
         the track is what a reader presses. */
      const track = row.locator('.switch-track');
      const box = row.locator('input[type="checkbox"]');
      const wasEnabled = await box.isChecked();

      await track.click();
      await page.waitForTimeout(400);

      expect(new URL(page.url()).pathname, 'the row opened as well as switched').toBe(before);
      expect(await box.isChecked()).toBe(!wasEnabled);

      // Put it back, because the mock keeps this and every other sweep reads it.
      await track.click();
      await page.waitForTimeout(400);
      expect(await box.isChecked()).toBe(wasEnabled);
    } finally {
      await page.close();
    }
  });

  it('opens from the keyboard, through the one link in the row', async () => {
    const page = await openList();
    try {
      const row = page.locator('.repository-row').first();
      const name = (await row.locator('.object-name').textContent())?.trim() ?? '';
      const link = row.locator('a.row-hit');

      await link.focus();
      await page.keyboard.press('Enter');

      await page
        .getByRole('heading', { name, exact: true })
        .waitFor({ state: 'visible', timeout: 15_000 });
    } finally {
      await page.close();
    }
  });

  /**
   * The repository is the whole address, and the whole page is on it.
   *
   * It used to be five panes behind a switch, each with an address of its own. The page
   * is one scroll now, so a reader who lands on it cold gets everything at once - and
   * the pane addresses answer 404 rather than opening the page and pretending the link
   * meant what it says.
   */
  it('reads a repository back cold, whole', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      // Never having seen the list: the page has to resolve the repository by the
      // name in the address rather than find it in rows it already holds.
      await page.goto(`${panel.origin}/workspace/${panel.account}/repositories/data-pipeline`, {
        waitUntil: 'domcontentloaded',
      });
      await page
        .getByRole('heading', { name: 'data-pipeline', exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });

      // Every card, on the one page, with nothing to press to reach them.
      for (const card of ['Repository control', 'Merging', 'Behavior', 'Commands']) {
        await page
          .getByRole('heading', { name: card, exact: true })
          .first()
          .waitFor({ state: 'visible' });
      }
      expect(await page.locator('.pane-tools').count()).toBe(0);
      expect(new URL(page.url()).pathname).toBe(
        `/workspace/${panel.account}/repositories/data-pipeline`,
      );
    } finally {
      await page.close();
    }
  });

  it('refuses the pane addresses it used to have', async () => {
    const page = await panel.browser.newPage();
    try {
      // Answered from the wire, which is what a shared link actually hits.
      const answered = await page.goto(
        `${panel.origin}/workspace/${panel.account}/repositories/data-pipeline/commands`,
        { waitUntil: 'domcontentloaded' },
      );

      expect(answered?.status()).toBe(404);
    } finally {
      await page.close();
    }
  });

  /**
   * Both ways out, because they are two different mechanisms and only one of
   * them was ever going to be tried by hand.
   *
   * The navigation still reads Repositories while a repository is open, since
   * the page is a place inside that view. That made the item inert: "already on
   * this view" was true, so pressing it did nothing on exactly the screen where
   * a reader most expects it to do something.
   */
  it.each([
    ['the way back above the title', '.pane-path a.crumb'],
    ['the navigation item for the view it is in', 'nav a[href$="/repositories"]'],
  ])('leaves for the list through %s', async (_name, selector) => {
    const page = await openList();
    try {
      const row = page.locator('.repository-row').first();
      const name = (await row.locator('.object-name').textContent())?.trim() ?? '';
      await row.locator('a.row-hit').click();
      await page.getByRole('heading', { name, exact: true }).waitFor({ state: 'visible' });

      await page.locator(selector).first().click();

      await page.locator('.repository-row').first().waitFor({ state: 'visible', timeout: 15_000 });
      expect(new URL(page.url()).pathname).toBe(`/workspace/${panel.account}/repositories`);
    } finally {
      await page.close();
    }
  });
});
