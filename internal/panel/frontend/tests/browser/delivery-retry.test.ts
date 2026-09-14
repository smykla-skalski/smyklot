import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';
import type { QueueDetail, QueueItem } from '../../src/lib/types';
import {
  mockRecoveryOperation,
  mockRecoveryPreview,
  mockRecoverDelivery,
} from '../../dev/delivery-recovery';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

/** Each desktop case owns its execution history while using the real mock records and API contract. */
async function isolateRecovery(
  page: Page,
  loseFirstResponse = false,
  requests: unknown[] = [],
  beforeReply?: () => Promise<void>,
) {
  const queue: QueueItem[] = [];
  let original: QueueDetail | undefined;
  await page.route('**/queue/*', async (route) => {
    if (route.request().method() !== 'GET') return route.continue();
    const id = decodeURIComponent(new URL(route.request().url()).pathname.split('/').at(-1)!);
    if (!original) {
      const response = await route.fetch();
      original = (await response.json()) as QueueDetail;
      queue.push(structuredClone(original.item));
    }
    const item = queue.find((entry) => entry.id === id);
    if (!item) return route.continue();
    const delivery = mockRecoveryOperation(queue, item);
    // Pending navigation must be guarded even when a prior successor is already visible.
    if (beforeReply && queue.length === 1 && delivery?.current?.queue) {
      delivery.current.queue.id = 'delivery:prior-successor';
    }
    await route.fulfill({ json: { ...original, item, delivery } });
  });
  await page.route('**/deliveries/*/recovery', async (route) => {
    const segments = new URL(route.request().url()).pathname.split('/');
    const source = decodeURIComponent(segments.at(-2)!);
    const target = decodeURIComponent(segments.at(-4)!);
    if (route.request().method() === 'GET') {
      await route.fulfill({ json: mockRecoveryPreview(queue, target, source) });
    } else {
      requests.push(route.request().postDataJSON());
      const result = mockRecoverDelivery(queue, target, source, route.request().postDataJSON());
      await beforeReply?.();
      if (loseFirstResponse && requests.length === 1) return route.abort('failed');
      await route.fulfill({ status: result.status, json: result.body });
    }
  });
  return queue;
}

