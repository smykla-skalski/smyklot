import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Locator, Page } from 'playwright-core';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';

import { startPanel, visit, type Panel } from './harness';
import type {
  SettingsCheckpoint,
  SyncOverride,
  WorkspaceSettingsBatchInput,
  WorkspaceSettingsBatchResponse,
} from '../../src/lib/types';

let panel: Panel;

async function expectDraftRecovery(inspector: Locator): Promise<void> {
  expect(await inspector.getByRole('alert').count()).toBe(1);
  const code = inspector.locator('.code:visible, .code-editor:visible');
  expect(await code.count()).toBe(1);
  const box = await code.boundingBox();
  const error = await inspector.getByRole('alert').boundingBox();
  expect(box).not.toBeNull();
  expect(error).not.toBeNull();
  const styles = await inspector.evaluate((node) =>
    Array.from(node.querySelectorAll('.editor-problem,.editor-problem > *, .preview-pane')).map(
      (element) => {
        const style = getComputedStyle(element);
        const rect = element.getBoundingClientRect();
        return {
          className: element.className,
          y: rect.y,
          height: rect.height,
          display: style.display,
          margin: style.margin,
          padding: style.padding,
          lineHeight: style.lineHeight,
          textBox: style.textBox,
          gap: style.gap,
        };
      },
    ),
  );
  expect(error!.y - box!.y - box!.height, JSON.stringify(styles)).toBeCloseTo(8, 0);
  const geometry = await inspector.evaluate((node) => {
    const error = node.querySelector('.editor-problem .form-error')!;
    const probe = document.createElement('span');
    probe.style.color = 'var(--danger)';
    node.append(probe);
    const expectedColor = getComputedStyle(probe).color;
    probe.remove();
    const heading = node.querySelector('.merge-pane-title .t')!.getBoundingClientRect();
    const status = node
      .querySelector('.merge-pane-title .setting-unmanaged')
      ?.getBoundingClientRect();
    return {
      color: getComputedStyle(error).color,
      expectedColor,
      statusCenter: status ? status.y + status.height / 2 - heading.y - heading.height / 2 : 0,
      overflow: node.scrollWidth - node.clientWidth,
    };
  });
  expect(geometry.color).toBe(geometry.expectedColor);
  expect(Math.abs(geometry.statusCenter)).toBeLessThan(1);
  expect(geometry.overflow).toBeLessThanOrEqual(1);
}

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

async function persistedMerge(page: Page, repositoryId: string): Promise<unknown> {
  return page.evaluate((id) => {
    const key = Object.keys(localStorage).find((key) =>
      key.startsWith('smyklot.panel.settings-drafts.v1:'),
    );
    if (!key) return null;
    const document = JSON.parse(localStorage.getItem(key)!);
    const record = document.records.find(
      (record: { deleted: boolean; resource: { type: string; repositoryId?: string } }) =>
        !record.deleted &&
        record.resource.type === 'sync-override' &&
        record.resource.repositoryId === id,
    );
    return (
      record?.draft.document.merges?.find(
        (merge: { path: string }) => merge.path === 'renovate.json',
      ) ?? null
    );
  }, repositoryId);
}

