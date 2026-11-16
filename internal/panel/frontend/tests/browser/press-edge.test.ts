import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Locator, Page } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

let page: Page;
let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
  page = await panel.browser.newPage({
    viewport: { width: 1500, height: 950 },
    deviceScaleFactor: 2,
  });
}, 60_000);

afterAll(async () => {
  await panel?.close();
});

async function topEdge(control: Locator): Promise<{ x: number; y: number }> {
  await control.waitFor({ state: 'visible', timeout: 30_000 });
  const box = await control.boundingBox();
  if (box === null) throw new Error('control has no box');
  return { x: box.x + box.width / 2, y: box.y };
}

async function holdAndRelease(
  target: Page,
  from: { x: number; y: number },
  to = from,
): Promise<void> {
  await target.mouse.move(from.x, from.y);
  await target.mouse.down();
  await target.waitForTimeout(300);
  await target.mouse.move(to.x, to.y);
  await target.mouse.up();
}

describe('a press on the top edge of a sidebar control', () => {
  it('follows the link after the press sinks it below the pointer', async () => {
    await visit(page, `${panel.origin}/root/access/users`);
    const row = page.locator('.tree a.tree-row:not(.is-active)').first();
    const edge = await topEdge(row);
    const href = await row.getAttribute('href');

    await holdAndRelease(page, edge);

    await page.waitForURL((url) => url.pathname === href, { timeout: 5_000 });
  });

  it('runs the button after the press sinks it below the pointer', async () => {
    await visit(page, `${panel.origin}/root/access/users`);
    const fold = page.locator('.side-fold');
    const edge = await topEdge(fold);
    const before = await fold.getAttribute('aria-expanded');

    await holdAndRelease(page, edge);

    await expect.poll(() => fold.getAttribute('aria-expanded')).not.toBe(before);
  });

  it('still cancels a press dragged off the control before release', async () => {
    await visit(page, `${panel.origin}/root/access/users`);
    const row = page.locator('.tree a.tree-row:not(.is-active)').first();
    const edge = await topEdge(row);
    const start = page.url();

    await holdAndRelease(page, edge, { x: edge.x, y: edge.y - 40 });
    await page.waitForTimeout(500);

    expect(page.url()).toBe(start);
  });
});
