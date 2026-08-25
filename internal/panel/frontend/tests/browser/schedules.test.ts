import type { Route } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

describe('background work schedules [Integration]', () => {
  it('renders empty policy overrides from older servers', async () => {
    const page = await panel.browser.newPage();
    const emptyOverrides = async (route: Route) => {
      const response = await route.fetch();
      const document = (await response.json()) as {
        policy_set?: { overrides?: unknown };
        policies?: { overrides?: unknown };
      };
      if (document.policy_set !== undefined) document.policy_set.overrides = null;
      if (document.policies !== undefined) document.policies.overrides = null;
      await route.fulfill({ response, json: document });
    };
    try {
      await page.route('**/api/v1/root/job-policies', emptyOverrides);
      await page.route('**/api/v1/targets/*/schedules', emptyOverrides);

      await visit(page, addressOf(panel, 'root/schedules'), {
        ready: '.schedules-view .policy-table-wrap tbody tr',
      });
      await page.getByRole('heading', { name: 'Schedules', level: 2 }).waitFor();

      await visit(page, addressOf(panel, 'i/schedules'), {
        ready: '.schedules-view .policy-table-wrap tbody tr',
      });
      await page.getByRole('heading', { name: 'Schedules', level: 2 }).waitFor();
    } finally {
      await page.close();
    }
  });

  it('shows Root the effective policies, named profiles, and pending decisions', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'root/schedules'), {
        ready: '.schedules-view .policy-table-wrap tbody tr',
      });

      await page.getByRole('heading', { name: 'Schedules', level: 2 }).waitFor();
      await expect
        .poll(() =>
          page.locator('.schedules-view .policy-table-wrap').first().locator('tbody tr').count(),
        )
        .toBe(11);
      await page
        .locator('.policy-source', { hasText: 'Global policy · revision' })
        .first()
        .waitFor();
      await page.getByText(/^Deployment 6h · Always Open · normal$/).waitFor();
      await page.getByRole('heading', { name: 'Profiles' }).waitFor();
      await page.locator('.profile-card', { hasText: 'Always Open' }).waitFor();
      await page.locator('.profile-card', { hasText: 'Europe business hours' }).waitFor();
      await page.getByRole('heading', { name: 'Schedule requests' }).waitFor();
      await page
        .getByText('Refresh repository paths during the release preparation window')
        .waitFor();
      await page.getByRole('button', { name: 'Approve' }).waitFor();
    } finally {
      await page.close();
    }
  });

  it('shows installation owners their effective schedule and request controls', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'i/schedules'), {
        ready: '.schedules-view .policy-table-wrap tbody tr',
      });

      await page.getByRole('heading', { name: 'Schedules', level: 2 }).waitFor();
      await expect
        .poll(() =>
          page.locator('.schedules-view .policy-table-wrap').first().locator('tbody tr').count(),
        )
        .toBe(6);
      await page
        .locator('.policy-source', { hasText: 'Global policy · revision' })
        .first()
        .waitFor();
      await page
        .locator('.policy-source', { hasText: 'Installation override · revision' })
        .waitFor();
      await page.getByRole('heading', { name: 'Request a recurring change' }).waitFor();
      await page.getByRole('button', { name: 'Send request' }).waitFor();
      await page.getByText('Europe business hours').first().waitFor();
      await page.getByRole('button', { name: 'Withdraw' }).waitFor();

      const form = page.locator('.request-form');
      await form.getByLabel('Workload').selectOption('pending_ci');
      await expect.poll(() => form.getByLabel('Cadence seconds').inputValue()).toBe('30');
      await expect.poll(() => form.getByLabel('Priority').inputValue()).toBe('normal');
      await expect.poll(() => form.getByLabel('Window profile').inputValue()).toBe('always-open');
    } finally {
      await page.close();
    }
  });

  it('keeps the schedule table inside a phone viewport', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 390, height: 844 } });
    try {
      await visit(page, addressOf(panel, 'root/schedules'), {
        ready: '.schedules-view .policy-table-wrap tbody tr',
      });

      const overflow = await page.evaluate(() => {
        const region = document.querySelector<HTMLElement>('.policy-table-wrap');
        if (region === null) return Number.POSITIVE_INFINITY;
        return region.getBoundingClientRect().right - document.documentElement.clientWidth;
      });
      expect(overflow).toBeLessThanOrEqual(1);

      const scroll = await page.evaluate(() => {
        const before = window.scrollY;
        window.scrollTo({ top: document.documentElement.scrollHeight });
        return { before, after: window.scrollY };
      });
      expect(scroll.after).toBeGreaterThan(scroll.before);
    } finally {
      await page.close();
    }
  });
});
