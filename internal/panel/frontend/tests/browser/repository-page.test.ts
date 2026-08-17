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
  await page.goto(`${panel.origin}/i/${panel.account}/repositories`, {
    waitUntil: 'domcontentloaded',
  });
  await page.locator('.repository-row').first().waitFor({ state: 'visible', timeout: 30_000 });
  /* A search stored from another sweep would leave a filtered list, and a row
     index means nothing then. */
  await page.getByPlaceholder('Search repositories').fill('');
  await page.locator('.repository-row').nth(1).waitFor({ state: 'visible' });

  return page;
}

describe('one repository as a page [Integration]', () => {
  it('opens from a press on the row, not only on its name', async () => {
    const page = await openList();
    try {
      const row = page.locator('.repository-row').first();
      const name = (await row.locator('.repo-copy strong').textContent())?.trim() ?? '';
      expect(name).not.toBe('');

      // The Updated cell: text, in the middle of the row, belonging to no control.
      await row.locator('td').nth(2).click();

      await page
        .getByRole('heading', { name, exact: true })
        .waitFor({ state: 'visible', timeout: 15_000 });
      expect(new URL(page.url()).pathname).toBe(
        `/i/${panel.account}/repositories/${encodeURIComponent(name)}`,
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
      const enabled = row.getByText('Enabled', { exact: true });
      const disabled = row.getByText('Disabled', { exact: true });
      const wasEnabled = await row.locator('input[type="radio"]').first().isChecked();

      // The word, which is the label - the press a reader makes, and the one
      // that used to open the repository as well as move the switch.
      await (wasEnabled ? disabled : enabled).click();
      await page.waitForTimeout(400);

      expect(new URL(page.url()).pathname, 'the row opened as well as switched').toBe(before);
      expect(await row.locator('input[type="radio"]').first().isChecked()).toBe(!wasEnabled);

      // Put it back, because the mock keeps this and every other sweep reads it.
      await (wasEnabled ? enabled : disabled).click();
      await page.waitForTimeout(400);
      expect(await row.locator('input[type="radio"]').first().isChecked()).toBe(wasEnabled);
    } finally {
      await page.close();
    }
  });

  it('opens from the keyboard, through the one link in the row', async () => {
    const page = await openList();
    try {
      const row = page.locator('.repository-row').first();
      const name = (await row.locator('.repo-copy strong').textContent())?.trim() ?? '';
      const link = row.locator('a.repo-copy');

      await link.focus();
      await page.keyboard.press('Enter');

      await page
        .getByRole('heading', { name, exact: true })
        .waitFor({ state: 'visible', timeout: 15_000 });
    } finally {
      await page.close();
    }
  });

  it('carries the open pane in the address, and reads one back cold', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      // Never having seen the list: the page has to resolve the repository by the
      // name in the address rather than find it in rows it already holds.
      await page.goto(`${panel.origin}/i/${panel.account}/repositories/data-pipeline/commands`, {
        waitUntil: 'domcontentloaded',
      });
      await page
        .getByRole('heading', { name: 'data-pipeline', exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });
      await page.getByText('Command overrides').waitFor({ state: 'visible' });

      /* The pane switch is a radio per option with its label drawn over it, so
         the radio itself is never what a pointer reaches - press the label, the
         way a reader does. */
      await page.locator('header').getByText('File', { exact: true }).click();
      await page.waitForFunction(() => !window.location.pathname.endsWith('/commands'), undefined, {
        timeout: 5_000,
      });
      // The File pane is where the page opens, so its address is the bare
      // repository - a section is written only when it is not that one.
      expect(new URL(page.url()).pathname).toBe(`/i/${panel.account}/repositories/data-pipeline`);
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
    ['the way back above the title', 'a.back-link'],
    ['the navigation item for the view it is in', 'nav a[href$="/repositories"]'],
  ])('leaves for the list through %s', async (_name, selector) => {
    const page = await openList();
    try {
      const row = page.locator('.repository-row').first();
      const name = (await row.locator('.repo-copy strong').textContent())?.trim() ?? '';
      await row.locator('a.repo-copy').click();
      await page.getByRole('heading', { name, exact: true }).waitFor({ state: 'visible' });

      await page.locator(selector).first().click();

      await page.locator('.repository-row').first().waitFor({ state: 'visible', timeout: 15_000 });
      expect(new URL(page.url()).pathname).toBe(`/i/${panel.account}/repositories`);
    } finally {
      await page.close();
    }
  });
});
