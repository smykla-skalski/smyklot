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

/** One piece of queued work, in the group the queue files it under. */
const ROW = '.general-queue .object-row';

/**
 * Press one of the queue's five views.
 *
 * Each segment is a radio under a label, and the label is what covers it - so the label
 * is what a pointer reaches, here as everywhere else the pattern is pressed.
 */
async function showQueue(page: Page, view: string): Promise<void> {
  await page.getByRole('radio', { name: view }).locator('xpath=ancestor::label[1]').click();
}

async function installMotionRecorder(page: Page): Promise<void> {
  await page.evaluate(() => {
    const original = Element.prototype.animate;
    document.documentElement.setAttribute('data-queue-motion', '[]');
    Element.prototype.animate = function (keyframes, options): Animation {
      if (this.closest('.general-queue') !== null) {
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
    visit(viewer, `${panel.origin}/root/queue`, { ready: ROW }),
    visit(actor, `${panel.origin}/root/queue`, { ready: ROW }),
    visit(overview, `${panel.origin}/root`, { ready: '[data-queue-item]' }),
  ]);
  await installMotionRecorder(viewer);
}, 300_000);

afterAll(async () => {
  await viewer?.close();
  await actor?.close();
  await overview?.close();
  await panel?.close();
});

describe('the general Queue live stream [Integration]', () => {
  it('shows general durable work in Root Overview', async () => {
    const summary = overview.locator('.card', {
      has: overview.getByRole('heading', { name: 'Queue', exact: true }),
    });
    await summary.getByText('Apply organization sync plan', { exact: true }).waitFor();
    await summary.getByText('Scan for new commands', { exact: true }).waitFor();
    /* The queue's own sentence, said the same way here as on the queue page: a wait,
       then when the work runs. The console used to name the state instead. */
    await summary.getByText(/GitHub rate limit; retry scheduled · tries again/).waitFor();
    expect(await summary.locator('[data-queue-item]').count()).toBe(3);
  });

  it('renders each Queue view before arming live motion', async () => {
    await viewer.evaluate(() => document.documentElement.setAttribute('data-queue-motion', '[]'));
    await showQueue(viewer, 'Needs a decision');
    await expect.poll(() => new URL(viewer.url()).pathname).toBe('/root/queue/approvals');
    await viewer.getByText('Review organization sync plan').waitFor({ state: 'visible' });
    await viewer.waitForTimeout(200);
    expect((await recordedMotion(viewer)).some((animation) => animation.duration > 0)).toBe(false);

    await showQueue(viewer, 'Done');
    await expect.poll(() => new URL(viewer.url()).pathname).toBe('/root/queue/history');
    await viewer.getByText('Refresh the list of repositories', { exact: true }).waitFor();

    await viewer.evaluate(() => document.documentElement.setAttribute('data-queue-motion', '[]'));
    await showQueue(viewer, 'All');
    await expect.poll(() => new URL(viewer.url()).pathname).toBe('/root/queue');
    await viewer.getByText('Apply organization sync plan', { exact: true }).waitFor();
    await viewer.waitForTimeout(200);
    expect((await recordedMotion(viewer)).some((animation) => animation.duration > 0)).toBe(false);
  });

  it('refreshes another reader when an audited action changes an item', async () => {
    const title = 'Merge after CI';
    const viewerRow = viewer.locator(ROW, { hasText: title });
    await viewerRow.waitFor({ state: 'visible' });
    // Normal is the priority a row says nothing about, so the standing to wait for is its absence.
    expect(await viewerRow.getByText('High', { exact: true }).count()).toBe(0);

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
    await viewerRow
      .getByText('High', { exact: true })
      .waitFor({ state: 'visible', timeout: 5_000 });
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

  it('reaches both readers when an item becomes runnable, and animates the queue', async () => {
    const viewerRow = viewer.locator(ROW, { hasText: 'Scan for new commands' });
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
    /* The queue says a state in the words a reader owns, so a row that becomes runnable
       says when it runs rather than naming the state it is in - and the console's
       overview says it in the same words, because both read one sentence. The row that
       was retrying after a rate limit stops saying so in both places at once. */
    await Promise.all([
      viewerRow.getByText(/ · runs /).waitFor({ timeout: 5_000 }),
      overviewRow.getByText(/ · runs /).waitFor({ timeout: 5_000 }),
    ]);
    expect(await overviewRow.getByText(/GitHub rate limit/).count()).toBe(0);

    /* Motion is the queue's, and only the queue's. The overview is a summary somebody
       glances at on their way somewhere, and rows that slide and fade there animate
       whenever any workspace's work moves - which is constantly, and none of it is
       what the reader came for. */
    await waitForFastMotion(viewer, 150);
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
    const settled = viewer.locator(ROW, { hasText: 'Merge after CI' });
    await expect
      .poll(() => settled.getByText('High', { exact: true }).count(), { timeout: 5_000 })
      .toBe(0);

    const motion = await recordedMotion(viewer);
    expect(
      motion.every((animation) => animation.duration === 0),
      `reduced motion ran: ${JSON.stringify(motion)}`,
    ).toBe(true);
  });
});
