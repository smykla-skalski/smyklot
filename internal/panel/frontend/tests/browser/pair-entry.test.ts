import { mkdir } from 'node:fs/promises';
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

describe('alias command picker [Browser]', () => {
  it.each(['light', 'dark'] as const)(
    'opens from its field and uses compact rows in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1000 },
        colorScheme,
      });
      try {
        await visit(page, addressOf(panel, 'workspace/settings'));
        const command = page.getByRole('combobox', { name: 'Command for alias ship' });
        await command.scrollIntoViewIfNeeded();
        await command.click();
        const options = page.locator('.pair-menu').getByRole('option');
        await expect.poll(() => options.count()).toBe(7);
        const auditDirectory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (auditDirectory !== undefined) {
          await mkdir(auditDirectory, { recursive: true });
          await page.screenshot({ path: join(auditDirectory, `alias-picker-${colorScheme}.png`) });
        }
        const sizes = await options.evaluateAll((nodes) =>
          nodes.map((node) => {
            const box = node.getBoundingClientRect();
            return {
              height: box.height,
              label: node.textContent,
              padding: getComputedStyle(node).padding,
            };
          }),
        );
        expect(
          sizes.every((size) => size.height === 34),
          JSON.stringify(sizes),
        ).toBe(true);
        await command.press('Escape');
        await expect.poll(() => command.getAttribute('aria-expanded')).toBe('false');
        await command.click();
        await expect.poll(() => options.count()).toBe(7);
        await command.fill('squ');
        await expect.poll(() => options.count()).toBe(1);
        await command.press('ArrowDown');
        const active = await command.evaluate((node) => {
          const id = node.getAttribute('aria-activedescendant');
          return {
            id,
            label: id === null ? null : document.getElementById(id)?.textContent,
            typed: (node as HTMLInputElement).value,
            focused: document.activeElement === node,
            options: [...document.querySelectorAll('.pair-option')].map((option) => ({
              id: option.id,
              value: option.getAttribute('data-value'),
              attrs: option.getAttributeNames(),
            })),
          };
        });
        expect(active.label, JSON.stringify(active)).toContain('squash');
        await command.press('Enter');
        await expect.poll(() => command.inputValue()).toBe('squash');
        await command.click();
        await expect.poll(() => options.count()).toBe(7);
        await command.fill('not-a-command');
        await page.getByText('No matching command', { exact: true }).waitFor();
        await command.press('Escape');
        await expect.poll(() => command.inputValue()).toBe('squash');
      } finally {
        await page.close();
      }
    },
  );
});
