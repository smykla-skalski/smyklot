import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

describe('the general Queue table [Integration]', () => {
  it('states the durable-work columns and keeps them inside the wide console', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, addressOf(panel, 'root/queue'), {
        ready: '.general-queue-table tbody tr',
      });
      const reading = await page.evaluate(() => {
        const region = document.querySelector<HTMLElement>('.general-queue-table');
        const table = region?.querySelector('table');
        return {
          headings: [...document.querySelectorAll('.general-queue-table thead th')].map((cell) =>
            (cell.textContent ?? '').trim(),
          ),
          overflow:
            region === null || table === null || table === undefined
              ? Number.POSITIVE_INFINITY
              : table.getBoundingClientRect().right - region.getBoundingClientRect().right,
        };
      });
      expect(reading.headings).toEqual(['Work', 'Status', 'Timing', 'Actions']);
      expect(reading.overflow).toBeLessThanOrEqual(1);
      await page
        .getByText(/Always Open · .*UTC · est\./)
        .first()
        .waitFor();
    } finally {
      await page.close();
    }
  });

  it('turns each item into a labelled card on a phone', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 390, height: 844 } });
    try {
      await visit(page, addressOf(panel, 'root/queue'), {
        ready: '.general-queue-table tbody tr',
      });
      const rows = await page.evaluate(() =>
        [...document.querySelectorAll<HTMLElement>('.general-queue-table tbody .data-row')].map(
          (row) => ({
            right: row.getBoundingClientRect().right,
            viewport: document.documentElement.clientWidth,
            labels: [...row.querySelectorAll<HTMLElement>('[data-label]')].map(
              (cell) => cell.dataset.label,
            ),
          }),
        ),
      );
      expect(rows.length).toBeGreaterThan(0);
      for (const row of rows) {
        expect(row.right).toBeLessThanOrEqual(row.viewport + 1);
        expect(row.labels).toEqual(['Work', 'State', 'Timing', 'Actions']);
      }
    } finally {
      await page.close();
    }
  });

  it('moves approval and terminal work into their named views', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'root/queue'), {
        ready: '.general-queue-table tbody tr',
      });
      const views = page.getByRole('group', { name: 'Queue views' });
      await views.getByText('Approvals', { exact: true }).click();
      await expect.poll(() => page.locator('.general-queue-table tbody .data-row').count()).toBe(1);
      await page.getByText('Review organization sync plan').waitFor({ state: 'visible' });

      await views.getByText('History', { exact: true }).click();
      await page.getByText('Refresh installation catalog').waitFor({ state: 'visible' });
    } finally {
      await page.close();
    }
  });

  it('opens workload detail and the immutable transition timeline', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'root/queue'), {
        ready: '.general-queue-table tbody tr',
      });
      const row = page.locator('.general-queue-table tbody .data-row', {
        hasText: 'Apply organization sync plan',
      });
      await row.getByRole('button', { name: 'Apply organization sync plan', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'Apply organization sync plan' });
      await dialog.waitFor({ state: 'visible' });
      await dialog.getByRole('heading', { name: 'Workload detail' }).waitFor();
      await dialog.getByText('Viewer-local eligibility').waitFor();
      await dialog.getByText('Window-local eligibility').waitFor();
      await dialog.getByText('3 create · 7 update · 2 delete').waitFor();
      await dialog.getByRole('heading', { name: 'Timeline' }).waitFor();
    } finally {
      await page.close();
    }
  });
});
