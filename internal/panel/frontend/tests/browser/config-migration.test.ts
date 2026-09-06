import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';

import { SETTLE_MS, startPanel, type Panel } from './harness';
import { REPOSITORY_FILE_VARIANTS } from '../../dev/repository-files';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

async function resetMigration(path: string, repository: string): Promise<void> {
  const page: Page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
  const crashes: string[] = [];
  page.on('pageerror', (error) => crashes.push(error.message));

  try {
    await page.goto(`${panel.origin}${path}`, { waitUntil: 'domcontentloaded' });
    // A page now, reached by the address it had as a dialog. What proves it is a
    // page is that nothing stands over the list: no dialog role, and the list is
    // not underneath it.
    const title = page.getByRole('heading', { name: repository, exact: true });
    await title.waitFor({ state: 'visible', timeout: 30_000 });
    expect(await page.getByRole('dialog').count()).toBe(0);

    await page.getByRole('button', { name: 'Inspect file', exact: true }).click();
    const inspector = page.getByRole('dialog', { name: 'Configuration file', exact: true });
    const reset = inspector.getByRole('button', { name: 'Retry proposal' });
    await reset.waitFor({ state: 'visible' });
    const response = page.waitForResponse(
      (candidate) =>
        candidate.request().method() === 'POST' &&
        new URL(candidate.url()).pathname.endsWith('/config-migration'),
    );
    await reset.click();
    expect((await response).status()).toBe(200);
    await reset.waitFor({ state: 'detached' });
    await inspector.getByRole('button', { name: 'Done', exact: true }).click();

    // Leaving and coming back inside the query stale window must not resurrect
    // the refused state from a cached detail response.
    await page.getByRole('link', { name: 'Repositories', exact: true }).first().click();
    await title.waitFor({ state: 'detached' });

    /* The list is virtualised and renders only the rows in view, so a repository
       further down is not in the page to be pressed. Narrow to it the way a
       reader would.

       Typed straight after coming back, which is the part worth keeping: the
       list is the same component the page was drawn inside, so it is still
       mounted with the reader's place in it - it used to be torn down and built
       again from the last stored search, and what had just been typed went with
       it. */
    await page.getByPlaceholder('Find a repository').fill(repository);
    /* Not `exact`: the row's link is the name AND the override count beside it,
       so its accessible name is "search-indexer 2 overrides" for any repository
       that has overrides - which is most of the ones worth opening. */
    await page.getByRole('link', { name: repository }).click();
    await title.waitFor({ state: 'visible' });
    await page.waitForTimeout(SETTLE_MS);
    await page.getByRole('button', { name: 'Inspect file', exact: true }).click();
    expect(await page.getByRole('button', { name: 'Retry proposal' }).count()).toBe(0);
    expect(crashes).toEqual([]);
  } finally {
    await page.close();
  }
}

