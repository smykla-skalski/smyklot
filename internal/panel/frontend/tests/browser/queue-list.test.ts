import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;

const READY = '.general-queue .object-row';

/** Press one of the queue's five views: a radio under the label that covers it. */
async function show(page: Page, view: string): Promise<void> {
  await page.getByRole('radio', { name: view }).locator('xpath=ancestor::label[1]').click();
}

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

describe('the general Queue list [Integration]', () => {
  /**
   * The queue answers three questions in the order a reader has them: what needs them,
   * what the service is doing on its own, and what it already did. The groups are what
   * says so, and a row that reads as a sentence is what makes them worth grouping.
   */
  it('groups work by what can be done about it and keeps it inside the wide console', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, addressOf(panel, 'root/queue'), { ready: READY });
      const reading = await page.evaluate(() => {
        const region = document.querySelector<HTMLElement>('.general-queue');
        const rows = [...document.querySelectorAll<HTMLElement>('.general-queue .object-row')];
        return {
          groups: [...document.querySelectorAll('.general-queue .card-title')].map((title) =>
            (title.textContent ?? '').trim(),
          ),
          overflow:
            region === null
              ? Number.POSITIVE_INFINITY
              : Math.max(
                  0,
                  ...rows.map(
                    (row) =>
                      row.getBoundingClientRect().right - region.getBoundingClientRect().right,
                  ),
                ),
          sentences: rows.map((row) =>
            (row.querySelector('.object-sum')?.textContent ?? '').trim(),
          ),
        };
      });
      /* What is done is part of what is happening: the view that shows everything
         carries the last day of it, under a heading that says so rather than claiming
         to be the whole record. */
      expect(reading.groups).toEqual([
        'Needs a decision',
        'Running and waiting',
        'Done in the last day',
      ]);
      expect(reading.overflow).toBeLessThanOrEqual(1);
      expect(reading.sentences).toContain('Running · 4 of 12 changes written');
      expect(reading.sentences.every((sentence) => sentence !== '')).toBe(true);
    } finally {
      await page.close();
    }
  });

  it('keeps every row inside a phone', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 390, height: 844 } });
    try {
      await visit(page, addressOf(panel, 'root/queue'), { ready: READY });
      const rows = await page.evaluate(() =>
        [...document.querySelectorAll<HTMLElement>('.general-queue .object-row')].map((row) => ({
          right: row.getBoundingClientRect().right,
          viewport: document.documentElement.clientWidth,
          name: (row.querySelector('.object-name')?.textContent ?? '').trim(),
          sentence: (row.querySelector('.object-sum')?.textContent ?? '').trim(),
        })),
      );
      expect(rows.length).toBeGreaterThan(0);
      for (const row of rows) {
        expect(row.right).toBeLessThanOrEqual(row.viewport + 1);
        expect(row.name).not.toBe('');
        expect(row.sentence).not.toBe('');
      }
    } finally {
      await page.close();
    }
  });

  it('narrows the whole page to the words a reader types', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'root/queue'), { ready: READY });
      await page.getByRole('searchbox', { name: 'Search the queue' }).fill('commands');
      await expect.poll(() => page.locator(READY).count()).toBe(1);
      await page.getByText('Scan for new commands').waitFor({ state: 'visible' });
      /* A card with nothing left in it is not a card: the groups are what the page
         holds, so a search that empties one takes the heading with it. */
      expect(await page.locator('.general-queue .card-title').allTextContents()).toEqual([
        'Running and waiting',
      ]);
    } finally {
      await page.close();
    }
  });

  it('moves approval and terminal work into their named views', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'root/queue'), { ready: READY });
      await show(page, 'Needs a decision');
      await expect.poll(() => new URL(page.url()).pathname).toBe('/root/queue/approvals');
      await expect.poll(() => page.locator(READY).count()).toBe(1);
      await page.getByText('Review organization sync plan').waitFor({ state: 'visible' });

      await show(page, 'Done');
      await expect.poll(() => new URL(page.url()).pathname).toBe('/root/queue/history');
      await page.getByText('Refresh the list of repositories').waitFor({ state: 'visible' });
    } finally {
      await page.close();
    }
  });

  it('opens what a job is doing and the immutable transition timeline', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'root/queue'), { ready: READY });
      await page
        .getByRole('button', { name: 'Open Apply organization sync plan', exact: true })
        .click();
      const dialog = page.getByRole('dialog', { name: 'Apply organization sync plan' });
      await dialog.waitFor({ state: 'visible' });
      await dialog.getByRole('heading', { name: 'What this job is doing' }).waitFor();
      await dialog.getByText('Ready, in your timezone').waitFor();
      await dialog.getByText("Ready, in the job's timezone").waitFor();
      await dialog.getByText('3 create · 7 update · 2 delete').waitFor();
      await dialog.getByRole('heading', { name: 'Timeline' }).waitFor();
    } finally {
      await page.close();
    }
  });
});
