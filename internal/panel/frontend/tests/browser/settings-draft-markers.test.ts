import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Page } from 'playwright-core';

import { startPanel, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

async function openRepositories(page: Page): Promise<void> {
  await page.goto(`${panel.origin}/workspace/${panel.account}/repositories`, {
    waitUntil: 'domcontentloaded',
  });
  await page.locator('.repository-row').first().waitFor({ state: 'visible', timeout: 30_000 });
}

describe('settings draft destinations [Integration]', () => {
  it.each([
    { colorScheme: 'dark', width: 1440 },
    { colorScheme: 'light', width: 1440 },
    { colorScheme: 'dark', width: 375 },
    { colorScheme: 'light', width: 375 },
  ] as const)(
    'keeps ruleset changes local and separated at $colorScheme $width',
    async ({ colorScheme, width }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1000 },
        reducedMotion: 'reduce',
      });
      try {
        await page.goto(
          `${panel.origin}/workspace/${panel.account}/sync/rulesets/main-protection`,
          { waitUntil: 'domcontentloaded' },
        );
        const bypass = page
          .locator('.card')
          .filter({ has: page.getByRole('heading', { name: 'Bypass list', exact: true }) });
        await bypass.waitFor({ state: 'visible', timeout: 30_000 });
        const appRow = bypass.locator('.actor-row').filter({ hasText: 'Smyklot' });
        const mode = appRow.getByRole('combobox');
        await mode.click();
        await page.getByRole('option', { name: 'Always allow', exact: true }).click();
        await expect.poll(() => appRow.getAttribute('data-unsaved')).toBe('true');
        expect(await page.locator('.card.is-unsaved').count()).toBe(1);
        expect(await page.locator('.policy-row.is-unsaved').count()).toBe(0);
        expect(await bypass.locator('.actor-row.is-unsaved').count()).toBe(1);
        const addActor = bypass.getByRole('button', { name: 'Add an actor', exact: true });
        expect(
          await bypass.locator('.card-head').getByRole('button', { name: 'Add an actor' }).count(),
        ).toBe(1);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await bypass.evaluate((node) => node.scrollIntoView({ block: 'center' }));
          await page.mouse.move(0, 0);
          await bypass.screenshot({
            path: join(directory, `bypass-header-${colorScheme}-${width}.png`),
          });
        }
        await mode.click();
        await page.getByRole('option', { name: 'Pull requests only', exact: true }).click();
        await expect.poll(() => page.locator('.is-unsaved[data-unsaved]').count()).toBe(0);
        await addActor.click();
        await bypass.getByRole('combobox', { name: 'Who' }).waitFor();
        expect(await addActor.getAttribute('aria-expanded')).toBe('true');
        await addActor.click();
        await bypass.getByRole('combobox', { name: 'Who' }).waitFor({ state: 'hidden' });
        expect(await addActor.getAttribute('aria-expanded')).toBe('false');
        await addActor.click();
        await bypass.getByRole('button', { name: 'Cancel', exact: true }).click();
        expect(await addActor.evaluate((node) => document.activeElement === node)).toBe(true);

        const conditions = page
          .locator('.card')
          .filter({ has: page.getByRole('heading', { name: 'Where it applies', exact: true }) });
        const included = conditions.locator('.policy-row').filter({ hasText: 'Included branches' });
        const excluded = conditions.locator('.policy-row').filter({ hasText: 'Excluded branches' });
        await included.getByRole('button', { name: 'Add a pattern' }).click();
        await page.getByRole('textbox', { name: 'Pattern to add' }).fill('release/*');
        await page.getByRole('textbox', { name: 'Pattern to add' }).press('Enter');
        await page
          .getByRole('textbox', { name: 'Pattern to add' })
          .waitFor({ state: 'hidden', timeout: 3000 });
        await expect.poll(() => included.getAttribute('data-unsaved')).toBe('true');
        expect(await excluded.getAttribute('data-unsaved')).toBeNull();
        await excluded.getByRole('button', { name: 'Add a pattern' }).click();
        await page.getByRole('textbox', { name: 'Pattern to add' }).fill('archive/*');
        await page.getByRole('textbox', { name: 'Pattern to add' }).press('Enter');
        await page
          .getByRole('textbox', { name: 'Pattern to add' })
          .waitFor({ state: 'hidden', timeout: 3000 });
        await expect.poll(() => excluded.getAttribute('data-unsaved')).toBe('true');
        const geometry = await included.evaluate((node) => {
          const style = getComputedStyle(node);
          const next = node.nextElementSibling!;
          return {
            gap: next.getBoundingClientRect().top - node.getBoundingClientRect().bottom,
            clearPixel: style.borderBottomWidth,
            border: style.borderBottomColor,
            clip: style.backgroundClip,
            separator: getComputedStyle(node, '::after').backgroundColor,
            animations: node.getAnimations().length,
          };
        });
        if (directory) {
          await writeFile(
            join(directory, `adjacent-edits-${colorScheme}-${width}.json`),
            JSON.stringify(geometry, null, 2),
          );
          await conditions.evaluate((node) => node.scrollIntoView({ block: 'center' }));
          await page.mouse.move(0, 0);
          await conditions.screenshot({
            path: join(directory, `adjacent-edits-${colorScheme}-${width}.png`),
          });
        }
        expect(geometry).toEqual({
          gap: 0,
          clearPixel: '1px',
          border: 'rgba(0, 0, 0, 0)',
          clip: 'padding-box',
          separator: 'rgba(0, 0, 0, 0)',
          animations: 0,
        });
        await included.getByRole('button', { name: 'Remove release/*', exact: true }).click();
        await excluded.getByRole('button', { name: 'Remove archive/*', exact: true }).click();
        await expect.poll(() => page.locator('.is-unsaved[data-unsaved]').count()).toBe(0);
        expect(await page.locator('.card.is-unsaved').count()).toBe(0);
      } finally {
        await page.close();
      }
    },
  );

  it('identifies the dirty workspace, section, repository, and exact setting', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });

    try {
      await openRepositories(page);
      const repositoryRow = page.locator('.repository-row').first();
      const repositoryName =
        (await repositoryRow.locator('.object-name').textContent())?.trim() ?? '';
      expect(repositoryName).not.toBe('');
      await repositoryRow.locator('a.row-hit').click();

      const quietPeriod = page.getByRole('textbox', { name: 'Quiet period after checks pass' });
      await quietPeriod.waitFor({ state: 'visible', timeout: 15_000 });
      const savedValue = await quietPeriod.inputValue();
      await quietPeriod.fill(savedValue === '45' ? '46' : '45');
      await quietPeriod.blur();
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      const settingRow = page.locator('.policy-row').filter({ has: quietPeriod });
      expect(await settingRow.getAttribute('data-unsaved')).toBe('true');

      /* The draft survives the reload, and what says so is the page it is on plus the
         tree's mark on every scope holding one. Nothing announces it: a notice would
         report a thing the reader had not just done, and cover the page saying it. */
      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible', timeout: 15_000 });
      expect(await page.locator('.tree-row.has-dirty').count()).toBeGreaterThan(0);

      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
      await page.getByRole('button', { name: 'Override the deployment session lifetime' }).click();
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      await page.goto(`${panel.origin}/root/workspaces`, { waitUntil: 'domcontentloaded' });
      const rootRepositoryHref = `/root/workspaces/${panel.account}/repositories`;
      /* The console's catalog is a list of sentences now, and a workspace is opened by
         name rather than by pressing its row - so the link is the row's one act, and it
         still carries where the unsaved work is. */
      const workspaceLink = page.locator(`a[href="${rootRepositoryHref}"]`);
      await workspaceLink.waitFor({ state: 'visible', timeout: 30_000 });
      const workspaceRow = page.locator('.object-row', { has: workspaceLink });
      expect(await workspaceRow.getAttribute('data-unsaved')).toBe('true');
      expect(await workspaceRow.innerText()).toContain('1 unsaved setting');

      await workspaceLink.click();
      await page.waitForURL((url) => url.pathname === rootRepositoryHref);
      const repositoryLeaf = page.locator(`a.tree-row[href="${rootRepositoryHref}"]`);
      expect(await repositoryLeaf.innerText()).toContain('Unsaved changes');

      const markedRepository = page
        .locator('.repository-row')
        .filter({ has: page.locator('.object-name', { hasText: repositoryName }) })
        .first();
      await markedRepository.waitFor({ state: 'visible', timeout: 15_000 });
      expect(await markedRepository.getAttribute('data-unsaved')).toBe('true');
      expect(await markedRepository.innerText()).toContain('Unsaved changes');
      expect(await page.locator('.settings-composer').count()).toBe(1);

      await page.getByRole('button', { name: 'Discard', exact: true }).click();
      await markedRepository.waitFor({ state: 'visible' });
      expect(await markedRepository.getAttribute('data-unsaved')).toBeNull();
      // Discard clears the draft immediately; the composer then finishes its exit animation.
      await expect
        .poll(() => page.locator('.settings-composer').count(), { timeout: 2_000 })
        .toBe(0);

      await page.goto(`${panel.origin}/root/runtime/settings`, { waitUntil: 'domcontentloaded' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      await page.getByRole('button', { name: 'Discard', exact: true }).click();
      await expect
        .poll(() => page.locator('.settings-composer').count(), { timeout: 2_000 })
        .toBe(0);
    } finally {
      await page.close();
    }
  });
});
