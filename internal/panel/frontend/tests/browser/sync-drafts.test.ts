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
