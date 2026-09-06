import type { Locator, Route } from 'playwright-core';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;

async function expectSharedModalControls(dialog: Locator): Promise<void> {
  const geometry = await dialog.evaluate((node) => {
    const durations = Array.from(node.querySelectorAll('.duration-field')).map((field) => {
      const box = field.getBoundingClientRect();
      const controls = field.querySelector('.duration-controls')!.getBoundingClientRect();
      const parent = field.closest('.form-field')!;
      const label = parent.querySelector('.form-label')!;
      const labelBox = label.getBoundingClientRect();
      const input = field.querySelector('input')!;
      const select = field.querySelector('select')!;
      const reference = document.createElement('input');
      reference.className = 'text-input';
      parent.append(reference);
      const paint = (element: Element) => {
        const style = getComputedStyle(element);
        return [
          style.backgroundColor,
          style.borderTopColor,
          style.borderTopWidth,
          style.borderRadius,
          style.fontSize,
          style.height,
        ];
      };
      const result = {
        width: box.width,
        controlsWidth: controls.width,
        x: controls.x,
        labelX: labelBox.x,
        gap: controls.y - labelBox.bottom,
        labelSize: getComputedStyle(label).fontSize,
        expectedLabelSize: getComputedStyle(reference).fontSize,
        labelWeight: getComputedStyle(label).fontWeight,
        input: paint(input),
        reference: paint(reference),
        inputHeight: input.getBoundingClientRect().height,
        selectHeight: select.getBoundingClientRect().height,
      };
      reference.remove();
      return result;
    });
    return {
      durations,
      nativeChecks: Array.from(node.querySelectorAll('input[type="checkbox"]')).filter(
        (input) => !input.closest('.switch,.check-item'),
      ).length,
      nativeSelects: Array.from(node.querySelectorAll('select')).filter(
        (select) => !select.closest('.select-wrap'),
      ).length,
      overflow: node.scrollWidth - node.clientWidth,
    };
  });
  expect(geometry.durations.length).toBeGreaterThan(0);
  for (const field of geometry.durations) {
    expect(field.width).toBeCloseTo(field.controlsWidth, 1);
    expect(field.x).toBeCloseTo(field.labelX, 1);
    expect(field.gap).toBeCloseTo(8, 1);
    expect(field.labelSize).toBe(field.expectedLabelSize);
    expect(field.labelWeight).toBe('600');
    expect(field.input).toEqual(field.reference);
    expect(field.inputHeight).toBe(34);
    expect(field.selectHeight).toBe(34);
  }
  expect(geometry.nativeChecks).toBe(0);
  expect(geometry.nativeSelects).toBe(0);
  expect(geometry.overflow).toBeLessThanOrEqual(1);
}

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

