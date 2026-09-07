import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';

import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page, Request } from 'playwright-core';

import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

function batchSave(request: Request): boolean {
  return (
    request.method() === 'PUT' &&
    /\/api\/v1\/targets\/[^/]+\/settings$/u.test(new URL(request.url()).pathname)
  );
}

async function flip(page: Page, name: string): Promise<void> {
  const input = page.getByRole('checkbox', { name });
  await page.locator('label.switch').filter({ has: input }).click();
}

/* The kind switch says what pressing it would DO - "Pause label syncing" when
   it is on, "Resume" when it is off - so the stable handle is where it sits
   rather than what it currently reads. */
async function flipKind(page: Page): Promise<void> {
  await page.locator('.page-status label.switch').click();
}

describe('workspace Sync drafts', () => {
  it('persists across routes and workspaces, then saves every setting once', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    const saves: Request[] = [];
    let dialogs = 0;
    page.on('request', (request) => {
      if (batchSave(request)) saves.push(request);
    });
    page.on('dialog', (dialog) => {
      dialogs += 1;
      void dialog.dismiss();
    });

    try {
      await visit(page, addressOf(panel, 'workspace/sync/labels'), { ready: 'h1' });
      await flip(page, 'Delete unlisted labels');
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(0);

      await page.locator(`a[href="/workspace/${panel.account}/sync/settings"]`).click();
      await page.getByRole('heading', { name: 'Repository options' }).waitFor({ state: 'visible' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(0);

      await flipKind(page);
      await page.getByText('2 changed settings').waitFor({ state: 'visible' });
      const saved = page.waitForRequest(batchSave);
      await page.getByRole('button', { name: 'Save' }).click();
      const request = await saved;
      expect((request.postDataJSON() as { sync_configs: unknown[] }).sync_configs).toHaveLength(2);
      await page.getByText('Settings saved').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(1);
      const answer = (await (await request.response())?.json()) as { checkpoint_id?: string };
      expect(answer.checkpoint_id).toBeTruthy();
      const audit = await page.evaluate(async () => {
        const history = (await (await fetch('/api/v1/targets/2001/audit?limit=1')).json()) as {
          items: { action: string; settings_checkpoint_id?: string }[];
        };
        return history.items[0];
      });
      expect(audit).toMatchObject({
        action: 'installation.settings.saved',
        settings_checkpoint_id: answer.checkpoint_id,
      });
      const checkpointProof = await page.evaluate(async (checkpointId) => {
        const source = (await (
          await fetch(`/api/v1/targets/2001/settings/checkpoints/${checkpointId}`)
        ).json()) as {
          action: string;
          items: Array<{
            kind: string;
            sync_kind?: string;
            after: {
              state: { document: Record<string, unknown>; revision: number } | null;
            };
            current: { revision: number } | null;
          }>;
        };
        const rootInspection = (await (
          await fetch(`/api/v1/root/workspaces/2001/settings/checkpoints/${checkpointId}`)
        ).json()) as { id: string };
        const labels = source.items.find(
          (item) => item.kind === 'sync_config' && item.sync_kind === 'labels',
        );
        if (labels?.after.state === null || labels?.after === undefined) {
          throw new Error('saved labels checkpoint item is missing');
        }
        const document = labels.after.state.document;
        const nested = JSON.parse(String(document.document)) as Record<string, unknown>;
        const changed = await fetch('/api/v1/targets/2001/settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sync_configs: [
              {
                kind: 'labels',
                enabled: document.enabled,
                labels: nested.labels,
                allow_removal: false,
                excludes: nested.excludes,
                expected_revision: labels.after.state.revision,
              },
            ],
          }),
        });
        if (!changed.ok) throw new Error(`second save failed with ${changed.status}`);
        const changedAnswer = (await changed.json()) as {
          sync_configs: Array<{ revision: number }>;
        };
        const restored = await fetch(
          `/api/v1/targets/2001/settings/checkpoints/${checkpointId}/restore`,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              state: 'after',
              selections: [
                {
                  kind: 'sync_config',
                  sync_kind: 'labels',
                  expected_revision: changedAnswer.sync_configs[0]?.revision,
                },
              ],
            }),
          },
        );
        if (!restored.ok) throw new Error(`restore failed with ${restored.status}`);
        const restoredAnswer = (await restored.json()) as { checkpoint_id?: string };
        const latest = (await (await fetch('/api/v1/targets/2001/audit?limit=1')).json()) as {
          items: Array<{ action: string; settings_checkpoint_id?: string }>;
        };
        return {
          sourceAction: source.action,
          sourceKinds: source.items.map((item) => `${item.kind}:${item.sync_kind ?? ''}`),
          rootCheckpointId: rootInspection.id,
          restoredCheckpointId: restoredAnswer.checkpoint_id,
          latestAudit: latest.items[0],
        };
      }, answer.checkpoint_id!);
      expect(checkpointProof).toMatchObject({
        sourceAction: 'installation.settings.saved',
        sourceKinds: expect.arrayContaining(['sync_config:labels', 'sync_config:settings']),
        rootCheckpointId: answer.checkpoint_id,
        latestAudit: {
          action: 'installation.settings.restored',
          settings_checkpoint_id: checkpointProof.restoredCheckpointId,
        },
      });
      expect(checkpointProof.restoredCheckpointId).toBeTruthy();

      await page.locator(`a[href="/workspace/${panel.account}/sync/labels"]`).click();
      await flip(page, 'Delete unlisted labels');
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      /* A reload restores the draft and says so where the draft is, which is the
         composer counting it - there is no notice announcing it any more. */
      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      const workspaces = page.locator('nav[aria-label="Consoles"] a[href^="/workspace/"]');
      expect(await workspaces.count()).toBeGreaterThan(1);
      const originalWorkspace = page.locator(
        `nav[aria-label="Consoles"] a[href^="/workspace/${panel.account}/"]`,
      );
      const other = page
        .locator(
          `nav[aria-label="Consoles"] a[href^="/workspace/"]:not([href^="/workspace/${panel.account}/"])`,
        )
        .first();
      const originalPath = new URL(page.url()).pathname;
      await other.click();
      await page.waitForURL((url) => url.pathname !== originalPath);
      expect(await originalWorkspace.getAttribute('aria-label')).toContain('unsaved changes');
      await originalWorkspace.click();
      await page.waitForURL((url) => url.pathname === `/workspace/${panel.account}/sync`);
      await page.locator(`a.tree-row[href="${originalPath}"]`).click();
      await page.waitForURL((url) => url.pathname === originalPath);
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(dialogs).toBe(0);
    } finally {
      await page.close();
    }
  });

  /**
   * A change that removes something reports what it removed, and hands back the way to
   * keep it - and the receipt never covers the bar a reader is about to press.
   */
  it('receipts a removal, stands clear of the save bar, and undoes', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, addressOf(panel, 'workspace/sync/labels'), { ready: 'h2' });

      /* The fixture's list is empty, so the row this removes is one it made: a receipt
         has to name the label it took away, and a made row proves the name travelled. */
      const name = 'needs-triage';
      await page.getByRole('button', { name: 'Add a label' }).click();
      await page.getByRole('textbox', { name: 'Label name' }).fill(name);
      await page.getByRole('textbox', { name: 'Label name' }).press('Enter');

      const first = page.locator('.label-row').first();
      await first.getByRole('button', { name: `Remove ${name}` }).click();

      const receipt = page.locator('.toast');
      await receipt.getByText(`Removed ${name}`).waitFor();

      /* The composer is up - a label just left the draft - so the receipt has to be
         standing above it rather than on it. */
      const clear = await page.evaluate(() => {
        const toast = document.querySelector('.toast')?.getBoundingClientRect();
        const bar = document
          .querySelector('.settings-composer, .apply-bar')
          ?.getBoundingClientRect();
        if (toast === undefined || bar === undefined) return null;

        return Math.round(bar.top - toast.bottom);
      });
      expect(clear === null || clear >= 0).toBe(true);

      await receipt.getByRole('button', { name: 'Undo' }).click();
      await page.locator('.toast').getByText(`${name} is back`).waitFor();
      await page
        .locator('.label-row')
        .filter({ hasText: name })
        .first()
        .waitFor({ state: 'visible' });

      /* Escape takes the receipt away, which is the design system's rule and the one
         way out of it that needs no pointer. It is the TOPMOST surface's press here -
         nothing is open over the page - and the receipt is what answers. */
      await page.keyboard.press('Escape');
      await page.locator('.toast').waitFor({ state: 'detached' });
    } finally {
      await page.close();
    }
  });
});

