import type { Page } from 'playwright-core';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { addressOf, startPanel, visit, type Panel } from './harness';

const LONG_NAME = 'smyklot:pending:ci:squash:required:release:now:123';
const DESCRIPTION = 'Pending CI - squash merge, required checks only';

interface Geometry {
  inlineOverlap: number;
  nameLines: number;
  nameOverflow: number;
  rowOverflow: number;
  stacked: boolean;
}

let panel: Panel;
let page: Page;
let patternEditorsAfterLabelAdd = 0;
let rowsAddedByRepeatedPress = 0;

async function geometry(): Promise<Geometry> {
  return page
    .locator('.label-row')
    .first()
    .evaluate((row) => {
      const name = row.querySelector<HTMLElement>('.label-name');
      const description = row.querySelector<HTMLElement>('.label-desc');
      if (name === null || description === null)
        throw new Error('the long label row did not render');

      const nameBox = name.getBoundingClientRect();
      const descriptionBox = description.getBoundingClientRect();
      const lineHeight = Number.parseFloat(getComputedStyle(name).lineHeight);

      return {
        inlineOverlap: nameBox.right - descriptionBox.left,
        nameLines: Math.round(nameBox.height / lineHeight),
        nameOverflow: name.scrollWidth - name.clientWidth,
        rowOverflow: row.scrollWidth - row.clientWidth,
        stacked: descriptionBox.top >= nameBox.bottom - 0.5,
      };
    });
}

beforeAll(async () => {
  panel = await startPanel();
  page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
  await visit(page, addressOf(panel, 'i/sync/labels'), { ready: 'h2' });

  await page.getByRole('button', { name: 'Add', exact: true }).click();
  const initialRows = await page.locator('.label-row').count();
  const addLabel = page.getByRole('button', { name: 'Add a label' });
  await addLabel.click();
  patternEditorsAfterLabelAdd = await page.getByRole('textbox', { name: 'Pattern' }).count();
  await addLabel.click();
  rowsAddedByRepeatedPress = (await page.locator('.label-row').count()) - initialRows;
  await page.getByRole('textbox', { name: 'Label name' }).fill(LONG_NAME);
  await page.getByRole('textbox', { name: 'Label name' }).press('Enter');
  await page.locator('.label-row').first().locator('.label-desc').click();
  await page.getByRole('textbox', { name: 'Label description' }).fill(DESCRIPTION);
  await page.getByRole('textbox', { name: 'Label description' }).press('Enter');
});

afterAll(async () => {
  await page?.close();
  await panel?.close();
});

describe('a long Sync label [Integration]', () => {
  it('closes the pattern editor when Add a label is pressed', () => {
    expect(patternEditorsAfterLabelAdd).toBe(0);
  });

  it('replaces an unnamed new row when Add is pressed again', () => {
    expect(rowsAddedByRepeatedPress).toBe(1);
  });

  it('stays in its own desktop column', async () => {
    const desktop = await geometry();

    expect(LONG_NAME).toHaveLength(50);
    expect(desktop.inlineOverlap).toBeLessThanOrEqual(0.5);
    expect(desktop.nameOverflow).toBeLessThanOrEqual(0);
    expect(desktop.rowOverflow).toBeLessThanOrEqual(0);
    expect(desktop.stacked).toBe(false);
  });

  it('stacks without widening a phone page', async () => {
    await page.setViewportSize({ width: 375, height: 812 });
    const phone = await geometry();

    expect(phone.nameLines).toBeGreaterThan(1);
    expect(phone.nameOverflow).toBeLessThanOrEqual(0);
    expect(phone.rowOverflow).toBeLessThanOrEqual(0);
    expect(phone.stacked).toBe(true);
  });
});