describe('configured file formatting in the development panel', () => {
  it('preserves a rejected raw draft when moving between repository and shared-file editors', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1440, height: 1000 } });
    const renderRequests: string[] = [];
    page.on('request', (request) => {
      if (
        request.url().endsWith('/sync/files/render') &&
        request.postDataJSON()?.repository !== undefined
      )
        renderRequests.push(request.postData() ?? '');
    });
    let savedCorrection = false;
    let overrideUrl = '';
    let original: SyncOverride | undefined;
    try {
      const repositoryUrl = `${panel.origin}/workspace/${panel.account}/repositories/smyklot`;
      await visit(page, repositoryUrl, { ready: '.sync-pane .code-editor' });
      const raw = page.locator('.sync-pane .cm-content[aria-label="Content adjustments"]').first();
      const corrected = { ...JSON.parse(await raw.innerText()), id: 3 };
      const duplicate = '{"id":2,"\\u0069d":3}';
      await raw.fill(duplicate);
      await page
        .getByText('Enter a valid JSON object for renovate.json', { exact: true })
        .waitFor();
      // Keep the layout's registry alive throughout the handoff and recovery.
      await page.locator(`a[href="/workspace/${panel.account}/sync/files"]`).click();
      await page.locator(`a[href="/workspace/${panel.account}/sync/files/renovate.json"]`).click();
      const loaded = page.waitForResponse(
        (response) =>
          response.request().method() === 'GET' &&
          /\/repositories\/[^/]+\/sync\/files$/.test(response.url()),
      );
      await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
      const loadedOverride = await loaded;
      overrideUrl = loadedOverride.url();
      original = (await loadedOverride.json()) as SyncOverride;
      const inspector = page.getByRole('dialog', { name: 'smyklot', exact: true });
      await inspector
        .getByText("Finish this adjustment in the repository's File sync settings")
        .waitFor();
      expect(await inspector.locator('.code-editor').count()).toBe(0);
      expect(await inspector.locator('.code').innerText()).toContain(duplicate);
      await inspector
        .getByRole('radio', { name: 'Final output', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      expect(await inspector.getByRole('alert').count()).toBe(1);
      expect(await inspector.getByText('Preparing final output…').count()).toBe(0);
      await inspector
        .getByRole('radio', { name: 'Content adjustments', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (directory) await mkdir(directory, { recursive: true });
      for (const [colorScheme, width] of [
        ['light', 1440],
        ['dark', 1440],
        ['light', 375],
        ['dark', 375],
      ] as const) {
        await page.emulateMedia({ colorScheme });
        await page.setViewportSize({ width, height: 1000 });
        if (directory)
          await inspector.screenshot({
            path: join(directory, `raw-draft-handoff-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        await expectDraftRecovery(inspector);
      }
      expect(renderRequests).toEqual([]);
      await inspector.getByRole('button', { name: 'Done', exact: true }).click();
      expect(await page.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(true);
      await page.setViewportSize({ width: 1440, height: 1000 });
      await page
        .getByRole('navigation', { name: 'Pages', exact: true })
        .getByRole('link', { name: /^Repositories/u })
        .click();
      await page
        .getByRole('searchbox', { name: 'Search repositories', exact: true })
        .fill('smyklot');
      await page.getByRole('link', { name: 'Open smyklot', exact: true }).click();
      await raw.waitFor();
      expect(await raw.innerText()).toBe(duplicate);
      await raw.fill(JSON.stringify(corrected));
      await expect
        .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
        .toBe(true);
      const saved = page.waitForResponse(
        (response) => response.request().method() === 'PUT' && response.url().endsWith('/settings'),
      );
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const response = await saved;
      expect(response.ok(), await response.text()).toBe(true);
      savedCorrection = true;
      const update = response.request().postDataJSON() as WorkspaceSettingsBatchInput;
      const savedDocument = update.sync_overrides?.[0].document as {
        merges: Array<{ overrides: unknown }>;
      };
      expect(savedDocument.merges[0].overrides).toEqual(corrected);
      const readback = (await (await page.request.get(overrideUrl)).json()) as SyncOverride;
      expect((readback.document.merges as Array<{ overrides: unknown }>)[0].overrides).toEqual(
        corrected,
      );
      await page.locator(`a[href="/workspace/${panel.account}/sync/files"]`).click();
      await page.locator(`a[href="/workspace/${panel.account}/sync/files/renovate.json"]`).click();
      const recoveredRender = page.waitForResponse(
        (response) =>
          response.url().endsWith('/sync/files/render') &&
          response.request().postDataJSON()?.repository !== undefined,
      );
      await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
      expect((await (await recoveredRender).json()).valid).toBe(true);
      expect(renderRequests.length).toBeGreaterThan(0);
    } finally {
      if (savedCorrection && original !== undefined) {
        const latest = (await (await page.request.get(overrideUrl)).json()) as SyncOverride;
        const repositoryId = overrideUrl.split('/repositories/')[1].split('/')[0];
        const restored = await page.request.put(`${panel.origin}/api/v1/targets/2001/settings`, {
          data: {
            sync_overrides: [
              {
                repository_id: repositoryId,
                kind: 'files',
                enabled: original.enabled,
                document: original.document,
                expected_revision: latest.revision,
              },
            ],
          },
        });
        expect(restored.ok()).toBe(true);
      }
      await page.close();
    }
  });

  it('retains overflow, underflow and signed-zero file numbers through render, reload and save', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1440, height: 1000 } });
    let original: SyncOverride | undefined;
    let overrideUrl = '';
    let repositoryId = '';
    try {
      await visit(page, `${panel.origin}/workspace/${panel.account}/sync/files/renovate.json`, {
        ready: '.file-editor',
      });
      const loaded = page.waitForResponse(
        (response) =>
          response.request().method() === 'GET' &&
          /\/repositories\/[^/]+\/sync\/files$/.test(response.url()),
      );
      await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
      const response = await loaded;
      original = (await response.json()) as SyncOverride;
      overrideUrl = response.url();
      repositoryId = new URL(overrideUrl).pathname.split('/').at(-3)!;
      const configured = await page.request.put(`${panel.origin}/api/v1/targets/2001/settings`, {
        data: {
          sync_overrides: [
            {
              repository_id: repositoryId,
              kind: 'files',
              enabled: original.enabled,
              expected_revision: original.revision,
              document: {
                ...original.document,
                merges: [
                  {
                    path: 'renovate.json',
                    strategy: 'deep-merge',
                    overrides: {
                      id: JSON.rawJSON('1e400'),
                      small: JSON.rawJSON('1e-400'),
                      zero: JSON.rawJSON('-0'),
                      data: { rawJSON: '1e400' },
                      flag: false,
                    },
                  },
                ],
              },
            },
          ],
        },
      });
      expect(configured.ok(), await configured.text()).toBe(true);
      await page.reload();
      const inspector = page.getByRole('dialog', { name: 'smyklot', exact: true });
      const editor = inspector.locator('.cm-content');
      const open = async () => {
        await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
        await expect.poll(() => editor.getAttribute('contenteditable')).toBe('true');
      };
      await open();
      const initial = await editor.innerText();
      expect(initial).toMatch(/"id":\s*1e400/u);
      expect(initial).toMatch(/"small":\s*1e-400/u);
      expect(initial).toMatch(/"zero":\s*-0/u);
      const preview = page.waitForResponse(
        (result) =>
          result.url().endsWith('/sync/files/render') &&
          (result.request().postData() ?? '').includes('"id":-1e400'),
      );
      await editor.fill(initial.replace(/"id":\s*1e400/u, '"id":-1e400'));
      const rendered = await preview;
      expect(rendered.ok()).toBe(true);
      const output = (await rendered.json()) as { valid: boolean; final_content: string };
      expect(output.valid).toBe(true);
      expect(output.final_content).toMatch(/"id":\s*-1e400/u);
      await inspector.getByRole('button', { name: 'Undo', exact: true }).click();
      expect(await editor.innerText()).toBe(initial);
      await editor.press('ControlOrMeta+Shift+Z');
      await expect.poll(() => editor.innerText()).toMatch(/"id":\s*-1e400/u);
      await expect
        .poll(() =>
          page.evaluate(() =>
            Object.keys(localStorage)
              .filter((key) => key.startsWith('smyklot.panel.settings-drafts.v1:'))
              .map((key) => localStorage.getItem(key))
              .join('\n'),
          ),
        )
        .toContain('"id":-1e400');
      await page.reload();
      await open();
      expect(await editor.innerText()).toMatch(/"id":\s*-1e400/u);
      expect(await editor.innerText()).toMatch(/"small":\s*1e-400/u);
      const correctedText = await editor.innerText();
      await editor.fill(correctedText.replace(/"id":\s*-1e400/u, '"id":null'));
      await inspector.getByText('These merge rules cannot store a new null field').waitFor();
      await page.reload();
      await open();
      expect(await editor.innerText()).toMatch(/"id":\s*null/u);
      await inspector.getByText('These merge rules cannot store a new null field').waitFor();
      const rejectedDirectory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (rejectedDirectory) await mkdir(rejectedDirectory, { recursive: true });
      for (const [colorScheme, width] of [
        ['light', 1440],
        ['dark', 1440],
        ['light', 375],
        ['dark', 375],
      ] as const) {
        await page.emulateMedia({ colorScheme });
        await page.setViewportSize({ width, height: 1000 });
        if (rejectedDirectory)
          await inspector.screenshot({
            path: join(rejectedDirectory, `rejected-null-${colorScheme}-${width}.png`),
            animations: 'disabled',
          });
        await expectDraftRecovery(inspector);
      }
      await page.emulateMedia({ colorScheme: 'light' });
      await page.setViewportSize({ width: 1440, height: 1000 });
      await inspector.getByRole('button', { name: 'Done', exact: true }).click();
      await expect
        .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isDisabled())
        .toBe(true);
      await open();
      await editor.fill(correctedText);
      await inspector.getByRole('button', { name: 'Done', exact: true }).click();
      const saved = page.waitForResponse(
        (result) => result.request().method() === 'PUT' && result.url().endsWith('/settings'),
      );
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const save = await saved;
      expect(save.ok(), await save.text()).toBe(true);
      expect(save.request().postData()).toContain('"id":-1e400');
      const readback = await page.request.get(overrideUrl);
      const body = await readback.text();
      expect(body).toContain('"id":-1e400');
      expect(body).toContain('"small":1e-400');
      expect(body).toContain('"zero":-0');
      expect(body).toContain('"data":{"rawJSON":"1e400"}');
      await page.reload();
      await open();
      expect(await editor.innerText()).toMatch(/"id":\s*-1e400/u);
      expect(await page.getByRole('button', { name: 'Save', exact: true }).count()).toBe(0);
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (directory !== undefined) {
        await mkdir(directory, { recursive: true });
        for (const [colorScheme, width] of [
          ['light', 1440],
          ['dark', 375],
        ] as const) {
          await page.emulateMedia({ colorScheme });
          await page.setViewportSize({ width, height: 1000 });
          await inspector.screenshot({
            path: join(directory, `numeric-literals-${colorScheme}-${width}.png`),
          });
        }
      }
      const checkpointId = ((await save.json()) as WorkspaceSettingsBatchResponse).checkpoint_id;
      expect(checkpointId).toBeDefined();
      const checkpointUrl = `${panel.origin}/api/v1/targets/2001/settings/checkpoints/${checkpointId}`;
      const checkpoint = (await (
        await page.request.get(checkpointUrl)
      ).json()) as SettingsCheckpoint;
      const adjustment = checkpoint.items.find(
        (item) => item.kind === 'sync_override' && item.repository_id === repositoryId,
      )!;
      expect(adjustment.after.state?.document.document).toContain('"id":-1e400');
      for (const state of ['before', 'after'] as const) {
        const latest = (await (await page.request.get(overrideUrl)).json()) as SyncOverride;
        const restored = await page.request.post(`${checkpointUrl}/restore`, {
          data: {
            state,
            selections: [
              {
                kind: 'sync_override',
                repository_id: repositoryId,
                sync_kind: 'files',
                expected_revision: latest.revision,
              },
            ],
          },
        });
        expect(restored.ok(), await restored.text()).toBe(true);
        const restoredBody = await (await page.request.get(overrideUrl)).text();
        expect(restoredBody).toContain(`"id":${state === 'before' ? '1e400' : '-1e400'}`);
        expect(restoredBody).toContain('"small":1e-400');
        expect(restoredBody).toContain('"zero":-0');
        expect(restoredBody).toContain('"data":{"rawJSON":"1e400"}');
      }
    } finally {
      if (original !== undefined && overrideUrl !== '') {
        const latest = (await (await page.request.get(overrideUrl)).json()) as SyncOverride;
        const restored = await page.request.put(`${panel.origin}/api/v1/targets/2001/settings`, {
          data: {
            sync_overrides: [
              {
                repository_id: repositoryId,
                kind: 'files',
                enabled: original.enabled,
                document: original.document,
                expected_revision: latest.revision,
              },
            ],
          },
        });
        expect(restored.ok()).toBe(true);
      }
      await page.close();
    }
  });

  it.each(['reopen', 'reload', 'unpin', 'hidden undo'] as const)(
    'preserves saved pin intent through %s',
    async (step) => {
      const page = await panel.browser.newPage({ viewport: { width: 1440, height: 1000 } });
      let original: SyncOverride | undefined;
      let repositoryId = '';
      let fixtureRevision: number | undefined;
      try {
        await visit(page, `${panel.origin}/workspace/${panel.account}/sync/files/renovate.json`, {
          ready: '.file-editor',
        });
        const loaded = page.waitForResponse(
          (response) =>
            response.request().method() === 'GET' &&
            /\/repositories\/[^/]+\/sync\/files$/.test(response.url()),
        );
        await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
        const response = await loaded;
        original = (await response.json()) as SyncOverride;
        repositoryId = new URL(response.url()).pathname.split('/').at(-3)!;
        const overrides =
          step === 'hidden undo'
            ? { automerge: false }
            : { automerge: false, timezone: 'Europe/Warsaw' };
        const configured = await page.request.put(`${panel.origin}/api/v1/targets/2001/settings`, {
          data: {
            sync_overrides: [
              {
                repository_id: repositoryId,
                kind: 'files',
                enabled: original.enabled,
                expected_revision: original.revision,
                document: {
                  ...original.document,
                  merges: (original.document.merges as Array<{ path: string }>).map((merge) =>
                    merge.path === 'renovate.json'
                      ? { path: 'renovate.json', strategy: 'deep-merge', overrides }
                      : merge,
                  ),
                },
              },
            ],
          },
        });
        expect(configured.ok()).toBe(true);
        fixtureRevision = (
          (await configured.json()) as WorkspaceSettingsBatchResponse
        ).sync_overrides?.find((entry) => entry.repository_id === repositoryId)?.revision;
        expect(fixtureRevision).toBeDefined();
        await page.reload();
        const inspector = page.getByRole('dialog', { name: 'smyklot', exact: true });
        const editor = inspector.locator('.cm-content');
        const open = async () => {
          await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
          await expect.poll(() => editor.getAttribute('contenteditable')).toBe('true');
        };
        const close = async () => {
          await inspector.getByRole('button', { name: 'Done', exact: true }).click();
          await inspector.waitFor({ state: 'hidden' });
        };
        await open();
        await editor.fill(
          (await editor.innerText()).replace('"automerge": false', '"automerge": true'),
        );
        await expect
          .poll(() => persistedMerge(page, repositoryId))
          .toMatchObject({ overrides: { automerge: true } });
        if (step === 'hidden undo') {
          const mounted = await editor.elementHandle();
          await inspector.getByRole('button', { name: 'View adjustment settings' }).click();
          await inspector.getByRole('button', { name: 'Undo', exact: true }).click();
          await inspector.getByRole('button', { name: 'Back to content' }).click();
          expect(await editor.evaluate((node, previous) => node === previous, mounted)).toBe(true);
          expect(JSON.parse(await editor.innerText()).automerge).toBe(false);
        } else if (step === 'unpin') {
          await inspector
            .getByRole('button', { name: 'Stop changing automerge', exact: true })
            .click();
          await expect
            .poll(() => persistedMerge(page, repositoryId))
            .toMatchObject({ overrides: { timezone: 'Europe/Warsaw' } });
          expect(
            ((await persistedMerge(page, repositoryId)) as { overrides: object }).overrides,
          ).not.toHaveProperty('automerge');
          await inspector.getByRole('button', { name: 'Undo', exact: true }).click();
          await expect
            .poll(() => persistedMerge(page, repositoryId))
            .toMatchObject({ overrides: { automerge: true } });
          await inspector.getByRole('button', { name: 'Undo', exact: true }).click();
          await expect.poll(() => persistedMerge(page, repositoryId)).toBeNull();
          await editor.focus();
          // Send the physical key so Shift produces uppercase Z on Linux too.
          await page.keyboard.press('ControlOrMeta+Shift+KeyZ');
          await expect
            .poll(() => persistedMerge(page, repositoryId))
            .toMatchObject({ overrides: { automerge: true } });
          await page.keyboard.press('ControlOrMeta+Shift+KeyZ');
          await expect
            .poll(
              async () =>
                ((await persistedMerge(page, repositoryId)) as { overrides: object })?.overrides,
            )
            .toEqual({ timezone: 'Europe/Warsaw' });
          await editor.fill(' ' + (await editor.innerText()));
          await expect
            .poll(
              async () =>
                ((await persistedMerge(page, repositoryId)) as { overrides: object })?.overrides,
            )
            .toEqual({ timezone: 'Europe/Warsaw' });
          await editor.fill((await editor.innerText()).replace('Europe/Warsaw', 'Europe/Paris'));
          await expect
            .poll(() => persistedMerge(page, repositoryId))
            .toMatchObject({ overrides: { timezone: 'Europe/Paris' } });
          await editor.fill((await editor.innerText()).replace('Europe/Paris', 'Europe/Warsaw'));
          await expect
            .poll(
              async () =>
                ((await persistedMerge(page, repositoryId)) as { overrides: object })?.overrides,
            )
            .toEqual({ timezone: 'Europe/Warsaw' });
          await close();
          await open();
          await editor.fill(
            (await editor.innerText()).replace('"automerge": false', '"automerge": true'),
          );
          await expect
            .poll(() => persistedMerge(page, repositoryId))
            .toMatchObject({ overrides: { automerge: true } });
          await editor.fill(
            (await editor.innerText()).replace('"automerge": true', '"automerge": false'),
          );
          await expect
            .poll(
              async () =>
                ((await persistedMerge(page, repositoryId)) as { overrides: object })?.overrides,
            )
            .toEqual({ timezone: 'Europe/Warsaw' });
          await close();
          expect(await page.getByRole('button', { name: 'Save', exact: true }).count()).toBe(1);
          return;
        } else {
          await close();
          if (step === 'reload') await page.reload();
          await open();
          expect(JSON.parse(await editor.innerText()).automerge).toBe(true);
          await editor.fill(
            (await editor.innerText()).replace('"automerge": true', '"automerge": false'),
          );
        }
        await expect.poll(() => persistedMerge(page, repositoryId)).toBeNull();
        await close();
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);
      } finally {
        if (original !== undefined && fixtureRevision !== undefined) {
          const restored = await page.request.put(`${panel.origin}/api/v1/targets/2001/settings`, {
            data: {
              sync_overrides: [
                {
                  repository_id: repositoryId,
                  kind: 'files',
                  enabled: original.enabled,
                  document: original.document,
                  expected_revision: fixtureRevision,
                },
              ],
            },
          });
          expect(restored.ok()).toBe(true);
        }
        await page.close();
      }
    },
  );

  it.each(['light', 'dark'] as const)(
    'preserves adjustment Undo while inspecting its stored override in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1000 },
        colorScheme,
      });
      const navigation: string[] = [];
      page.on('framenavigated', (frame) => {
        if (frame === page.mainFrame()) navigation.push(frame.url());
      });
      let initialOverride: SyncOverride | undefined;
      let overrideUrl = '';
      let savedRevision: number | undefined;
      try {
        await visit(page, `${panel.origin}/workspace/${panel.account}/sync/files/renovate.json`, {
          ready: '.file-editor',
        });
        const initialResponse = page.waitForResponse(
          (response) =>
            response.request().method() === 'GET' &&
            /\/repositories\/[^/]+\/sync\/files$/.test(response.url()),
        );
        await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
        const loaded = await initialResponse;
        initialOverride = (await loaded.json()) as SyncOverride;
        overrideUrl = loaded.url();
        const repositoryId = new URL(overrideUrl).pathname.split('/').at(-3)!;
        const inspector = page.getByRole('dialog', { name: 'smyklot', exact: true });
        const editor = inspector.locator('.cm-content');
        await expect.poll(() => editor.getAttribute('contenteditable')).toBe('true');
        const original = await editor.innerText();
        const edited = original.replace('"automerge": false', '"automerge": true');
        expect(edited).not.toBe(original);
        await editor.fill(edited);
        const undo = inspector.getByRole('button', { name: 'Undo', exact: true });
        await undo.waitFor();
        expect(await inspector.getByRole('alert').count()).toBe(0);
        const mounted = await editor.elementHandle();

        for (const width of [1440, 375]) {
          await page.setViewportSize({ width, height: 1000 });
          await inspector.getByRole('button', { name: 'View adjustment settings' }).click();
          const stored = inspector.locator('.code:visible');
          const toolbar = inspector.locator('.merge-pane-title');
          const title = toolbar.getByText('Adjustment settings', { exact: true });
          const readOnly = toolbar.getByText('Read only', { exact: true });
          expect(await readOnly.count()).toBe(1);
          const undoBox = await undo.boundingBox();
          for (const label of [title, readOnly]) {
            const box = await label.boundingBox();
            expect(box).not.toBeNull();
            expect(
              Math.abs(box!.y + box!.height / 2 - (undoBox!.y + undoBox!.height / 2)),
            ).toBeLessThanOrEqual(1);
          }
          expect(await stored.count()).toBe(1);
          expect(await stored.innerText()).toContain('"automerge": true');
          expect(await stored.innerText()).toContain('"strategy": "append"');
          expect(await inspector.locator('.code-editor:visible').count()).toBe(0);
          expect(await inspector.locator('.code-editor').count()).toBe(1);

          const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
          if (directory) {
            await mkdir(directory, { recursive: true });
            await inspector.screenshot({
              path: join(directory, `stored-override-${colorScheme}-${width}.png`),
              animations: 'disabled',
            });
          }
          await inspector.getByRole('button', { name: 'Back to content' }).click();
          expect(await toolbar.getByText('Read only', { exact: true }).count()).toBe(0);
          expect(await toolbar.getByText('Content adjustments', { exact: true }).count()).toBe(1);
          expect(await editor.evaluate((node, previous) => node === previous, mounted)).toBe(true);
          expect(await editor.innerText()).toBe(edited);
          expect(await inspector.locator('.code:visible').count()).toBe(0);
          expect(await inspector.locator('.code-editor:visible').count()).toBe(1);
          if (directory) {
            await inspector.screenshot({
              path: join(directory, `restored-adjustment-${colorScheme}-${width}.png`),
              animations: 'disabled',
            });
          }
        }

        await undo.click();
        expect(await editor.innerText()).toBe(original);
        expect(await inspector.getByRole('alert').count()).toBe(0);
        await inspector.getByRole('button', { name: 'Done', exact: true }).click();
        await inspector.waitFor({ state: 'hidden' });
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).count())
          .toBe(0);

        await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
        await expect.poll(() => editor.getAttribute('contenteditable')).toBe('true');
        await editor.fill(edited);
        await expect
          .poll(() =>
            page.evaluate((id) => {
              const key = Object.keys(localStorage).find((key) =>
                key.startsWith('smyklot.panel.settings-drafts.v1:'),
              );
              if (!key) return null;
              const document = JSON.parse(localStorage.getItem(key)!);
              const record = document.records.find(
                (record: { deleted: boolean; resource: { type: string; repositoryId?: string } }) =>
                  !record.deleted &&
                  record.resource.type === 'sync-override' &&
                  record.resource.repositoryId === id,
              );
              return record?.draft.document.merges?.find(
                (merge: { path: string }) => merge.path === 'renovate.json',
              );
            }, repositoryId),
          )
          .toMatchObject({
            path: 'renovate.json',
            overrides: { automerge: true },
            arrays: [{ path: '$.ignorePaths', strategy: 'append' }],
            deduplicate: true,
          });
        expect(await inspector.getByRole('alert').count()).toBe(0);
        await inspector.getByRole('button', { name: 'Done', exact: true }).click();
        await page.reload();
        await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
        await expect.poll(() => editor.getAttribute('contenteditable')).toBe('true');
        expect(JSON.parse(await editor.innerText())).toEqual(JSON.parse(edited));
        expect(await inspector.getByRole('alert').count()).toBe(0);
        await inspector.getByRole('button', { name: 'Done', exact: true }).click();
        const saved = page.waitForResponse(
          (response) =>
            response.request().method() === 'PUT' &&
            response.url().endsWith('/api/v1/targets/2001/settings'),
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        const response = await saved;
        expect(response.ok()).toBe(true);
        const result = (await response.json()) as WorkspaceSettingsBatchResponse;
        savedRevision = result.sync_overrides?.find(
          (entry) => entry.repository_id === repositoryId,
        )?.revision;
        expect(savedRevision).toBeGreaterThan(initialOverride.revision);
        const input = response.request().postDataJSON() as WorkspaceSettingsBatchInput;
        const change = input.sync_overrides?.find((entry) => entry.repository_id === repositoryId);
        expect(change?.document).toMatchObject({
          merges: expect.arrayContaining([
            expect.objectContaining({
              path: 'renovate.json',
              overrides: expect.objectContaining({ automerge: true }),
              arrays: [{ path: '$.ignorePaths', strategy: 'append' }],
              deduplicate: true,
            }),
          ]),
        });
        const otherMerges = (document: Record<string, unknown>) =>
          (document.merges as Array<{ path: string }>).filter(
            (merge) => merge.path !== 'renovate.json',
          );
        expect(otherMerges(change!.document)).toEqual(otherMerges(initialOverride.document));
        const readback = await page.request.get(overrideUrl);
        expect(readback.ok()).toBe(true);
        expect(((await readback.json()) as SyncOverride).document).toEqual(change?.document);
        await page.getByText('Settings saved', { exact: true }).waitFor();
      } catch (error) {
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory) {
          await mkdir(directory, { recursive: true });
          await page.screenshot({ path: join(directory, `failure-${colorScheme}.png`) });
          await writeFile(
            join(directory, `failure-${colorScheme}.json`),
            JSON.stringify({ navigation, text: await page.locator('body').innerText() }, null, 2),
          );
        }
        throw error;
      } finally {
        if (savedRevision !== undefined && initialOverride !== undefined) {
          const repositoryId = new URL(overrideUrl).pathname.split('/').at(-3)!;
          const restored = await page.request.put(`${panel.origin}/api/v1/targets/2001/settings`, {
            data: {
              sync_overrides: [
                {
                  repository_id: repositoryId,
                  kind: 'files',
                  enabled: initialOverride.enabled,
                  document: initialOverride.document,
                  expected_revision: savedRevision,
                },
              ],
            },
          });
          expect(restored.ok()).toBe(true);
        }
        await page.close();
      }
    },
  );

  it('renders shared and repository policies through the backend contract', async () => {
    const page: Page = await panel.browser.newPage({ viewport: { width: 1440, height: 1000 } });
    const crashes: string[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));

    try {
      await visit(page, `${panel.origin}/workspace/${panel.account}/sync/files/renovate.json`, {
        ready: '.file-editor',
      });

      const template = page.locator('.file-editor');
      const initial = await template.locator('.cm-content').innerText();
      expect(initial.endsWith('\n')).toBe(false);
      expect(await page.locator('.formatting-editor:visible').count()).toBe(0);
      await template.getByRole('button', { name: 'Template options' }).click();
      const options = page.getByRole('dialog', { name: 'Template options', exact: true });
      expect(await options.getByRole('group', { name: 'Final Newline' }).count()).toBe(0);
      await options
        .getByRole('group', { name: 'Arrays', exact: true })
        .getByRole('radio', { name: 'Expanded', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      await options.getByRole('button', { name: 'Done', exact: true }).click();
      await template
        .getByRole('radio', { name: 'Preview', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      await template.locator('.is-add').first().waitFor();
      expect(await template.locator('.code:visible').count()).toBe(1);
      expect(await template.locator('.code-editor:visible').count()).toBe(0);
      await template.getByRole('button', { name: 'Template options' }).click();
      await options.getByRole('button', { name: 'Apply formatting', exact: true }).click();
      await template.locator('.cm-content').waitFor({ state: 'visible' });
      expect(await template.locator('.cm-content').innerText()).not.toBe(initial);
      await template.getByRole('button', { name: 'Undo', exact: true }).click();
      expect(await template.locator('.cm-content').innerText()).toBe(initial);
      await template.locator('.cm-content').focus();
      await page.keyboard.press('Alt+Shift+f');
      await expect.poll(() => template.locator('.cm-content').innerText()).not.toBe(initial);
      await template
        .getByRole('radio', { name: 'Preview', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      await template
        .getByRole('radio', { name: 'Edit', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      await template.getByRole('button', { name: 'Undo', exact: true }).click();
      expect(await template.locator('.cm-content').innerText()).toBe(initial);

      await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
      const repository = page.getByRole('dialog', { name: 'smyklot', exact: true });
      expect(
        await repository
          .getByRole('radio', { name: 'Content adjustments', exact: true })
          .isChecked(),
      ).toBe(true);
      expect(
        await repository
          .locator('[name="repository-output-view"]')
          .evaluateAll((nodes) => nodes.map((n) => (n as HTMLInputElement).value)),
      ).toEqual(['content', 'preview']);
      await repository.locator('.cm-content').waitFor();
      const adjustment = await repository.locator('.cm-content').innerText();
      const changedAdjustment = adjustment.replace('"automerge": false', '"automerge": true');
      expect(changedAdjustment).not.toBe(adjustment);
      await repository.locator('.cm-content').fill(changedAdjustment);
      await repository.getByRole('button', { name: 'Undo', exact: true }).waitFor();
      await repository
        .getByRole('radio', { name: 'Final output', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      expect(await repository.locator('.code-editor:visible').count()).toBe(0);
      await repository
        .getByRole('radio', { name: 'Content adjustments', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      await repository.getByRole('button', { name: 'Undo', exact: true }).click();
      expect(await repository.locator('.cm-content').innerText()).toBe(adjustment);
      await repository
        .getByRole('radio', { name: 'Final output', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      await repository.locator('.rendered-output').waitFor({ state: 'visible', timeout: 30_000 });
      const exact = await repository.locator('.exact-output').innerText();
      expect(exact).toContain('"schedule": ["* 4 * * 6"]');
      expect(exact).toContain('"ignorePaths": ["crates/harness-codex-acp/**"]');
      expect(exact).toContain('\r\n');
      await repository
        .getByRole('button', { name: 'Repository file options', exact: true })
        .click();
      const formatting = repository.locator('.repository-formatting');
      const inspectorGeometry = await formatting.evaluate((node) => {
        const tabs = document.querySelector('.repository-view-tools')!;
        const box = node.getBoundingClientRect();
        return {
          top: box.top,
          tabsBottom: tabs.getBoundingClientRect().bottom,
          height: box.height,
          width: box.width,
          childWidth: node.firstElementChild!.getBoundingClientRect().width,
        };
      });
      expect(inspectorGeometry.top).toBeGreaterThanOrEqual(inspectorGeometry.tabsBottom + 15);
      expect(inspectorGeometry.height).toBeGreaterThan(400);
      expect(Math.abs(inspectorGeometry.width - inspectorGeometry.childWidth)).toBeLessThan(1);

      expect(
        await formatting
          .getByRole('group', { name: 'Line Ending' })
          .getByRole('radio', { name: 'CRLF' })
          .isChecked(),
      ).toBe(true);
      expect(
        await formatting
          .getByRole('region', { name: 'JSON', exact: true })
          .getByRole('group', { name: 'Arrays' })
          .getByRole('radio', { name: 'Compact' })
          .isChecked(),
      ).toBe(true);

      await repository.getByRole('button', { name: 'Done', exact: true }).click();
      await template.getByRole('button', { name: 'Template options' }).click();
      const editor = page.locator('.formatting-editor').first();
      expect(await editor.getByRole('region', { name: 'TOML', exact: true }).count()).toBe(0);
      await editor
        .getByRole('group', { name: 'Arrays', exact: true })
        .getByRole('radio', { name: 'Expanded', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      await page.waitForTimeout(300);

      const geometry = await editor.evaluate((editor) => {
        const controls = [...editor.querySelectorAll<HTMLElement>('.policy-row fieldset')];
        const width = (name: string, occurrence = 0): number =>
          controls
            .filter((control) => control.querySelector('legend')?.textContent?.trim() === name)
            .at(occurrence)
            ?.getBoundingClientRect().width ?? 0;
        const quoteSegments = [
          ...controls
            .filter((control) => control.querySelector('legend')?.textContent?.trim() === 'Arrays')
            .at(0)!
            .querySelectorAll<HTMLElement>('label'),
        ].map((label) => label.getBoundingClientRect().width);
        const wrapped = controls.flatMap((control) =>
          [...control.querySelectorAll<HTMLElement>('.band-trim')]
            .filter((label) => {
              const range = document.createRange();
              range.selectNodeContents(label);
              return range.getClientRects().length > 1;
            })
            .map((label) => label.textContent?.trim() ?? ''),
        );
        const quoteStyle = controls
          .filter((control) => control.querySelector('legend')?.textContent?.trim() === 'Arrays')
          .at(0)!;
        const thumb = quoteStyle.querySelector<HTMLElement>('.selection-indicator')!;
        const selected = quoteStyle
          .querySelector<HTMLInputElement>('input:checked')!
          .closest('label')!;
        const thumbBox = thumb.getBoundingClientRect();
        const selectedBox = selected.getBoundingClientRect();
        const segmentCorners = (style: CSSStyleDeclaration): string[] => [
          style.borderStartStartRadius,
          style.borderStartEndRadius,
          style.borderEndEndRadius,
          style.borderEndStartRadius,
        ];
        const quoteLabels = [...quoteStyle.querySelectorAll('label')];

        const selectedIndex = quoteLabels.indexOf(selected);

        return {
          arraysWidth: width('Arrays'),
          /* Each fill, with where it stands relative to the thumb, so the law can be
             asserted for every segment rather than for three named ones. */
          fills: quoteLabels.map((label, index) => {
            const style = getComputedStyle(label, '::before');
            return {
              corners: segmentCorners(style),
              first: index === 0,
              /* A logical inset, so this reads the same under RTL. */
              insets: [style.insetInlineStart, style.insetInlineEnd],
              last: index === quoteLabels.length - 1,
              selected: index === selectedIndex,
              /* Which side the thumb is on, if it is beside this segment at all. */
              thumbSide:
                index === selectedIndex - 1 ? 'end' : index === selectedIndex + 1 ? 'start' : null,
            };
          }),
          keyOrderWidth: width('Key Order'),
          quoteSegmentSpread: Math.max(...quoteSegments) - Math.min(...quoteSegments),
          thumbCorners: segmentCorners(getComputedStyle(thumb)),
          thumbLeftDelta: Math.abs(thumbBox.left - selectedBox.left),
          thumbWidthDelta: Math.abs(thumbBox.width - selectedBox.width),
          wrapped,
        };
      });
      expect(geometry.arraysWidth).toBeGreaterThan(geometry.keyOrderWidth);
      expect(geometry.quoteSegmentSpread).toBeGreaterThan(1);
      expect(geometry.thumbLeftDelta).toBeLessThanOrEqual(0.05);
      expect(geometry.thumbWidthDelta).toBeLessThanOrEqual(0.05);
      /* The thumb floats, so it is rounded on all four. It used to square the corners
         facing a neighbour and round only the pair facing the track's end, which is
         right for something that reaches that end and wrong everywhere else: at any
         option but the first or last it was square on both sides, and on a two-option
         control it sat mid-track with one squared edge. Read off the thumb rather than
         written down, so the number stays the stylesheet's to pick. */
      const [radius] = geometry.thumbCorners;
      expect(radius).not.toBe('0px');
      expect(geometry.thumbCorners).toEqual([radius, radius, radius, radius]);

      /* A fill squares the side facing a neighbour that DRAWS - the selected option,
         which wears the thumb - and runs on underneath it by that same radius, because
         the thumb is rounded and its curve leaves a wedge of bare track belonging to
         neither box. Everywhere else it keeps all four corners and bleeds nowhere: a
         segment whose neighbour draws nothing has nothing to be flush against, and a
         bleed under an ordinary neighbour would be a hover reaching into a segment
         nobody is pointing at.

         The selected option's own fill is excluded from both. It lies under the thumb
         and is invisible until the thumb itself is hovered, at which point a bleed is
         six pixels of hover reaching out past the thumb onto the option beside it.

         Corners read in logical order: start-start, start-end, end-end, end-start. */
      for (const [index, fill] of geometry.fills.entries()) {
        const facing = fill.selected ? null : fill.thumbSide;
        expect(
          fill.corners,
          `segment ${index} corners (thumb ${facing ?? 'not adjacent'})`,
        ).toEqual([
          facing === 'start' ? '0px' : radius,
          facing === 'end' ? '0px' : radius,
          facing === 'end' ? '0px' : radius,
          facing === 'start' ? '0px' : radius,
        ]);
        expect(fill.insets, `segment ${index} bleed`).toEqual([
          facing === 'start' ? `-${radius}` : '0px',
          facing === 'end' ? `-${radius}` : '0px',
        ]);
      }
      expect(
        geometry.fills.some((fill) => fill.thumbSide !== null),
        'no segment was adjacent to the thumb, so the bleed rule went unchecked',
      ).toBe(true);
      expect(geometry.wrapped).toEqual([]);
      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });

  it('uses one shared pressed surface for sync rows in both themes', async () => {
    for (const colorScheme of ['light', 'dark'] as const) {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1000 },
        colorScheme,
      });
      try {
        for (const route of ['sync', 'sync/files', 'sync/files/renovate.json', 'sync/rulesets']) {
          await visit(page, `${panel.origin}/workspace/${panel.account}/${route}`, {
            ready: '.object-row',
          });
          const row = page
            .locator('.object-row')
            .filter({ has: page.locator('.row-hit') })
            .first();
          const direct = page.locator('a.object-row, button.object-row').first();
          const hit = (await row.count()) > 0 ? row.locator('.row-hit') : direct;
          await hit.scrollIntoViewIfNeeded();
          await hit.hover();
          const read = () =>
            hit.evaluate((node) => {
              const row = node.closest('.object-row')!;
              const style = getComputedStyle(row);
              const target = getComputedStyle(node);
              return {
                background: style.backgroundColor,
                image: style.backgroundImage,
                shadow: style.boxShadow,
                translate: style.translate,
                child:
                  row === node
                    ? null
                    : {
                        background: target.backgroundColor,
                        image: target.backgroundImage,
                        shadow: target.boxShadow,
                        translate: target.translate,
                      },
              };
            });
          await page.waitForTimeout(200);
          const hover = await read();
          await page.mouse.down();
          await page.waitForTimeout(200);
          const active = await read();
          expect(active.background, route).not.toBe(hover.background);
          expect(active.image, route).toBe('none');
          expect(active.shadow, route).not.toBe('none');
          expect(active.translate, route).toBe('0px 1px');
          if (active.child !== null)
            expect(active.child, route).toEqual({
              background: 'rgba(0, 0, 0, 0)',
              image: 'none',
              shadow: 'none',
              translate: 'none',
            });
          await page.screenshot({
            path: `../../../.bart/sync-redesign/after/pressed-${route.replaceAll('/', '-')}-${colorScheme}.png`,
          });
          await page.mouse.move(0, 0);
          await page.mouse.up();
        }
      } finally {
        await page.close();
      }
    }
  });

  it('adds an unsaved template, renders it and saves the complete draft', async () => {
    const page = await panel.browser.newPage({ viewport: { width: 1280, height: 900 } });
    try {
      await visit(page, `${panel.origin}/workspace/${panel.account}/sync/files`, { ready: 'h1' });
      await page.getByRole('button', { name: 'Add a file', exact: true }).click();
      await page
        .getByPlaceholder('renovate.json, or a path no repository has yet')
        .fill('new-preview.json');
      await page.getByRole('option', { name: /Start new-preview.json/ }).click();
      await page.getByRole('heading', { name: 'new-preview.json', exact: true }).waitFor();
      await page.locator('.cm-content').fill('{"hello":"world"}');
      await page
        .getByRole('radio', { name: 'Preview', exact: true })
        .locator('xpath=ancestor::label[1]')
        .click();
      await page
        .getByRole('region', { name: 'Read-only final output with highlighted changes' })
        .waitFor();
      const saved = page.waitForResponse(
        (response) =>
          response.request().method() === 'PUT' &&
          response.url().endsWith('/api/v1/targets/2001/settings'),
      );
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const response = await saved;
      expect(response.ok()).toBe(true);
      const body = response.request().postDataJSON() as {
        sync_configs: Array<{ document: { files: Array<{ path: string; content: string }> } }>;
      };
      expect(
        body.sync_configs
          .flatMap((c) => c.document.files)
          .find((f) => f.path === 'new-preview.json')?.content,
      ).toBe('{"hello":"world"}\n');
      await page.getByText('Settings saved', { exact: true }).waitFor();
      await page.reload();
      await page.getByRole('heading', { name: 'new-preview.json', exact: true }).waitFor();
      expect(await page.locator('.cm-content').innerText()).toContain('"hello":"world"');
    } finally {
      await page.close();
    }
  });

  for (const width of [375, 768, 1024, 1440]) {
    for (const colorScheme of ['light', 'dark'] as const) {
      it(`keeps the file inspector usable at ${width}px in ${colorScheme}`, async () => {
        const page = await panel.browser.newPage({ viewport: { width, height: 900 }, colorScheme });
        try {
          await visit(page, `${panel.origin}/workspace/${panel.account}/sync/files/renovate.json`, {
            ready: 'h1',
          });
          expect(await page.locator('.page-eyebrow').count()).toBe(0);
          const editor = page.locator('.file-editor');
          const geometryOfEditor = () =>
            editor.evaluate((node) => {
              const controls = [...node.querySelectorAll('fieldset, .icon-button, .btn')].map((e) =>
                e.getBoundingClientRect(),
              );
              const code =
                node.querySelector('.file-preview .code') ?? node.querySelector('.code-editor');
              return {
                heights: controls.map((r) => r.height),
                centers: controls.map((r) => r.top + r.height / 2),
                codeTop: code!.getBoundingClientRect().top,
              };
            });
          const editing = await geometryOfEditor();
          expect(Math.max(...editing.heights) - Math.min(...editing.heights)).toBeLessThan(0.05);
          expect(Math.max(...editing.centers) - Math.min(...editing.centers)).toBeLessThan(0.05);
          await page.screenshot({
            path: `../../../.bart/sync-redesign/after/editor-${width}-${colorScheme}.png`,
          });
          await editor
            .getByRole('radio', { name: 'Preview', exact: true })
            .locator('xpath=ancestor::label[1]')
            .click();
          await editor.locator('.file-preview .code').waitFor();
          const preview = await geometryOfEditor();
          expect(Math.abs(editing.codeTop - preview.codeTop)).toBeLessThan(0.05);
          expect(await editor.locator('.code:visible').count()).toBe(1);
          await page.screenshot({
            path: `../../../.bart/sync-redesign/after/preview-${width}-${colorScheme}.png`,
          });
          await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
          const dialog = page.getByRole('dialog', { name: 'smyklot', exact: true });
          expect(
            await dialog
              .getByRole('radio', { name: 'Content adjustments', exact: true })
              .isChecked(),
          ).toBe(true);
          await dialog.evaluate(async (node) => {
            await Promise.all(
              node.getAnimations().map((animation) => animation.finished.catch(() => {})),
            );
          });
          await page.screenshot({
            path: `../../../.bart/sync-redesign/after/adjustment-${width}-${colorScheme}.png`,
          });
          await dialog
            .getByRole('radio', { name: 'Final output', exact: true })
            .locator('xpath=ancestor::label[1]')
            .click();
          await dialog.locator('.rendered-output').waitFor();
          const toolbar = await dialog.locator('.repository-view-tools').evaluate((node) => {
            const controls = [...node.children].map((e) => e.getBoundingClientRect());
            return {
              width: node.getBoundingClientRect().width,
              segment: controls[0].width,
              heights: controls.map((r) => r.height),
              centers: controls.map((r) => r.top + r.height / 2),
            };
          });
          expect(toolbar.segment).toBeLessThan(toolbar.width - 30);
          expect(Math.max(...toolbar.heights) - Math.min(...toolbar.heights)).toBeLessThan(0.05);
          expect(Math.max(...toolbar.centers) - Math.min(...toolbar.centers)).toBeLessThan(0.05);

          await dialog
            .getByRole('button', { name: 'Repository file options', exact: true })
            .click();
          await dialog.getByRole('region', { name: 'Common', exact: true }).waitFor();
          const geometry = await dialog.evaluate((node) => {
            const box = node.getBoundingClientRect();
            const body = node.querySelector('.modal-body')!;
            const formatting = node.querySelector('.formatting-editor')!.getBoundingClientRect();
            const tabs = node.querySelector('.repository-view-tools')!.getBoundingClientRect();
            return {
              left: box.left,
              right: box.right,
              width: innerWidth,
              overflow: body.scrollWidth - body.clientWidth,
              overflowing: [...body.querySelectorAll<HTMLElement>('*')]
                .filter(
                  (element) =>
                    element.getBoundingClientRect().right > body.getBoundingClientRect().right + 1,
                )
                .map(
                  (element) =>
                    `${element.tagName}.${element.className}: ${element.getBoundingClientRect().right}`,
                ),
              start: formatting.top - tabs.bottom,
              contentHeight: formatting.height,
            };
          });
          expect(geometry.left).toBeGreaterThanOrEqual(0);
          expect(geometry.right).toBeLessThanOrEqual(geometry.width + 1);
          await page.screenshot({
            path: `../../../.bart/sync-redesign/after/inspector-${width}-${colorScheme}.png`,
          });
          expect(geometry.overflow, geometry.overflowing.join('\n')).toBeLessThanOrEqual(1);
          expect(geometry.start).toBeGreaterThanOrEqual(15);
          expect(geometry.contentHeight).toBeGreaterThan(400);
          await page.keyboard.press('Escape');
          await dialog.waitFor({ state: 'hidden' });
          expect(
            await page
              .getByRole('button', { name: 'Open output for smyklot', exact: true })
              .evaluate((node) => document.activeElement === node),
          ).toBe(true);
        } finally {
          await page.close();
        }
      });
    }
  }
});
