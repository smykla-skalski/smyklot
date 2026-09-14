import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel({ liveSync: true });
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop sync against the live development lifecycle', () => {
  it.each(['light', 'dark'] as const)(
    'follows a real check to completion in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      try {
        // The seeded approved plan must finish before a fresh scan is requested.
        await expect
          .poll(
            async () => {
              const response = await page.request.get(
                `${panel.origin}/api/v1/targets/2001/sync/plan`,
              );
              return (await response.json()).plan;
            },
            { timeout: 15_000 },
          )
          .toBeNull();
        await visit(page, addressOf(panel, 'workspace/sync'));
        await page.getByRole('button', { name: 'Check now', exact: true }).click();
        const link = page.getByRole('link', { name: 'View request', exact: true });
        await link.waitFor();
        const href = await link.getAttribute('href');
        expect(href).toContain('/sync/request/check/');
        await link.click();
        await page.getByRole('dialog', { name: 'Sync request', exact: true }).waitFor();
        const inspector = page.getByRole('dialog');
        await inspector.getByRole('heading', { name: 'Check in progress', exact: true }).waitFor();
        const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
        const capture = async (scene: string) => {
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          await page.mouse.move(1900, 20);
          await page.screenshot({
            path: join(directory, `F33-live-check-${scene}-${colorScheme}.png`),
            fullPage: true,
          });
        };
        await capture('running');
        await inspector
          .getByRole('heading', { name: 'Check finished with gaps', exact: true })
          .waitFor();
        expect(await inspector.innerText()).toContain('found an open proposal');
        expect(await inspector.innerText()).toContain('could not proceed');
        expect(new URL(page.url()).pathname).toBe(href);
        await capture('complete');
        await page.reload();
        await inspector
          .getByRole('heading', { name: 'Check finished with gaps', exact: true })
          .waitFor();
        await inspector.locator('summary').filter({ hasText: 'Execution details' }).click();
        expect(await inspector.innerText()).toContain('Succeeded');
        expect(await inspector.innerText()).toContain('Attempt');
        expect(await inspector.innerText()).toContain('Finished');
        const results = inspector.getByRole('link', {
          name: 'View repository results',
          exact: true,
        });
        const resultHref = await results.getAttribute('href');
        await results.click();
        await inspector.getByRole('heading', { name: 'Repository check', exact: true }).waitFor();
        expect(new URL(page.url()).pathname).toBe(resultHref);
        await inspector
          .getByRole('heading', { name: 'Check finished with gaps', exact: true })
          .waitFor();
        await inspector.getByRole('button', { name: 'Close check', exact: true }).click();
        await page.waitForURL(/\/sync$/u);
        await page
          .getByRole('button', { name: 'platform-infra sync details', exact: true })
          .click();
        const detail = page.locator('.sync-repo-detail');
        expect(await detail.innerText()).toContain('Matched at last check');
        expect(await detail.innerText()).toContain('Pull request proposed');
        await capture('observations');
      } finally {
        await page.close();
      }
    },
  );
});
