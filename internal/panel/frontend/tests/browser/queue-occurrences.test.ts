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
