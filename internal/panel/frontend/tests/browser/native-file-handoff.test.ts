import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';
import { NATIVE_FILE_VARIANTS } from '../../dev/native-files';
import type { SyncOverride } from '../../src/lib/types';
import { startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

const sharedPath = (path: string) =>
  `/workspace/${panel.account}/sync/files/${path.split('/').map(encodeURIComponent).join('/')}`;
const overrideUrl = () => `${panel.origin}/api/v1/targets/2001/repositories/4001/sync/files`;

async function openOutput(page: Page) {
  await page.getByRole('searchbox', { name: 'Find a repository output' }).fill('smyklot');
  await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'smyklot', exact: true });
  await dialog.getByRole('link', { name: 'Edit adjustments', exact: true }).waitFor();
  return dialog;
}

async function restore(page: Page, original: SyncOverride) {
  const latest = await (await page.request.get(overrideUrl())).json();
  const response = await page.request.put(`${panel.origin}/api/v1/targets/2001/settings`, {
    data: {
      sync_overrides: [
        {
          repository_id: '4001',
          kind: 'files',
          enabled: original.enabled,
          document: original.document,
          expected_revision: latest.revision,
        },
      ],
    },
  });
  expect(response.ok(), await response.text()).toBe(true);
}

describe('native file adjustment handoff [Browser]', () => {
  it('ignores late handoff validation after a new editor corrects the draft', async () => {
    const page = await panel.browser.newPage({
      viewport: { width: 1440, height: 1100 },
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(10_000);
    const { file } = NATIVE_FILE_VARIANTS.find(({ format }) => format === 'yaml')!;
    const invalid = 'unfinished-handoff: [\n';
    let release!: () => void;
    const gate = new Promise<void>((resolve) => (release = resolve));
    const held: Promise<void>[] = [];
    let received = 0;
    page.on('response', (response) => {
      if (
        response.url().endsWith('/sync/files/render') &&
        response.request().postDataJSON()?.draft_content === invalid
      )
        received += 1;
    });
    await page.route('**/sync/files/render', async (route) => {
      if (route.request().postDataJSON()?.draft_content !== invalid) return route.continue();
      let finished!: () => void;
      held.push(new Promise<void>((resolve) => (finished = resolve)));
      await gate;
      await route.fulfill({
        json: {
          valid: false,
          final_content: '',
          matches_formatting: false,
          diagnostics: [
            { stage: 'parse', code: 'invalid_template', message: 'The old template was invalid' },
          ],
        },
      });
      finished();
    });
    try {
      await visit(page, `${panel.origin}${sharedPath(file.path)}`, { ready: '.file-editor' });
      const template = page.locator('.file-editor .cm-content').first();
      await template.fill(invalid.trimEnd());
      const dialog = await openOutput(page);
      await dialog.getByRole('link', { name: 'Edit adjustments', exact: true }).click();
      await dialog.getByRole('button', { name: 'Opening editor…' }).waitFor();
      await expect.poll(() => held.length).toBeGreaterThan(0);
      await dialog.getByRole('button', { name: 'Done', exact: true }).click();
      await page.locator(`a[href="/workspace/${panel.account}/sync/files"]`).first().click();
      await page.getByRole('link', { name: file.path }).click();
      await template.waitFor();
      expect(await template.innerText()).toContain('unfinished-handoff');
      await template.fill('enabled: true\nretries: 6');
      const save = page.getByRole('button', { name: 'Save', exact: true });
      await expect.poll(() => save.isEnabled(), { timeout: 5000 }).toBe(true);
      release();
      await Promise.all(held);
      await expect.poll(() => received).toBe(held.length);
      await page.evaluate(
        () =>
          new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          ),
      );
      expect(await save.isEnabled()).toBe(true);
      expect(await page.getByText('The old template was invalid', { exact: true }).count()).toBe(0);
      expect(decodeURIComponent(new URL(page.url()).pathname)).toBe(sharedPath(file.path));
    } finally {
      release();
      await page.close();
    }
  });

  it('creates the first adjustment for the requested file only after the user chooses Add', async () => {
    const page = await panel.browser.newPage({
      viewport: { width: 375, height: 1100 },
      reducedMotion: 'reduce',
    });
    page.setDefaultTimeout(10_000);
    let original: SyncOverride | undefined;
    try {
      const path = '.github/workflows/ci.yaml';
      await visit(page, `${panel.origin}${sharedPath(path)}`, { ready: '.file-editor' });
      original = await (await page.request.get(overrideUrl())).json();
      await (
        await openOutput(page)
      )
        .getByRole('link', { name: 'Edit adjustments', exact: true })
        .click();
      const offered = page.getByRole('group', { name: `Adjustment for ${path}`, exact: true });
      await expect
        .poll(() => offered.evaluate((node) => node === document.activeElement))
        .toBe(true);
      expect(await page.locator('.settings-composer').count()).toBe(0);
      await offered.getByRole('button', { name: 'Add adjustment', exact: true }).click();
      const adjustment = page.getByRole('article', { name: `Adjustment for ${path}`, exact: true });
      await expect
        .poll(() => adjustment.evaluate((node) => node === document.activeElement))
        .toBe(true);
      expect(
        await adjustment.getByRole('textbox', { name: 'File', exact: true }).inputValue(),
      ).toBe(path);
      await adjustment.locator('.cm-content').fill('{"name":"Local checks"}');
      await expect
        .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
        .toBe(true);
      const saved = page.waitForResponse(
        (response) => response.request().method() === 'PUT' && response.url().endsWith('/settings'),
      );
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const response = await saved;
      expect(response.ok(), await response.text()).toBe(true);
      expect(response.request().postDataJSON().sync_overrides[0].document.merges.at(-1)).toEqual({
        path,
        overrides: { name: 'Local checks' },
      });
      await page.reload();
      await adjustment.locator('.cm-content').waitFor();
      expect(await adjustment.locator('.cm-content').innerText()).toContain('Local checks');
    } finally {
      if (original) await restore(page, original);
      await page.close();
    }
  });

  it.each([
    { path: '.config/quality.toml', colorScheme: 'light', width: 1440 },
    { path: '.github/workflows/ci.yaml', colorScheme: 'dark', width: 375 },
  ] as const)(
    'offers inspection without mutation for a reader: $path',
    async ({ path, colorScheme, width }) => {
      const page = await panel.browser.newPage({
        colorScheme,
        viewport: { width, height: 1100 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      const writes: string[] = [];
      page.on('request', (request) => {
        if (request.method() === 'PUT' && request.url().endsWith('/settings'))
          writes.push(request.url());
      });
      try {
        await page.route('**/api/v1/targets', async (route) => {
          const response = await route.fetch();
          const body = await response.json();
          for (const target of body.targets) target.capabilities.write = false;
          await route.fulfill({ response, json: body });
        });
        await visit(page, `${panel.origin}${sharedPath(path)}`, { ready: '.file-editor' });
        await page.getByRole('searchbox', { name: 'Find a repository output' }).fill('smyklot');
        await page.getByRole('button', { name: 'Open output for smyklot', exact: true }).click();
        const dialog = page.getByRole('dialog', { name: 'smyklot', exact: true });
        await dialog.getByRole('link', { name: 'Inspect adjustments', exact: true }).click();
        const target = page.locator(`[aria-label="Adjustment for ${path}"]`);
        await expect
          .poll(() => target.evaluate((node) => node === document.activeElement))
          .toBe(true);
        expect(
          await target.getByRole('button', { name: 'Add adjustment', exact: true }).count(),
        ).toBe(0);
        const inputs = await target.locator('input').all();
        for (const input of inputs) expect(await input.isDisabled()).toBe(true);
        expect(await page.locator('.settings-composer').count()).toBe(0);
        expect(writes).toEqual([]);
        const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
        if (directory)
          await page.screenshot({ path: join(directory, `reader-${colorScheme}-${width}.png`) });
      } finally {
        await page.close();
      }
    },
  );

  it.each(NATIVE_FILE_VARIANTS)(
    'edits, previews, saves and reloads $format through real SPA navigation',
    async ({ format, file }) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1440, height: 1100 },
        reducedMotion: 'reduce',
      });
      page.setDefaultTimeout(10_000);
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      let original: SyncOverride | undefined;
      try {
        await visit(page, `${panel.origin}${sharedPath(file.path)}`, { ready: '.file-editor' });
        original = await (await page.request.get(overrideUrl())).json();
        const template = page.locator('.file-editor .cm-content').first();
        if (format === 'yaml') await template.fill(`${file.content}# Unsaved workspace note`);
        await page.evaluate(() => {
          (window as unknown as { handoffMarker: number }).handoffMarker = 17;
        });
        const dialog = await openOutput(page);
        await dialog.getByRole('link', { name: 'Edit adjustments', exact: true }).click();
        const adjustment = page.getByRole('article', {
          name: `Adjustment for ${file.path}`,
          exact: true,
        });
        await adjustment.waitFor();
        await expect
          .poll(() => adjustment.evaluate((node) => document.activeElement === node))
          .toBe(true);
        expect(
          await page.evaluate(() => (window as unknown as { handoffMarker: number }).handoffMarker),
        ).toBe(17);
        expect(
          await adjustment.getByRole('textbox', { name: 'File', exact: true }).inputValue(),
        ).toBe(file.path);
        if (format === 'yaml') {
          const pathInput = await adjustment
            .getByRole('textbox', { name: 'File', exact: true })
            .elementHandle();
          await pathInput!.fill('.config/moved.yaml');
          expect(
            await page
              .getByRole('group', { name: `Adjustment for ${file.path}`, exact: true })
              .count(),
          ).toBe(0);
          await pathInput!.fill(file.path);
          await page.evaluate(
            () =>
              new Promise<void>((resolve) =>
                requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
              ),
          );
          expect(
            await pathInput!.evaluate((node) => ({
              connected: node.isConnected,
              focused: document.activeElement === node,
            })),
          ).toEqual({ connected: true, focused: true });
        }
        const editor = adjustment.locator('.cm-content');
        await editor.fill(format === 'markdown' ? 'Run five local checks' : '{"retries":5}');
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        await page.goBack();
        await page.getByRole('heading', { name: file.path, exact: true }).waitFor();
        expect(
          await page.evaluate(() => (window as unknown as { handoffMarker: number }).handoffMarker),
        ).toBe(17);
        const rendered = page.waitForResponse(
          (response) =>
            response.url().endsWith('/sync/files/render') &&
            response.request().postDataJSON()?.repository?.id === '4001',
        );
        const reopened = await openOutput(page);
        const result = await (await rendered).json();
        expect(result.valid, JSON.stringify(result.diagnostics)).toBe(true);
        if (format === 'yaml') expect(result.final_content).toContain('# Unsaved workspace note');
        expect(result.final_content).toContain(
          format === 'markdown' ? 'Run five local checks' : '5',
        );
        await reopened
          .getByRole('radio', { name: 'Final output', exact: true })
          .locator('xpath=ancestor::label[1]')
          .click();
        expect(await reopened.locator('.code:visible, .code-editor:visible').count()).toBe(1);
        expect(await reopened.locator('.exact-output').innerText()).toContain(
          format === 'markdown' ? 'Run five local checks' : '5',
        );
        await reopened.getByRole('button', { name: 'Done', exact: true }).click();
        if (format === 'yaml') {
          expect(await template.innerText()).toContain('# Unsaved workspace note');
          await template.fill(file.content.trimEnd());
        }
        const saved = page.waitForResponse(
          (response) =>
            response.request().method() === 'PUT' && response.url().endsWith('/settings'),
        );
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        const response = await saved;
        expect(response.ok(), await response.text()).toBe(true);
        const merge = response
          .request()
          .postDataJSON()
          .sync_overrides[0].document.merges.find(
            (row: { path: string }) => row.path === file.path,
          );
        expect(format === 'markdown' ? merge.sections[0].content : merge.overrides.retries).toBe(
          format === 'markdown' ? 'Run five local checks' : 5,
        );
        await page.reload();
        const reloaded = await openOutput(page);
        await reloaded.getByRole('link', { name: 'Edit adjustments', exact: true }).click();
        await editor.waitFor();
        expect(await editor.innerText()).toContain(
          format === 'markdown' ? 'Run five local checks' : '5',
        );
        expect(await page.locator('.settings-composer').count()).toBe(0);
        expect(errors).toEqual([]);
      } finally {
        if (original) await restore(page, original);
        await page.close();
      }
    },
  );

  it('returns an invalid raw draft to the same editor, preserving exact digits through correction and save', async () => {
    const page = await panel.browser.newPage();
    page.setDefaultTimeout(10_000);
    let original: SyncOverride | undefined;
    try {
      const path = '.config/quality.toml';
      await visit(page, `${panel.origin}${sharedPath(path)}`, { ready: '.file-editor' });
      original = await (await page.request.get(overrideUrl())).json();
      await (
        await openOutput(page)
      )
        .getByRole('link', { name: 'Edit adjustments', exact: true })
        .click();
      const editor = page
        .getByRole('article', { name: `Adjustment for ${path}`, exact: true })
        .locator('.cm-content');
      await editor.fill('{"retries":9007199254740993,');
      await page.goBack();
      const dialog = await openOutput(page);
      await dialog
        .getByText("Finish this adjustment in the repository's File sync settings")
        .waitFor();
      expect(await dialog.locator('.code').innerText()).toContain('9007199254740993,');
      await dialog.getByRole('link', { name: 'Edit adjustments', exact: true }).click();
      await editor.waitFor();
      expect(await editor.innerText()).toBe('{"retries":9007199254740993,');
      await editor.fill('{"retries":9007199254740993}');
      await expect
        .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
        .toBe(true);
      const saved = page.waitForResponse(
        (response) => response.request().method() === 'PUT' && response.url().endsWith('/settings'),
      );
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      const response = await saved;
      expect(response.ok(), await response.text()).toBe(true);
      expect(response.request().postData()).toContain('"retries":9007199254740993');
      await page.reload();
      await editor.waitFor();
      expect(await editor.innerText()).toContain('9007199254740993');
    } finally {
      if (original) await restore(page, original);
      await page.close();
    }
  });

  it.each(['light', 'dark'] as const)(
    'keeps native handoffs and their selected editors coherent in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({ colorScheme, reducedMotion: 'reduce' });
      page.setDefaultTimeout(10_000);
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      if (directory) await mkdir(directory, { recursive: true });
      try {
        for (const width of [375, 768, 1024, 1440]) {
          await page.setViewportSize({ width, height: 1100 });
          for (const { format, file } of NATIVE_FILE_VARIANTS) {
            await visit(page, `${panel.origin}${sharedPath(file.path)}`, { ready: '.file-editor' });
            const dialog = await openOutput(page);
            await expect.poll(() => dialog.locator('.is-rendering').count()).toBe(0);
            expect(await dialog.locator('.code:visible, .code-editor:visible').count()).toBe(1);
            expect(
              await dialog.evaluate((node) => node.scrollWidth - node.clientWidth),
            ).toBeLessThanOrEqual(1);
            const status = dialog.locator('.merge-pane-label .setting-unmanaged');
            if (await status.count()) {
              const title = (await dialog.locator('.merge-pane-label .t').boundingBox())!;
              const readOnly = (await status.boundingBox())!;
              expect(
                Math.abs(title.y + title.height / 2 - readOnly.y - readOnly.height / 2),
              ).toBeLessThanOrEqual(1);
            }
            if (directory)
              await dialog.screenshot({
                path: join(directory, `handoff-${format}-${colorScheme}-${width}.png`),
              });
            await dialog.getByRole('link', { name: 'Edit adjustments', exact: true }).click();
            const adjustment = page.getByRole('article', {
              name: `Adjustment for ${file.path}`,
              exact: true,
            });
            await expect
              .poll(() => adjustment.evaluate((node) => document.activeElement === node))
              .toBe(true);
            const geometry = await adjustment.evaluate((node) => ({
              top: node.getBoundingClientRect().top,
              topbar: document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0,
              overflow: document.documentElement.scrollWidth - innerWidth,
              editorCount: node.querySelectorAll('.code-editor').length,
              headerBottom: node.querySelector('.file-heading')!.getBoundingClientRect().bottom,
              expectedTop:
                Math.max(
                  0,
                  document.querySelector('.top-bar')?.getBoundingClientRect().bottom ?? 0,
                ) + parseFloat(getComputedStyle(node).scrollMarginBlockStart),
              scrollRemaining: document.documentElement.scrollHeight - innerHeight - scrollY,
            }));
            if (directory && geometry.overflow > 1) {
              await page.screenshot({
                path: join(directory, `overflow-${format}-${colorScheme}-${width}.png`),
              });
              const offenders = await page.evaluate(() =>
                [...document.querySelectorAll('*')].flatMap((node) => {
                  const bounds = node.getBoundingClientRect();
                  return bounds.right > innerWidth + 1
                    ? [
                        {
                          tag: node.tagName,
                          classes: node.className,
                          text: node.textContent?.slice(0, 100),
                          bounds,
                          display: getComputedStyle(node).display,
                          minWidth: getComputedStyle(node).minWidth,
                          width: getComputedStyle(node).width,
                        },
                      ]
                    : [];
                }),
              );
              await writeFile(
                join(directory, `overflow-${format}-${colorScheme}-${width}.json`),
                JSON.stringify(offenders, null, 2),
              );
            }
            expect(geometry.top).toBeGreaterThanOrEqual(geometry.topbar);
            expect(geometry.headerBottom).toBeLessThanOrEqual(1100);
            expect(
              Math.abs(geometry.top - geometry.expectedTop) <= 1 || geometry.scrollRemaining <= 1,
            ).toBe(true);
            expect(geometry.overflow).toBeLessThanOrEqual(1);
            expect(geometry.editorCount).toBe(1);
            if (directory)
              await page.screenshot({
                path: join(directory, `editor-${format}-${colorScheme}-${width}.png`),
              });
            if (format === 'markdown') {
              const controls = await adjustment.locator('.section-action-row').evaluate((node) => {
                const picker = node.querySelector('[role="combobox"]')!.getBoundingClientRect();
                const remove = node
                  .querySelector('button[aria-label^="Remove section"]')!
                  .getBoundingClientRect();
                return {
                  pickerHeight: picker.height,
                  removeWidth: remove.width,
                  removeHeight: remove.height,
                  trailing: node.getBoundingClientRect().right - remove.right,
                  center: (picker.top + picker.bottom - remove.top - remove.bottom) / 2,
                  sectionGap:
                    node
                      .closest('.sync-merge')!
                      .querySelector(':scope > .form-row')!
                      .getBoundingClientRect().top -
                    node
                      .closest('.sync-merge')!
                      .querySelector('.file-editor')!
                      .getBoundingClientRect().bottom,
                };
              });
              expect(controls.pickerHeight).toBe(34);
              expect(controls.removeWidth).toBe(34);
              expect(controls.removeHeight).toBe(34);
              expect(Math.abs(controls.center)).toBeLessThanOrEqual(1);
              expect(Math.abs(controls.trailing)).toBeLessThanOrEqual(1);
              expect(controls.sectionGap).toBe(16);
              await adjustment.getByRole('combobox', { name: /Action for section 1/ }).click();
              expect(await page.getByRole('option').count()).toBe(7);
              const lastOption = await page
                .getByRole('option', { name: 'Prepend to document', exact: true })
                .boundingBox();
              expect(lastOption!.y + lastOption!.height).toBeLessThanOrEqual(1100);
              if (directory)
                await page.screenshot({
                  path: join(directory, `menu-markdown-${colorScheme}-${width}.png`),
                });
              await page.keyboard.press('Escape');
            }
          }
        }
      } finally {
        await page.close();
      }
    },
  );

  it('renders the existing Markdown replacement fixture and keeps its controls inside the card', async () => {
    const page = await panel.browser.newPage({ reducedMotion: 'reduce' });
    page.setDefaultTimeout(10_000);
    const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
    if (directory) await mkdir(directory, { recursive: true });
    try {
      for (const colorScheme of ['light', 'dark'] as const) {
        await page.emulateMedia({ colorScheme });
        for (const width of [375, 768, 1024, 1440]) {
          await page.setViewportSize({ width, height: 1100 });
          await visit(page, `${panel.origin}${sharedPath('CONTRIBUTING.md')}`, {
            ready: '.file-editor',
          });
          const rendered = page.waitForResponse(
            (response) =>
              response.url().endsWith('/sync/files/render') &&
              response.request().postDataJSON()?.repository?.id === '4001',
          );
          const dialog = await openOutput(page);
          const result = await (await rendered).json();
          expect(result.valid, JSON.stringify(result.diagnostics)).toBe(true);
          expect(result.final_content).toContain(
            'Run mise run check before opening a pull request',
          );
          expect(result.final_content).toContain('Squash on merge');
          await dialog.getByRole('link', { name: 'Edit adjustments', exact: true }).click();
          const adjustment = page.getByRole('article', {
            name: 'Adjustment for CONTRIBUTING.md',
            exact: true,
          });
          await expect
            .poll(() => adjustment.evaluate((node) => document.activeElement === node))
            .toBe(true);
          expect(
            await adjustment.getByRole('textbox', { name: 'Find', exact: true }).inputValue(),
          ).toBe('make check');
          expect(
            await adjustment
              .getByRole('textbox', { name: 'Replace with', exact: true })
              .inputValue(),
          ).toBe('mise run check');
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth - innerWidth),
          ).toBeLessThanOrEqual(1);
          const geometry = await adjustment.evaluate((node) => {
            const rect = (selector: string) =>
              node.querySelector(selector)!.getBoundingClientRect();
            const actions = [...node.querySelectorAll('.section-action-row')].map((row) => {
              const label = row.querySelector('.setting-name')!.getBoundingClientRect();
              const picker = row.querySelector('[role=combobox]')!.getBoundingClientRect();
              return { below: picker.top >= label.bottom, offset: picker.top - label.top };
            });
            const replacement = rect('input[placeholder="mise run check"]');
            const remove = rect('button[aria-label^="Remove replacement"]');
            return {
              actions,
              replacementCenter:
                (replacement.top + replacement.bottom - remove.top - remove.bottom) / 2,
              trailing: node.getBoundingClientRect().right - remove.right,
            };
          });
          expect(geometry.actions[0].below).toBe(geometry.actions[1].below);
          expect(
            Math.abs(geometry.actions[0].offset - geometry.actions[1].offset),
          ).toBeLessThanOrEqual(1);
          expect(Math.abs(geometry.replacementCenter)).toBeLessThanOrEqual(1);
          expect(Math.abs(geometry.trailing)).toBeLessThanOrEqual(1);
          for (const occurrence of await adjustment
            .getByRole('spinbutton', { name: 'Occurrence' })
            .all()) {
            expect(await occurrence.getAttribute('placeholder')).toBe('Unique');
            expect(
              await occurrence.evaluate(
                (node) =>
                  document.getElementById(node.getAttribute('aria-describedby')!)?.textContent,
              ),
            ).toBe('Leave blank when the heading appears once; otherwise enter its number');
          }
          if (directory)
            await adjustment.screenshot({
              path: join(directory, `editor-contributing-${colorScheme}-${width}.png`),
            });
        }
      }
    } finally {
      await page.close();
    }
  });
});