describe('automatic sync status interactions', () => {
  for (const width of [1280, 375]) {
    it(`opens whole rows while keeping switch margins independent at ${width}px`, async () => {
      const page = await panel.browser.newPage({ viewport: { width, height: 900 } });
      try {
        await visit(page, addressOf(panel, 'workspace/sync'), { ready: '.sync-repo-summary' });
        const rows = page.locator('.sync-repo-summary');
        expect(await rows.count()).toBe(8);
        // A queued run stays on its schedule. Immediate dispatch is an explicit
        // action inside the inspector, never a side effect of 'Check now'.
        expect(await page.getByRole('button', { name: 'Check now', exact: true }).count()).toBe(0);
        const changes = page.getByRole('button', { name: 'View changes', exact: true });
        await changes.click();
        const inspector = page.getByRole('dialog', { name: 'Sync details', exact: true });
        await inspector.waitFor({ state: 'visible' });
        expect(await inspector.getByRole('button', { name: /Approve/ }).count()).toBe(0);
        await page.keyboard.press('Escape');
        await inspector.waitFor({ state: 'detached' });
        expect(await changes.evaluate((element) => element === document.activeElement)).toBe(true);
        const first = rows.first();
        const trigger = first.getByRole('button');
        const geometry = await first.evaluate((row) => {
          const ground = row.getBoundingClientRect();
          const hit = row.querySelector('.row-hit')!.getBoundingClientRect();
          return { width: ground.width - hit.width, height: ground.height - hit.height };
        });
        expect(Math.abs(geometry.width)).toBeLessThan(1);
        expect(Math.abs(geometry.height)).toBeLessThan(1);

        // Real pointer coordinates catch content intercepting an invisible row target.
        const name = await first.locator('.object-name').boundingBox();
        if (name === null) throw new Error('repository name has no bounds');
        await page.mouse.move(name.x + 5, name.y + name.height / 2);
        await expect
          .poll(() => first.evaluate((row) => getComputedStyle(row).backgroundColor))
          .not.toBe('rgba(0, 0, 0, 0)');
        const hover = await first.evaluate((row) => getComputedStyle(row).backgroundColor);
        await page.mouse.down();
        await expect
          .poll(() => first.evaluate((row) => getComputedStyle(row).backgroundColor))
          .not.toBe(hover);
        await page.mouse.up();
        await expect.poll(() => trigger.getAttribute('aria-expanded')).toBe('true');
        await trigger.press('Enter');
        await expect.poll(() => trigger.getAttribute('aria-expanded')).toBe('false');

        const expand = page.getByRole('button', { name: 'Show all 28 repositories' });
        expect(await expand.evaluate((element) => element.closest('.card-head') !== null)).toBe(
          true,
        );
        await expand.click();
        await expect.poll(() => rows.count()).toBe(28);
        const lastName = await rows.last().locator('.object-name').innerText();
        await page.getByRole('button', { name: 'Show fewer repositories' }).click();
        await expect.poll(() => rows.count()).toBe(8);
        await page.getByRole('searchbox', { name: 'Find a syncing repository' }).fill(lastName);
        await expect.poll(() => rows.count()).toBe(1);
        expect(await rows.first().locator('.object-name').innerText()).toBe(lastName);
        await page.getByRole('searchbox', { name: 'Find a syncing repository' }).fill('');

        const configuration = page.getByRole('region', {
          name: 'Shared configuration',
          exact: true,
        });
        const input = configuration.getByRole('checkbox', { name: 'Labels sync', exact: true });
        const label = configuration.locator('label.switch').filter({
          has: page.getByRole('checkbox', { name: 'Labels sync', exact: true }),
        });
        const initial = await input.isChecked();
        await label.scrollIntoViewIfNeeded();
        const bounds = await label.boundingBox();
        if (bounds === null) throw new Error('switch label has no bounds');
        expect(bounds.width).toBeGreaterThanOrEqual(44);
        expect(bounds.height).toBeGreaterThanOrEqual(44);
        // Above and to the left of the track belongs to the switch, not navigation.
        await page.mouse.click(bounds.x + 4, bounds.y + 4);
        await expect.poll(() => input.isChecked()).toBe(!initial);
        expect(new URL(page.url()).pathname).toBe(`/workspace/${panel.account}/sync`);
        await label.click();
        await expect.poll(() => input.isChecked()).toBe(initial);

        const configRow = configuration.locator('.object-row').first();
        const copy = await configRow.locator('.object-sum').boundingBox();
        if (copy === null) throw new Error('configuration copy has no bounds');
        await page.mouse.click(copy.x + 5, copy.y + copy.height / 2);
        await page.waitForURL((url) => url.pathname.endsWith('/sync/labels'));
        await page.getByRole('heading', { name: 'Labels', exact: true }).waitFor();
      } finally {
        await page.close();
      }
    });
  }
});

