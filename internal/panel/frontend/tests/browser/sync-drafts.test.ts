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

describe('installation Sync drafts', () => {
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
      await visit(page, addressOf(panel, 'i/sync/labels'), { ready: 'h2' });
      await flip(page, 'Remove labels this list does not name');
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(0);

      await page.locator(`a[href="/i/${panel.account}/sync/settings"]`).click();
      await page.getByRole('heading', { name: 'Settings' }).waitFor({ state: 'visible' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(saves).toHaveLength(0);

      await flip(page, 'Settings sync');
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
          await fetch(`/api/v1/root/installations/2001/settings/checkpoints/${checkpointId}`)
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

      await page.locator(`a[href="/i/${panel.account}/sync/labels"]`).click();
      await flip(page, 'Remove labels this list does not name');
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      await page.reload({ waitUntil: 'domcontentloaded' });
      await page.getByText('Unsaved settings restored').waitFor({ state: 'visible' });
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });

      const workspaces = page.locator('nav[aria-label="Consoles"] a[href^="/i/"]');
      expect(await workspaces.count()).toBeGreaterThan(1);
      const originalWorkspace = page.locator(
        `nav[aria-label="Consoles"] a[href^="/i/${panel.account}/"]`,
      );
      const other = page
        .locator(`nav[aria-label="Consoles"] a[href^="/i/"]:not([href^="/i/${panel.account}/"])`)
        .first();
      const originalPath = new URL(page.url()).pathname;
      await other.click();
      await page.waitForURL((url) => url.pathname !== originalPath);
      expect(await originalWorkspace.getAttribute('aria-label')).toContain('unsaved changes');
      await originalWorkspace.click();
      await page.waitForURL((url) => url.pathname === `/i/${panel.account}/sync`);
      await page.locator(`a.tree-kid[href="${originalPath}"]`).click();
      await page.waitForURL((url) => url.pathname === originalPath);
      await page.getByText('1 changed setting').waitFor({ state: 'visible' });
      expect(dialogs).toBe(0);
    } finally {
      await page.close();
    }
  });
});
