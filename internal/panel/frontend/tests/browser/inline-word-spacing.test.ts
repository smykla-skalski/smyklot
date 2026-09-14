import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import type { Locator } from 'playwright-core';
import { afterAll, beforeAll, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

// Measure the rendered word boundary, not textContent: flex layout can preserve
// whitespace in the DOM while dropping its visual advance.
async function wordGap(locator: Locator, emphasisFirst: boolean) {
  return locator.evaluate((element, first) => {
    const emphasis = element.querySelector('span, small, strong')!;
    const text = [...element.childNodes].find(
      (node) => node.nodeType === Node.TEXT_NODE && node.textContent?.trim(),
    )!;
    const content = text.textContent!;
    const index = first ? content.search(/\S/) : content.search(/\s*$/) - 1;
    const range = document.createRange();
    range.setStart(text, index);
    range.setEnd(text, index + 1);
    const glyph = range.getBoundingClientRect();
    const mark = emphasis.getBoundingClientRect();
    return first ? glyph.left - mark.right : mark.left - glyph.right;
  }, emphasisFirst);
}

it.each(['light', 'dark'] as const)(
  'preserves inline word boundaries in %s desktop scenes',
  async (theme) => {
    const page = await panel.browser.newPage({
      viewport: { width: 1920, height: 1200 },
      colorScheme: theme,
    });
    const directory = process.env.SMYKLOT_SYNC_OBSERVATION_SCREENSHOTS;
    async function capture(scene: string) {
      if (!directory) return;
      await mkdir(directory, { recursive: true });
      await page.mouse.move(1900, 20);
      await page.screenshot({ path: join(directory, `F22-${scene}-${theme}.png`) });
    }
    try {
      for (const [route, scene] of [
        ['root', 'root-attention'],
        ['workspace/', 'workspace-attention'],
      ]) {
        await visit(page, addressOf(panel, route));
        expect(await page.locator('.verdict-head .card-title').first().textContent()).toMatch(
          /4 items need attention/,
        );
        await capture(scene);
      }
      await visit(page, addressOf(panel, 'workspace/access/users?dialog=access-decision&user=ada'));
      await page.locator('label.choice-card').filter({ hasText: 'Suspend access' }).click();
      expect(await page.locator('.reason-field > span').first().textContent()).toBe(
        'Reason (optional)',
      );
      await capture('optional-reason');
      await visit(page, addressOf(panel, 'workspace/sync/plan'));
      const timestamp = page
        .locator('.hero-meta-lines > span')
        .filter({ hasText: 'Changes prepared' });
      expect(await wordGap(timestamp, false)).toBeGreaterThan(2);
      await capture('prepared-time');
    } finally {
      await page.close();
    }
  },
);
