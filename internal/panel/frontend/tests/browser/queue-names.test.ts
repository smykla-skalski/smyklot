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
it.each(['light', 'dark'] as const)('names queue filters in %s desktop', async (colorScheme) => {
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
      path: join(directory, `F13-${scene}-${colorScheme}.png`),
      animations: 'disabled',
    });
  };
  try {
    for (const scope of ['root', 'workspace/smykla-skalski']) {
      const prefix = scope === 'root' ? 'root' : 'workspace';
      await page.goto(`${panel.origin}/${scope}/queue`);
      await page.getByRole('button', { name: 'Filter queue', exact: true }).click();
      const hours = page.getByRole('listbox', { name: 'Hours', exact: true });
      await hours
        .getByRole('option', { name: 'Always Open', exact: true })
        .scrollIntoViewIfNeeded();
      expect(await hours.getByRole('option', { name: 'always-open', exact: true }).count()).toBe(0);
      await capture(`${prefix}-hours`);
      const repositories = page.getByRole('listbox', { name: 'Repository', exact: true });
      const repository = repositories.getByRole('option', {
        name: 'smykla-skalski/platform-infra',
        exact: true,
      });
      await repository.scrollIntoViewIfNeeded();
      expect(await repositories.getByRole('option', { name: '4002', exact: true }).count()).toBe(0);
      await capture(`${prefix}-repositories`);
      const filtered = page.waitForRequest(
        (request) =>
          request.url().includes('/queue?') &&
          new URL(request.url()).searchParams.get('repository') === '4002',
      );
      await repository.click();
      await filtered;
      expect(await repository.getAttribute('aria-selected')).toBe('true');
      await page.keyboard.press('Escape');
    }
  } finally {
    await page.close();
  }
});
