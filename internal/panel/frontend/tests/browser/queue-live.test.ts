import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

let panel: Panel;
let viewer: Page;
let actor: Page;
let overview: Page;

interface QueueMotion {
  className: string;
  duration: number;
}

async function installMotionRecorder(page: Page): Promise<void> {
  await page.evaluate(() => {
    const original = Element.prototype.animate;
    document.documentElement.setAttribute('data-queue-motion', '[]');
    Element.prototype.animate = function (keyframes, options): Animation {
      if (this.closest('.general-queue-table, .queue-panel') !== null) {
        const raw = document.documentElement.getAttribute('data-queue-motion') ?? '[]';
        const records = JSON.parse(raw) as QueueMotion[];
        records.push({
          className: this.getAttribute('class') ?? '',
          duration: typeof options === 'number' ? options : Number(options?.duration ?? 0),
        });
        document.documentElement.setAttribute('data-queue-motion', JSON.stringify(records));
      }
      return original.call(this, keyframes, options);
    };
  });
}

async function recordedMotion(page: Page): Promise<QueueMotion[]> {
  return page.evaluate(() =>
    JSON.parse(document.documentElement.getAttribute('data-queue-motion') ?? '[]'),
  );
}

async function waitForFastMotion(page: Page, maximumDuration: number): Promise<void> {
  await expect
    .poll(
      async () =>
        (await recordedMotion(page)).some(
          (animation) => animation.duration > 0 && animation.duration <= maximumDuration,
        ),
      { interval: 10, timeout: 1_000 },
    )
    .toBe(true);
}

beforeAll(async () => {
  panel = await startPanel();
  viewer = await panel.browser.newPage();
  actor = await panel.browser.newPage();
  overview = await panel.browser.newPage();
  await Promise.all([
    visit(viewer, `${panel.origin}/root/queue`, { ready: '.general-queue-table tbody .data-row' }),
    visit(actor, `${panel.origin}/root/queue`, { ready: '.general-queue-table tbody .data-row' }),
    visit(overview, `${panel.origin}/root`, { ready: '.queue-panel [data-queue-item]' }),
  ]);
  await Promise.all([installMotionRecorder(viewer), installMotionRecorder(overview)]);
}, 300_000);

afterAll(async () => {
  await viewer?.close();
  await actor?.close();
  await overview?.close();
  await panel?.close();
});