describe('repository option control anatomy', () => {
  for (const colorScheme of ['light', 'dark'] as const) {
    for (const width of [375, 768, 1440]) {
      it(`${colorScheme} ${width} keeps options and actions together`, async () => {
        const page = await panel.browser.newPage({
          colorScheme,
          viewport: { width, height: 1000 },
        });
        try {
          await visit(page, addressOf(panel, 'workspace/sync/settings'));
          const card = page
            .locator('.card')
            .filter({ has: page.getByRole('heading', { name: 'Commit wording', exact: true }) });
          await card.scrollIntoViewIfNeeded();
          const directory = process.env.SMYKLOT_OPTIONS_VISUAL_DIR;
          if (directory) {
            await mkdir(directory, { recursive: true });
            await page.evaluate(() => {
              if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
              window.scrollTo({ top: 0, left: 0, behavior: 'instant' });
            });
            await page.mouse.move(0, 0);
            await page.screenshot({
              path: join(directory, `${colorScheme}-${width}.png`),
              fullPage: true,
              animations: 'disabled',
            });
            await card.screenshot({ path: join(directory, `wording-${colorScheme}-${width}.png`) });
          }
          for (const label of ['Squash commit title', 'Squash commit message']) {
            const row = card
              .locator('.policy-row')
              .filter({ has: page.locator('.setting-name', { hasText: label }) });
            const picker = await row.locator('.value-select').boundingBox();
            const remove = await row.locator('.setting-clear, .icon-button').boundingBox();
            if (!picker || !remove) throw new Error('Missing option controls');
            expect(Math.abs(picker.height - 34)).toBeLessThan(0.6);
            expect(Math.abs(remove.height - picker.height)).toBeLessThan(0.6);
            expect(Math.abs(remove.width - remove.height)).toBeLessThan(0.6);
            expect(
              Math.abs(remove.y + remove.height / 2 - picker.y - picker.height / 2),
            ).toBeLessThan(0.6);
            expect(remove.x).toBeGreaterThanOrEqual(picker.x + picker.width);
            // A short value must stay readable when its pair has an entire line.
            const labelBox = row.locator('.value-select .button-label');
            expect(
              await labelBox.evaluate((element) => element.scrollWidth - element.clientWidth),
            ).toBeLessThanOrEqual(1);
          }
          const wiki = page.locator('[data-option=has_wiki]');
          const wikiCopy = (await wiki.locator('.setting-say').boundingBox())!;
          const wikiSwitch = (await wiki.locator('.switch').boundingBox())!;
          expect(
            Math.abs(wikiCopy.y + wikiCopy.height / 2 - wikiSwitch.y - wikiSwitch.height / 2),
          ).toBeLessThan(0.6);
          const trigger = card.getByRole('combobox', { name: 'Squash commit title', exact: true });
          const initial = await trigger.innerText();
          await trigger.click();
          const menu = page.getByRole('listbox', { name: 'Squash commit title', exact: true });
          await menu.waitFor();
          if (directory)
            await page.screenshot({ path: join(directory, `choices-${colorScheme}-${width}.png`) });
          await page.keyboard.press('End');
          await page.keyboard.press('Enter');
          await expect.poll(() => trigger.innerText()).not.toBe(initial);
          await trigger.click();
          await page.keyboard.press('Home');
          await page.keyboard.press('Enter');
          await expect.poll(() => trigger.innerText()).toBe(initial);
          await expect.poll(() => card.locator('.is-unsaved').count()).toBe(0);
          await page
            .getByRole('button', { name: 'Save', exact: true })
            .waitFor({ state: 'hidden' });
          const beforeHeight = (await card.boundingBox())!.height;
          const manage = card.getByRole('combobox', {
            name: 'Manage an option in Commit wording',
            exact: true,
          });
          await manage.click();
          const choices = page.getByRole('listbox', {
            name: 'Manage an option in Commit wording',
            exact: true,
          });
          await choices.waitFor();
          expect(Math.abs((await card.boundingBox())!.height - beforeHeight)).toBeLessThan(0.6);
          if (directory)
            await page.screenshot({ path: join(directory, `manage-${colorScheme}-${width}.png`) });
          await choices.getByRole('option', { name: 'Merge commit title', exact: true }).click();
          const added = card
            .locator('.policy-row')
            .filter({ has: page.locator('.setting-name', { hasText: /^Merge commit title$/u }) });
          await added.waitFor();
          await expect.poll(() => manage.innerText()).toBe('Manage an option');
          await expect
            .poll(() =>
              added.getByRole('combobox').evaluate((element) => element === document.activeElement),
            )
            .toBe(true);
          await added
            .getByRole('button', { name: 'Stop managing Merge commit title', exact: true })
            .click();
          await expect.poll(() => added.count()).toBe(0);
          await expect.poll(() => card.locator('.is-unsaved').count()).toBe(0);
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
          ).toBeLessThanOrEqual(1);
        } finally {
          await page.close();
        }
      });
    }
  }
});

