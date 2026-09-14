import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

const variants = [
  { colorScheme: 'light', reducedMotion: 'reduce' },
  { colorScheme: 'dark', reducedMotion: 'reduce' },
  { colorScheme: 'light', reducedMotion: 'no-preference' },
  { colorScheme: 'dark', reducedMotion: 'no-preference' },
] as const;

describe('contextual sidebar current destination', () => {
  it.each(variants)(
    'selects only the current page in $colorScheme with $reducedMotion',
    async ({ colorScheme, reducedMotion }) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
        reducedMotion,
      });
      page.setDefaultTimeout(10_000);
      try {
        await visit(page, addressOf(panel, 'root/workspaces/{account}/settings'));
        const tree = page.locator('.tree');
        const verify = async (label: string) => {
          const selected = tree.locator('[aria-current="page"]');
          await expect.poll(() => selected.count()).toBe(1);
          await expect.poll(async () => (await selected.innerText()).trim()).toBe(label);
          await expect
            .poll(async () =>
              selected.evaluate((row) => {
                const thumb = row.parentElement!.querySelector('.nav-thumb')!;
                const a = row.getBoundingClientRect();
                const b = thumb.getBoundingClientRect();
                return Math.max(
                  Math.abs(a.top - b.top),
                  Math.abs(a.left - b.left),
                  Math.abs(a.width - b.width),
                  Math.abs(a.height - b.height),
                );
              }),
            )
            .toBeLessThan(1);
          const contrast = await selected.evaluate((row) => {
            const rgb = (color: string) =>
              color
                .match(/[\d.]+/g)!
                .slice(0, 3)
                .map(Number);
            const luminance = (color: string) =>
              rgb(color)
                .map((value) => {
                  const s = value / 255;
                  return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
                })
                .reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
            const foreground = luminance(getComputedStyle(row).color);
            const background = luminance(
              getComputedStyle(row.parentElement!.querySelector('.nav-thumb')!).backgroundColor,
            );
            return (
              (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05)
            );
          });
          expect(contrast).toBeGreaterThanOrEqual(4.5);
          return contrast;
        };
        const capture = async (scene: string, label: string) => {
          const contrast = await verify(label);
          const directory = process.env.SMYKLOT_CONTEXTUAL_SCREENSHOTS;
          if (!directory) return;
          await mkdir(directory, { recursive: true });
          const stem = `F42-${scene}-${colorScheme}-${reducedMotion}`;
          await page.screenshot({ path: join(directory, `${stem}.png`) });
          const evidence = process.env.SMYKLOT_CONTEXTUAL_EVIDENCE;
          if (evidence) {
            await mkdir(evidence, { recursive: true });
            await writeFile(
              join(evidence, `${stem}.json`),
              JSON.stringify(
                { label, contrast, colorScheme, reducedMotion, url: page.url() },
                null,
                2,
              ),
            );
          }
        };
        await page.mouse.move(1900, 20);
        await capture('settings', 'Workspace settings');
        for (const label of ['Repositories', 'Users', 'Invitations', 'Audit', 'Failures']) {
          const link = tree.getByRole('link', { name: label, exact: true }).last();
          await link.focus();
          await page.keyboard.press('Enter');
          await verify(label);
          if (label === 'Users') {
            await link.focus();
            await capture('users-focus', label);
          }
        }
        const repositories = tree.getByRole('link', { name: 'Repositories', exact: true }).last();
        await repositories.click();
        await repositories.hover();
        await capture('repositories-hover', 'Repositories');
        await visit(page, addressOf(panel, 'root/workspaces/{account}/repositories/api-gateway'));
        await verify('Repositories');
        await tree.getByRole('link', { name: 'Workspaces', exact: true }).click();
        await verify('Workspaces');
        expect(
          await tree.getByRole('link', { name: 'Workspace settings', exact: true }).count(),
        ).toBe(0);
      } finally {
        await page.close();
      }
    },
    30_000,
  );
});