describe('the general Queue live stream [Integration]', () => {
  it('shows general durable work in Root Overview', async () => {
    const summary = overview.locator('.queue-panel');
    await summary.getByText('3 active · 1 awaiting approval').waitFor();
    await summary.getByText('Apply organization sync plan', { exact: true }).waitFor();
    await summary.getByText('Discover pull request reactions', { exact: true }).waitFor();
    expect(await summary.locator('[data-queue-item]').count()).toBe(3);
  });

  it('renders each Queue view before arming live motion', async () => {
    await viewer.evaluate(() => document.documentElement.setAttribute('data-queue-motion', '[]'));
    await viewer.getByRole('link', { name: 'Approvals', exact: true }).click();
    await expect.poll(() => new URL(viewer.url()).pathname).toBe('/root/queue/approvals');
    await viewer.getByText('Review organization sync plan').waitFor({ state: 'visible' });
    await viewer.waitForTimeout(200);

    const spacing = await viewer.evaluate(() => {
      const heading = document.querySelector('.general-queue-table thead tr');
      const row = document.querySelector('.general-queue-table tbody .data-row');
      if (!(heading instanceof HTMLElement) || !(row instanceof HTMLElement)) return null;
      return row.getBoundingClientRect().top - heading.getBoundingClientRect().bottom;
    });
    expect(spacing).not.toBeNull();
    expect(spacing ?? Number.POSITIVE_INFINITY).toBeLessThanOrEqual(1);
    expect(await trailingTableSpace(viewer)).toBeLessThanOrEqual(1);
    expect((await recordedMotion(viewer)).some((animation) => animation.duration > 0)).toBe(false);

    await viewer.locator('a[href="/root/queue/history"]').click();
    await expect.poll(() => new URL(viewer.url()).pathname).toBe('/root/queue/history');
    await viewer.getByText('Refresh installation catalog', { exact: true }).waitFor();
    expect(await trailingTableSpace(viewer)).toBeLessThanOrEqual(1);

    await viewer.evaluate(() => document.documentElement.setAttribute('data-queue-motion', '[]'));
    await viewer.getByRole('link', { name: 'Active', exact: true }).click();
    await expect.poll(() => new URL(viewer.url()).pathname).toBe('/root/queue');
    await viewer.getByText('Apply organization sync plan', { exact: true }).waitFor();
    await viewer.waitForTimeout(200);
    expect((await recordedMotion(viewer)).some((animation) => animation.duration > 0)).toBe(false);
  });

  it('refreshes another reader when an audited action changes an item', async () => {
    const title = 'Merge platform-infra#184 after CI';
    const viewerRow = viewer.locator('.general-queue-table tbody .data-row', { hasText: title });
    await viewerRow.locator('.priority-normal').waitFor({ state: 'visible' });

    const status = await actor.evaluate(async () => {
      const response = await fetch('/api/v1/root/queue/queue-pending-ci/actions', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          type: 'set_priority',
          priority: 'high',
          expected_revision: 1,
        }),
      });
      return response.status;
    });
    expect(status).toBe(200);
    await viewerRow.locator('.priority-high').waitFor({ state: 'visible', timeout: 5_000 });
    await waitForFastMotion(viewer, 150);
  });

  it('rejects an action based on a stale item revision', async () => {
    const status = await actor.evaluate(async () => {
      const response = await fetch('/api/v1/root/queue/queue-pending-ci/actions', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          type: 'run_now',
          reason: 'incident response',
          expected_revision: 1,
        }),
      });
      return response.status;
    });
    expect(status).toBe(409);
  });

  it('animates state changes in Queue and Root Overview', async () => {
    const viewerRow = viewer.locator('.general-queue-table tbody .data-row', {
      hasText: 'Discover pull request reactions',
    });
    const overviewRow = overview.locator('[data-queue-item="queue-reaction-retry"]');

    const status = await actor.evaluate(async () => {
      const response = await fetch('/api/v1/root/queue/queue-reaction-retry/actions', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          type: 'run_now',
          reason: 'operator requested an immediate retry',
          expected_revision: 1,
        }),
      });
      return response.status;
    });
    expect(status).toBe(200);
    await Promise.all([
      viewerRow.getByText('Ready', { exact: true }).waitFor({ timeout: 5_000 }),
      overviewRow.getByText('Ready', { exact: true }).waitFor({ timeout: 5_000 }),
    ]);

    await Promise.all([waitForFastMotion(viewer, 150), waitForFastMotion(overview, 140)]);
  });

  it('removes queue motion when reduced motion is requested', async () => {
    await viewer.emulateMedia({ reducedMotion: 'reduce' });
    await viewer.waitForTimeout(100);
    await viewer.evaluate(() => document.documentElement.setAttribute('data-queue-motion', '[]'));

    const status = await actor.evaluate(async () => {
      const response = await fetch('/api/v1/root/queue/queue-pending-ci/actions', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          type: 'set_priority',
          priority: 'normal',
          expected_revision: 2,
        }),
      });
      return response.status;
    });
    expect(status).toBe(200);
    await viewer
      .locator('.general-queue-table tbody .data-row', {
        hasText: 'Merge platform-infra#184 after CI',
      })
      .locator('.priority-normal')
      .waitFor({ state: 'visible', timeout: 5_000 });

    const motion = await recordedMotion(viewer);
    expect(
      motion.every((animation) => animation.duration === 0),
      `reduced motion ran: ${JSON.stringify(motion)}`,
    ).toBe(true);
  });
});

async function trailingTableSpace(page: Page): Promise<number> {
  return page.evaluate(() => {
    const table = document.querySelector('.general-queue-table table');
    const row = document.querySelector('.general-queue-table tbody tr:last-child');
    if (!(table instanceof HTMLElement) || !(row instanceof HTMLElement)) {
      return Number.POSITIVE_INFINITY;
    }
    return table.getBoundingClientRect().bottom - row.getBoundingClientRect().bottom;
  });
}
