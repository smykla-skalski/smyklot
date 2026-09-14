import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, expect, it } from 'vitest';
import { startPanel, type Panel } from './harness';
let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});
it.each(['light', 'dark'] as const)(
  'explains queue occurrence actions in %s desktop views',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    const capture = async (scene: string) => {
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.screenshot({
        path: join(directory, `F12-${scene}-${colorScheme}.png`),
        animations: 'disabled',
      });
    };
    try {
      for (const scope of ['root', 'workspace/smykla-skalski']) {
        await page.goto(`${panel.origin}/${scope}/queue`);
        const row = page.locator('.object-row').filter({ hasText: 'Scan for new commands' });
        await row.getByRole('button', { name: 'Retry now', exact: true }).waitFor();
        const running = page
          .locator('.object-row')
          .filter({ hasText: 'Apply organization sync plan' });
        expect(await running.getByRole('button', { name: /^(Run|Retry|Cancel)/ }).count()).toBe(0);
        const finished = page
          .locator('.object-row')
          .filter({ hasText: 'Refresh the list of repositories' });
        expect(await finished.innerText()).toContain('Succeeded');
        expect(await finished.getByRole('button', { name: /^(Run|Retry|Cancel)/ }).count()).toBe(0);
        await capture(scope === 'root' ? 'root-active' : 'workspace-active');
        await row.getByRole('button', { name: 'Retry now', exact: true }).click();
        const dialog = page.getByRole('dialog', { name: 'Retry now' });
        await dialog.waitFor();
        expect(await dialog.innerText()).toContain('This does not create another occurrence');
        expect(await dialog.innerText()).toContain('capacity is available');
        await capture(scope === 'root' ? 'root-retry' : 'workspace-retry');
        await page.keyboard.press('Escape');
        await dialog.waitFor({ state: 'hidden' });
        await page
          .getByRole('button', { name: 'Open Refresh the list of repositories', exact: true })
          .click();
        const completed = page.getByRole('dialog', { name: 'Refresh the list of repositories' });
        await completed.getByText('Finished', { exact: true }).waitFor();
        await completed.getByText('Outcome', { exact: true }).waitFor();
        expect(await completed.getByText('Revision', { exact: true }).isVisible()).toBe(false);
        expect(await completed.getByText('What this job is doing', { exact: true }).count()).toBe(
          0,
        );
        expect(await completed.getByText('Estimated start', { exact: true }).count()).toBe(0);
        expect(await completed.getByText('Succeeded', { exact: true }).count()).toBeGreaterThan(0);
        await capture(scope === 'root' ? 'root-completed' : 'workspace-completed');
        await completed.getByText('Scheduling details', { exact: true }).click();
        await completed.getByText('Revision', { exact: true }).waitFor();
        await capture(scope === 'root' ? 'root-completed-details' : 'workspace-completed-details');
        await page.keyboard.press('Escape');
        await completed.waitFor({ state: 'hidden' });
      }
    } finally {
      await page.close();
    }
  },
);

it.each(['light', 'dark'] as const)(
  'applies run-now to the selected occurrence in %s desktop',
  async (colorScheme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme,
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(5000);
    const capture = async (scene: string) => {
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.screenshot({
        path: join(directory, `F12-${scene}-${colorScheme}.png`),
        animations: 'disabled',
      });
    };
    try {
      for (const scope of ['root', 'workspace/smykla-skalski']) {
        const prefix = scope === 'root' ? 'root' : 'workspace';
        await page.goto(`${panel.origin}/${scope}/queue`);
        const row = page.locator('.object-row').filter({ hasText: 'Scan for new commands' });
        const action = row.getByRole('button', { name: /^(Retry now|Run next occurrence now)$/ });
        await action.waitFor();
        const label = (await action.innerText()).trim();
        await action.click();
        const dialog = page.getByRole('dialog', { name: label, exact: true });
        expect(await dialog.innerText()).toContain('This does not create another occurrence');
        await dialog
          .getByRole('textbox', { name: /Reason/ })
          .fill('Check the existing occurrence now');
        await capture(`${prefix}-run-now-confirm`);
        const responsePromise = page.waitForResponse(
          (response) =>
            response.request().method() === 'POST' && response.url().endsWith('/actions'),
        );
        await dialog.getByRole('button', { name: label, exact: true }).click();
        const response = await responsePromise;
        expect(response.status()).toBe(200);
        const item = await response.json();
        const requestedId = decodeURIComponent(new URL(response.url()).pathname.split('/').at(-2)!);
        expect(item.id).toBe(requestedId);
        expect(item.state).toBe('ready');
        expect(item.immediate).toBe(true);
        await dialog.waitFor({ state: 'hidden' });
        await page
          .getByText('Ready when worker capacity is available - Scan for new commands', {
            exact: true,
          })
          .waitFor();
        await row.getByRole('button', { name: 'Open Scan for new commands', exact: true }).click();
        const inspector = page.getByRole('dialog', { name: 'Scan for new commands', exact: true });
        await inspector.getByText('Ready', { exact: true }).waitFor();
        const facts = inspector.locator('.facts').first();
        const dimensions = await facts.evaluate((element) => ({
          height: element.clientHeight,
          content: element.scrollHeight,
        }));
        expect(dimensions.height).toBeGreaterThanOrEqual(dimensions.content);
        await capture(`${prefix}-run-now-result`);
        const body = inspector.locator('.modal-body');
        if (await body.evaluate((element) => element.scrollHeight > element.clientHeight)) {
          await inspector.locator('.timeline li').last().scrollIntoViewIfNeeded();
          await capture(`${prefix}-run-now-timeline-end`);
        }
        await page.keyboard.press('Escape');
      }
    } finally {
      await page.close();
    }
  },
);