describe('desktop delivery retry', () => {
  it.each(
    [
      'root',
      'root/history/failures',
      'workspace/history/failures',
      'root/workspaces/{account}/history/failures',
    ].flatMap((route) =>
      (['light', 'dark'] as const).map((colorScheme) => ({ route, colorScheme })),
    ),
  )('reviews and starts retry at $route in $colorScheme', async ({ route, colorScheme }) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    await isolateRecovery(page);
    try {
      await visit(page, addressOf(panel, route));
      await page.getByRole('button', { name: 'Inspect queue item' }).first().click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('button', { name: 'Review retry' }).click();
      await dialog.getByRole('button', { name: 'Start retry' }).waitFor();
      expect(await dialog.innerText()).toContain('The original failure stays in history');
      await dialog.getByRole('button', { name: 'Start retry' }).click();
      await expect.poll(() => dialog.locator('dd').allTextContents()).toContain('Scheduled');
      await page.keyboard.press('Escape');
      await page.getByRole('button', { name: 'Inspect queue item' }).first().click();
      await expect.poll(() => dialog.locator('dd').allTextContents()).toContain('Failed');
      await dialog.getByRole('button', { name: 'Inspect latest run' }).waitFor();
    } finally {
      await page.close();
    }
  });

  it('retains the accepted request after a lost response and dismissal', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    page.setDefaultTimeout(5000);
    const requests: unknown[] = [];
    await isolateRecovery(page, true, requests);
    try {
      await visit(page, addressOf(panel, 'root/history/failures'));
      await page.getByRole('button', { name: 'Inspect queue item' }).first().click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('button', { name: 'Review retry' }).click();
      await dialog.getByRole('button', { name: 'Start retry' }).click();
      await dialog.getByRole('button', { name: 'Check retry result' }).waitFor();
      await expect.poll(() => dialog.innerText()).toContain('could not be confirmed');
      await page.keyboard.press('Escape');
      await page.getByRole('button', { name: 'Inspect queue item' }).first().click();
      await dialog.getByRole('button', { name: 'Check retry result' }).click();
      await expect.poll(() => dialog.locator('dd').allTextContents()).toContain('Scheduled');
      expect(requests).toHaveLength(2);
      expect(requests[1]).toEqual(requests[0]);
    } finally {
      await page.close();
    }
  });

  it('keeps the explanation and inspector while submission is pending', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    page.setDefaultTimeout(5000);
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    await isolateRecovery(page, false, [], () => held);
    try {
      await visit(page, addressOf(panel, 'root/history/failures'));
      await page.getByRole('button', { name: 'Inspect queue item' }).first().click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('button', { name: 'Review retry' }).click();
      await dialog.getByRole('button', { name: 'Start retry' }).click();
      await dialog.getByRole('button', { name: 'Requesting retry…' }).waitFor();
      expect(await dialog.innerText()).toContain('The original failure stays in history');
      expect(
        await dialog.getByRole('button', { name: 'Inspect latest run', exact: true }).isDisabled(),
      ).toBe(true);
      expect(await dialog.getByRole('button', { name: 'Close', exact: true }).isDisabled()).toBe(
        true,
      );
      await page.keyboard.press('Escape');
      expect(await dialog.isVisible()).toBe(true);
      release();
      await expect.poll(() => dialog.locator('dd').allTextContents()).toContain('Scheduled');
      await page.keyboard.press('Escape');
      await dialog.waitFor({ state: 'hidden' });
    } finally {
      release();
      await page.close();
    }
  });

  it('requires another availability review after a definitive conflict', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1920, height: 1200 } });
    page.setDefaultTimeout(5000);
    await isolateRecovery(page);
    const requests: { request_key: string }[] = [];
    await page.route('**/deliveries/*/recovery', async (route) => {
      if (route.request().method() !== 'POST') return route.fallback();
      requests.push(route.request().postDataJSON());
      if (requests.length > 1) return route.fallback();
      await route.fulfill({
        status: 409,
        json: { error: { code: 'state_changed', message: 'Review the latest recovery state.' } },
      });
    });
    try {
      await visit(page, addressOf(panel, 'root/history/failures'));
      await page.getByRole('button', { name: 'Inspect queue item' }).first().click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('button', { name: 'Review retry' }).click();
      await dialog.getByRole('button', { name: 'Start retry' }).click();
      await expect.poll(() => dialog.innerText()).toContain('Review the latest recovery state.');
      expect(await dialog.getByRole('button', { name: 'Start retry' }).count()).toBe(0);
      expect(await dialog.getByRole('button', { name: 'Check retry result' }).count()).toBe(0);
      await dialog.getByRole('button', { name: 'Check availability' }).click();
      await dialog.getByRole('button', { name: 'Start retry' }).click();
      await expect.poll(() => dialog.locator('dd').allTextContents()).toContain('Scheduled');
      expect(requests).toHaveLength(2);
      expect(requests[1]?.request_key).not.toBe(requests[0]?.request_key);
    } finally {
      await page.close();
    }
  });

  it.each([
    {
      reason: 'configuration_invalid',
      message: 'Fix the configuration',
      colorScheme: 'light' as const,
    },
    {
      reason: 'configuration_disconnected',
      message: 'Connect the configuration file',
      colorScheme: 'light' as const,
    },
    {
      reason: 'configuration_disconnected',
      message: 'Connect the configuration file',
      colorScheme: 'dark' as const,
    },
  ])(
    'explains $reason in $colorScheme without offering submission',
    async ({ reason, message, colorScheme }) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      page.setDefaultTimeout(5000);
      await isolateRecovery(page);
      try {
        await page.route('**/deliveries/*/recovery', (route) =>
          route.fulfill({
            json: {
              available: false,
              reason,
              revision: 1,
              current_run_id: 1,
            },
          }),
        );
        await visit(page, addressOf(panel, 'root/history/failures'));
        await page.getByRole('button', { name: 'Inspect queue item' }).first().click();
        const dialog = page.getByRole('dialog');
        await dialog.getByRole('button', { name: 'Review retry' }).click();
        await expect.poll(() => dialog.innerText()).toContain(message);
        expect(await dialog.getByRole('button', { name: 'Start retry' }).count()).toBe(0);
      } finally {
        await page.close();
      }
    },
  );
});