describe('configuration file observations and search priority', () => {
  it.each(Object.entries(REPOSITORY_FILE_VARIANTS))(
    'keeps %s readable and identifies only observed files',
    async (repository, fixture) => {
      const page = await panel.browser.newPage({ viewport: { width: 1440, height: 1000 } });
      try {
        await page.goto(`${panel.origin}/workspace/${panel.account}/repositories/${repository}`);
        const trigger = page.getByRole('button', { name: 'Inspect file', exact: true });
        await trigger.click();
        const inspector = page.getByRole('dialog', { name: 'Configuration file', exact: true });
        await inspector.waitFor();
        await inspector.evaluate(async (node) => {
          await Promise.all(node.getAnimations().map((animation) => animation.finished));
        });
        const selected = inspector.getByRole('region', { name: 'Selected configuration file' });
        const status = {
          valid: 'Valid',
          invalid: 'Invalid',
          missing: 'Missing',
          unknown: 'Not checked',
        };
        expect(await selected.locator('.pill').innerText()).toBe(status[fixture.status]);
        const found = fixture.status === 'valid' || fixture.status === 'invalid';
        expect(
          await selected
            .getByRole('heading', {
              name: found ? 'Selected file' : 'File status',
              exact: true,
            })
            .count(),
        ).toBe(1);
        expect(await selected.getByText('Path', { exact: true }).count()).toBe(found ? 1 : 0);
        expect(
          await inspector.getByText('The first file found is used', { exact: true }).count(),
        ).toBe(1);
        expect(await inspector.locator('.search-order .pill').allTextContents()).toEqual(
          found ? ['Selected', ...(fixture.superseded ?? []).map(() => 'Ignored')] : [],
        );
        expect(await inspector.getByRole('region', { name: 'Parsed file settings' }).count()).toBe(
          fixture.status === 'valid' ? 1 : 0,
        );
        for (const colorScheme of ['light', 'dark'] as const) {
          await page.emulateMedia({ colorScheme });
          for (const width of [1440, 375]) {
            await page.setViewportSize({ width, height: 1000 });
            const measured = await inspector.evaluate((node) => {
              const rows = Array.from(node.querySelectorAll('.search-path-row'));
              const boxes = rows.map((row) => row.getBoundingClientRect());
              const pathBoxes = rows.map((row) =>
                row.querySelector('.search-path')!.getBoundingClientRect(),
              );
              const checked = Array.from(node.querySelectorAll('.policy-row')).find((row) =>
                row.textContent?.startsWith('Last checked'),
              );
              const label = checked?.querySelector('.setting-name')?.getBoundingClientRect();
              const value = checked?.querySelector('[data-exact]')?.getBoundingClientRect();
              return {
                overlap: boxes.some(
                  (box, index) => index > 0 && box.top < boxes[index - 1]!.bottom - 1,
                ),
                sharedPathColumn: pathBoxes.every(
                  (box) => Math.abs(box.left - pathBoxes[0]!.left) < 1,
                ),
                numberInset: rows.map((row) => {
                  const number = row.querySelector('.search-priority')!;
                  const heading = row.closest('.card')!.querySelector('h3')!;
                  return number.getBoundingClientRect().left - heading.getBoundingClientRect().left;
                }),
                bareStatuses: rows
                  .flatMap((row) => Array.from(row.querySelectorAll('.pill')))
                  .filter((pill) => getComputedStyle(pill).backgroundColor === 'rgba(0, 0, 0, 0)')
                  .length,
                labelCenter: label ? label.y + label.height / 2 : null,
                valueCenter: value ? value.y + value.height / 2 : null,
                overflow: node.scrollWidth - node.clientWidth,
                rowOverflows: rows.filter((row) => row.scrollWidth > row.clientWidth + 1).length,
                statusCenters: rows.flatMap((row) => {
                  const pill = row.querySelector('.pill')?.getBoundingClientRect();
                  const path = row.querySelector('.search-path')!.getBoundingClientRect();
                  return pill ? [pill.y + pill.height / 2 - path.y - path.height / 2] : [];
                }),
              };
            });
            expect(measured.overlap).toBe(false);
            expect(measured.sharedPathColumn).toBe(true);
            expect(measured.numberInset.every((inset) => Math.abs(inset) < 1)).toBe(true);
            expect(measured.bareStatuses).toBe(0);
            expect(measured.overflow).toBeLessThanOrEqual(1);
            expect(measured.rowOverflows).toBe(0);
            expect(measured.statusCenters.every((center) => Math.abs(center) < 1)).toBe(true);
            if (measured.labelCenter !== null && measured.valueCenter !== null)
              expect(Math.abs(measured.labelCenter - measured.valueCenter)).toBeLessThan(1);
            const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
            if (directory) {
              await mkdir(join(directory, 'repository-configuration'), { recursive: true });
              await inspector.screenshot({
                path: join(
                  directory,
                  'repository-configuration',
                  `${repository}-${colorScheme}-${width}.png`,
                ),
                animations: 'disabled',
              });
            }
            if (width === 375 && fixture.migration && fixture.migration !== 'none') {
              const migration = inspector.getByRole('region', { name: 'TOML migration' });
              const action = migration.locator('a[href], button').first();
              // Bring the proposal's whole row into view, including the card's
              // closing frame rather than pinning a button against the footer.
              await migration.scrollIntoViewIfNeeded();
              const actionBox = await action.boundingBox();
              const bodyBox = await inspector.locator('.modal-body').boundingBox();
              const footerBox = await inspector.locator(':scope > footer').boundingBox();
              expect(actionBox).not.toBeNull();
              expect(bodyBox).not.toBeNull();
              expect(footerBox).not.toBeNull();
              expect(actionBox!.y).toBeGreaterThanOrEqual(bodyBox!.y);
              expect(actionBox!.y + actionBox!.height).toBeLessThanOrEqual(footerBox!.y);
              if (directory) {
                await inspector.screenshot({
                  path: join(
                    directory,
                    'repository-configuration',
                    `${repository}-${colorScheme}-${width}-migration.png`,
                  ),
                  animations: 'disabled',
                });
              }
              await inspector.locator('.modal-body').evaluate((node) => {
                node.scrollTop = 0;
              });
            }
          }
        }
        await page.keyboard.press('Escape');
        await inspector.waitFor({ state: 'detached' });
        expect(await trigger.evaluate((node) => document.activeElement === node)).toBe(true);
      } finally {
        await page.close();
      }
    },
  );
});

describe('the TOML migration reset in the development panel', () => {
  it('works in a workspace and keeps the page presentation', async () => {
    await resetMigration(
      `/workspace/${panel.account}/repositories/migration-demo`,
      'migration-demo',
    );
  });

  it('works through the Root workspace API', async () => {
    await resetMigration(
      `/root/workspaces/${panel.account}/repositories/search-indexer`,
      'search-indexer',
    );
  });
});