describe('managed option focus visibility', () => {
  for (const width of [375, 768]) {
    it(`${width} reveals an added option above the current viewport`, async () => {
      const page = await panel.browser.newPage({ viewport: { width, height: 520 } });
      try {
        await visit(page, addressOf(panel, 'workspace/sync/settings'));
        const merging = page
          .locator('.card')
          .filter({ has: page.getByRole('heading', { name: 'Merging', exact: true }) });
        await merging
          .getByRole('button', { name: 'Stop managing Squash merging', exact: true })
          .click();
        const manage = merging.getByRole('combobox', {
          name: 'Manage an option in Merging',
          exact: true,
        });
        for (const label of ['Rebase merging', 'Offer to update the branch']) {
          await manage.click();
          await page.getByRole('option', { name: label, exact: true }).click();
        }
        await manage.evaluate((element) => {
          const top = Math.max(
            0,
            document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0,
          );
          window.scrollBy({
            top: element.getBoundingClientRect().top - top - 24,
            behavior: 'instant',
          });
        });
        await manage.click();
        await page.getByRole('option', { name: 'Squash merging', exact: true }).click();
        const input = merging.getByRole('checkbox', { name: 'Squash merging', exact: true });
        await expect
          .poll(() => input.evaluate((element) => document.activeElement === element))
          .toBe(true);
        await expect
          .poll(() =>
            input.evaluate((element) => {
              const control = element.closest('.policy-row')!;
              const top = Math.max(
                0,
                document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0,
              );
              return control.getBoundingClientRect().top - top;
            }),
          )
          .toBeGreaterThanOrEqual(0);
        const row = merging.locator('[data-option="allow_squash_merge"]');
        const bounds = (await row.boundingBox())!;
        const composer = await page.locator('.settings-composer').boundingBox();
        expect(bounds.y + bounds.height).toBeLessThanOrEqual(composer?.y ?? 520);
        const directory = process.env.SMYKLOT_OPTIONS_VISUAL_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({ path: join(directory, `focused-${width}.png`) });
        }
      } finally {
        await page.close();
      }
    });
  }
});