describe('background work schedules [Integration]', () => {
  it.each([
    { colorScheme: 'light', width: 1440 },
    { colorScheme: 'dark', width: 1440 },
    { colorScheme: 'light', width: 375 },
    { colorScheme: 'dark', width: 375 },
  ] as const)(
    'recognizes configuration file sync in service and workspace timing at $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({ colorScheme, viewport: { width, height: 1000 } });
      page.setDefaultTimeout(10_000);
      try {
        await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });
        await page.getByRole('button', { name: /Show all \d+ jobs/ }).click();
        const job = page.locator('.object-row', {
          has: page.getByText('Configuration file sync', { exact: true }),
        });
        await job.getByText(/every 15 minutes around the clock/).waitFor();
        await job
          .getByRole('button', { name: 'Edit schedule - Configuration file sync', exact: true })
          .click();
        const editor = page.getByRole('dialog', { name: 'Configure job', exact: true });
        await editor.waitFor();
        expect(
          await editor.getByRole('textbox', { name: 'How often', exact: true }).inputValue(),
        ).toBe('15');
        await editor.getByText(/Checks connected configuration files for changes/).waitFor();
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await editor.screenshot({
            path: join(directory, `service-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        }
        await expectSharedModalControls(editor);
        const overflow = await editor.evaluate(
          (element) => element.scrollWidth - element.clientWidth,
        );
        expect(overflow).toBeLessThanOrEqual(1);
        await page.keyboard.press('Escape');

        let submitted: Record<string, unknown> | undefined;
        await page.route('**/api/v1/targets/*/schedule-requests', async (route) => {
          if (route.request().method() !== 'POST') {
            await route.continue();
            return;
          }
          submitted = route.request().postDataJSON() as Record<string, unknown>;
          await route.fulfill({ status: 200, json: {} });
        });
        await visit(page, addressOf(panel, 'workspace/settings'), { ready: '#ws-timing' });
        const timing = page.locator('#ws-timing');
        await timing.locator('summary').click();
        await timing.getByRole('button', { name: 'Request a change' }).click();
        const request = page.getByRole('dialog', { name: 'Request a change to when Smyklot acts' });
        await request.getByLabel('Job', { exact: true }).selectOption('config_file_sync');
        await request.getByText(/Checks connected configuration files for changes/).waitFor();
        expect(
          await request.getByRole('textbox', { name: 'How often', exact: true }).inputValue(),
        ).toBe('15');
        await request.getByRole('textbox', { name: 'How often', exact: true }).fill('20');
        await request.getByLabel('Reason').fill('Check missed file changes every twenty minutes');
        if (directory)
          await request.screenshot({
            path: join(directory, `workspace-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        expect(
          await request.evaluate((element) => element.scrollWidth - element.clientWidth),
        ).toBeLessThanOrEqual(1);
        await expectSharedModalControls(request);
        await request.getByRole('button', { name: 'Send request' }).click();
        await expect
          .poll(() => submitted)
          .toEqual(
            expect.objectContaining({
              kind: 'config_file_sync',
              cadence_seconds: 1200,
              base_revision: 1,
              default_priority: 'normal',
            }),
          );
      } finally {
        await page.close();
      }
    },
  );

  it('announces the initial schedule load until every response arrives', async () => {
    const page = await panel.browser.newPage();
    let releaseResponse = (): void => {};
    const heldResponse = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });
    try {
      await page.route('**/api/v1/root/job-policies', async (route) => {
        await heldResponse;
        await route.continue();
      });
      await page.goto(addressOf(panel, 'root/schedules'), { waitUntil: 'domcontentloaded' });

      const view = page.locator('.view-frame[aria-busy]');
      await view.waitFor();
      await expect.poll(() => view.getAttribute('aria-busy')).toBe('true');

      releaseResponse();
      await expect.poll(() => view.getAttribute('aria-busy')).toBe('false');
      await view.locator('.object-row').first().waitFor();
    } finally {
      releaseResponse();
      await page.close();
    }
  });

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

      await visit(page, addressOf(panel, 'root/schedules'), {
        ready: '.view-frame .object-row',
      });
      await page.getByRole('heading', { name: 'Schedules', level: 1 }).waitFor();
    } finally {
      await page.close();
    }
  });

  /**
   * A job is a sentence, and the page opens on the four that ran most recently rather
   * than on all twelve: a console opens on what is happening. What each card owes a
   * reader is checked by its words, because that is the whole of what changed - a
   * cadence is said in human units and the hours are said as a week.
   */
  it('says what every job does, how often, and in whose hours', async () => {
    const page = await panel.browser.newPage();
    try {
      await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });

      await page.getByRole('heading', { name: 'Schedules', level: 1 }).waitFor();

      const jobs = page.locator('.card', { has: page.getByRole('heading', { name: 'Jobs' }) });
      await expect.poll(() => jobs.locator('.object-row').count()).toBe(4);
      await jobs.getByText('Showing 4 of 12 jobs', { exact: false }).waitFor();
      // The cadence in words, and the hours the job runs in - never 21600 seconds.
      await jobs
        .getByText(/every 5 minutes around the clock/)
        .first()
        .waitFor();

      await jobs.getByRole('button', { name: 'Show all 12 jobs' }).click();
      await expect.poll(() => jobs.locator('.object-row').count()).toBe(12);

      const hours = page.locator('.card', { has: page.getByRole('heading', { name: 'Hours' }) });
      await hours.getByText('Always Open', { exact: true }).waitFor();
      await hours.getByText(/Europe\/Warsaw · Mon to Fri/).waitFor();

      // A request waiting on somebody leads the page, worded as the ask it is.
      const decide = page.locator('.card', {
        has: page.getByRole('heading', { name: 'Needs a decision' }),
      });
      await decide.getByText(/asks: File indexing every 30 minutes/).waitFor();
      await decide
        .getByText('Refresh which paths are watched during the release preparation window', {
          exact: false,
        })
        .waitFor();
      await decide.getByRole('button', { name: 'Approve' }).waitFor();
      await decide.getByRole('button', { name: 'Decline' }).waitFor();
    } finally {
      await page.close();
    }
  });

  /**
   * A workspace has no Schedules page any more: timing is the service's to set, so what a
   * workspace gets is one row on its settings page saying when Smyklot acts and the way to
   * ask for that to change. This walks the row, opens the ask and sends it.
   */
  it('gives a workspace when Smyklot acts, and the way to ask for a change', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, addressOf(panel, 'workspace/settings'), { ready: '#ws-timing' });

      const timing = page.locator('#ws-timing');
      await timing.locator('summary').click();
      await timing.getByText('When Smyklot acts', { exact: true }).waitFor();
      // Read from the windows, not from a name: the fixture's policies name two profiles.
      await timing.locator('.setting-fact').waitFor();

      const dialog = page.locator('#workspace-timing-request');
      await timing.getByRole('button', { name: 'Request a change' }).click();
      await dialog.waitFor({ state: 'visible' });

      const send = dialog.getByRole('button', { name: 'Send request' });
      await expect.poll(() => send.isDisabled()).toBe(true);
      await dialog.getByLabel('Reason').fill('Keep the sync inside the release window');
      await expect.poll(() => send.isEnabled()).toBe(true);

      await send.click();
      await timing.getByText('A change to', { exact: false }).first().waitFor();
      await page.getByText('Sent to the operators for a decision').waitFor();
    } finally {
      await page.close();
    }
  });

  it('keeps every schedule row inside a phone viewport', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 390, height: 844 } });
    try {
      await visit(page, addressOf(panel, 'root/schedules'), { ready: '.view-frame .object-row' });

      const overflow = await page.evaluate(() => {
        const rows = [...document.querySelectorAll<HTMLElement>('.object-row')];
        if (rows.length === 0) return Number.POSITIVE_INFINITY;
        const width = document.documentElement.clientWidth;

        return Math.max(...rows.map((row) => row.getBoundingClientRect().right - width));
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
