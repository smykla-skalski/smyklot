import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, type Panel } from './harness';
let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});
describe('desktop live sync read states', () => {
  it.each(['light', 'dark'] as const)(
    'distinguishes loading, failed reads and empty results in %s',
    async (theme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme: theme,
        reducedMotion: 'reduce',
      });
      let release = () => {};
      const firstRead = new Promise<void>((resolve) => {
        release = resolve;
      });
      try {
        let reads = 0;
        let fail = true;
        const retryGate: { wait?: Promise<void> } = {};
        let releaseRetry = () => {};
        let posts = 0;
        page.on('request', (request) => {
          if (request.method() === 'POST' && request.url().includes('/sync/')) posts++;
        });
        await page.route('**/api/v1/targets/2001/sync/plan', async (route) => {
          reads++;
          if (reads === 1) await firstRead;
          if (retryGate.wait) await retryGate.wait;
          await route.fulfill(
            fail
              ? { status: 503, json: { error: 'temporarily unavailable' } }
              : {
                  json: {
                    plan: null,
                    check: {
                      action: 'check',
                      target_id: '2001',
                      available: true,
                      reason: 'available',
                      effect: 'request_repository_check',
                    },
                  },
                },
          );
        });
        const url = addressOf(panel, 'workspace/sync/plan');
        await page.goto(url, { waitUntil: 'domcontentloaded' });
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        const empty = inspector.getByRole('heading', { name: 'No changes to review', exact: true });
        const capture = async (scene: string) => {
          const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({ path: join(directory, `F33-read-state-${scene}-${theme}.png`) });
        };
        await inspector.getByText('Loading sync result…', { exact: true }).waitFor();
        expect(await empty.count()).toBe(0);
        await capture('loading');
        release();
        await inspector.getByText('Changes could not be loaded', { exact: true }).waitFor();
        expect(await empty.count()).toBe(0);
        await capture('initial-failure');
        fail = false;
        await inspector.getByRole('button', { name: 'Try again', exact: true }).focus();
        await inspector.getByRole('button', { name: 'Try again', exact: true }).press('Enter');
        await empty.waitFor();
        await expect
          .poll(() => empty.evaluate((element) => element === document.activeElement))
          .toBe(true);
        expect(page.url()).toBe(url);
        await capture('recovered-empty');
        fail = true;
        await inspector.getByRole('button', { name: 'Refresh status', exact: true }).click();
        await inspector.getByText('Changes could not be loaded', { exact: true }).waitFor();
        expect(await empty.count()).toBe(0);
        expect(
          await inspector.getByRole('button', { name: 'Check repositories', exact: true }).count(),
        ).toBe(0);
        await capture('refresh-failure');
        fail = false;
        await inspector.getByRole('button', { name: 'Try again', exact: true }).focus();
        await inspector.getByRole('button', { name: 'Try again', exact: true }).press('Enter');
        await empty.waitFor();
        await expect
          .poll(() => empty.evaluate((element) => element === document.activeElement))
          .toBe(true);
        expect(page.url()).toBe(url);
        fail = true;
        await inspector.getByRole('button', { name: 'Refresh status', exact: true }).click();
        await inspector.getByText('Changes could not be loaded', { exact: true }).waitFor();
        fail = false;
        retryGate.wait = new Promise<void>((resolve) => {
          releaseRetry = resolve;
        });
        await inspector.getByRole('button', { name: 'Try again', exact: true }).click();
        await inspector.getByRole('button', { name: 'Trying again…', exact: true }).waitFor();
        const close = inspector.getByRole('button', { name: 'Close sync details', exact: true });
        await close.focus();
        releaseRetry();
        await empty.waitFor();
        expect(await close.evaluate((element) => element === document.activeElement)).toBe(true);
        await capture('focus-preserved');
        expect(posts).toBe(0);
      } finally {
        release();
        await page.close();
      }
    },
  );
});
