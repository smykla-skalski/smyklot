import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { addressOf, startPanel, visit, type Panel } from './harness';

let panel: Panel;
beforeAll(async () => {
  panel = await startPanel();
});
afterAll(async () => {
  await panel?.close();
});

describe('desktop shared-file lifecycle', () => {
  it.each(['light', 'dark'] as const)(
    'keeps a new %s template unsaved until its save succeeds',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      page.setDefaultTimeout(5000);
      const path = `audit-draft-${colorScheme}.json`;
      const saves: unknown[] = [];
      let refuse = true;
      await page.route('**/api/v1/targets/*/settings', async (route) => {
        if (route.request().method() !== 'PUT') return route.continue();
        saves.push(route.request().postDataJSON());
        if (refuse) {
          refuse = false;
          return route.fulfill({
            status: 503,
            json: { error: { code: 'unavailable', message: 'Save is temporarily unavailable' } },
          });
        }
        return route.continue();
      });
      try {
        await visit(page, addressOf(panel, 'workspace/sync/files'));
        await page.getByRole('button', { name: 'Add a file', exact: true }).click();
        await page.getByPlaceholder('renovate.json, or a path no repository has yet').fill(path);
        await page
          .getByRole('option')
          .filter({ hasText: `Start ${path}` })
          .click();
        await page.getByRole('heading', { name: path, exact: true }).waitFor();
        await expect
          .poll(() => page.locator('.page-head').innerText())
          .toContain('Unsaved new file');
        expect(await page.locator('.page-head').innerText()).not.toContain('updated');
        expect(
          await page
            .getByText(`Draft template created for ${path}. Add content, then save.`, {
              exact: true,
            })
            .count(),
        ).toBe(1);
        expect(saves).toHaveLength(0);
        await page.locator('.cm-content').first().fill('{"enabled": true}');
        await expect
          .poll(() => page.getByRole('button', { name: 'Save', exact: true }).isEnabled())
          .toBe(true);
        expect(saves).toHaveLength(0);
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Settings were not saved', { exact: true }).waitFor();
        expect(await page.locator('.page-head').innerText()).toContain('Unsaved new file');
        expect(await page.locator('.cm-content').first().innerText()).toContain('"enabled": true');
        await page.getByRole('button', { name: 'Save', exact: true }).click();
        await page.getByText('Settings saved', { exact: true }).waitFor();
        await expect
          .poll(() => page.locator('.page-head').innerText())
          .not.toContain('Unsaved new file');
        expect(await page.locator('.page-head').innerText()).toContain(
          'shared-file settings updated',
        );
        expect(saves).toHaveLength(2);
      } finally {
        await page.close();
      }
    },
  );
});

describe('desktop editor gutter contrast', () => {
  it.each(['light', 'dark'] as const)(
    'keeps shared-file line numbers readable in %s',
    async (colorScheme) => {
      const page = await panel.browser.newPage({
        viewport: { width: 1920, height: 1200 },
        colorScheme,
      });
      const directory = process.env.SMYKLOT_VISUAL_AUDIT_DIR;
      const measurements: { state: string; ratios: number[] }[] = [];
      try {
        await visit(page, addressOf(panel, 'workspace/sync/files/renovate.json'));
        const content = page.locator('.cm-content').first();
        await content.waitFor();
        for (const state of [
          'normal',
          'focused',
          'selected',
          'output',
          'output-selected',
          'output-invalid',
          'repository-adjustment',
        ]) {
          if (state === 'focused') await content.click();
          if (state === 'selected') await content.press('ControlOrMeta+a');
          const output = page.getByRole('dialog', { name: 'smyklot', exact: true });
          if (state === 'output') {
            await page
              .getByRole('button', { name: 'Open output for smyklot', exact: true })
              .click();
            await output.locator('.cm-content').waitFor();
            await expect.poll(() => output.locator('.cm-overridden-no').count()).toBeGreaterThan(0);
          }
          if (state === 'output-selected') {
            await output.locator('.cm-content').click();
            await output.locator('.cm-content').press('ControlOrMeta+a');
          }
          if (state === 'output-invalid') {
            await output.locator('.cm-content').fill('{');
            await output.locator('.editor-problem').waitFor();
          }
          if (state === 'repository-adjustment') {
            await output.getByRole('button', { name: 'Undo', exact: true }).click();
            await output.getByRole('link', { name: 'Edit adjustments', exact: true }).click();
            const adjustment = page.getByRole('article', {
              name: 'Adjustment for renovate.json',
              exact: true,
            });
            await adjustment.waitFor();
            await adjustment.locator('.cm-content').waitFor();
            await adjustment.scrollIntoViewIfNeeded();
          }
          if (state.startsWith('output')) {
            await output.evaluate(async (node) => {
              await Promise.all(
                node.getAnimations({ subtree: true }).map((animation) => animation.finished),
              );
            });
          }
          const surface = state.startsWith('output') ? output : page;
          const ratios = await surface
            .locator(
              state.includes('selected')
                ? '.cm-lineNumbers .cm-gutterElement, .cm-line, .cm-line > span'
                : '.cm-lineNumbers .cm-gutterElement, .cm-line, .cm-line > span, .editor-problem .form-error',
            )
            .evaluateAll((nodes, selected) => {
              const canvas = document.createElement('canvas');
              canvas.width = canvas.height = 1;
              const pen = canvas.getContext('2d', { willReadFrequently: true })!;
              const luminance = () => {
                const values = [...pen.getImageData(0, 0, 1, 1).data].slice(0, 3).map((channel) => {
                  const value = channel / 255;
                  return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
                });
                return values[0]! * 0.2126 + values[1]! * 0.7152 + values[2]! * 0.0722;
              };
              return nodes
                .filter((node) => node.checkVisibility() && node.textContent?.trim())
                .map((node) => {
                  const ancestors: Element[] = [];
                  let current: Element | null = node;
                  while (current) {
                    ancestors.push(current);
                    const root = current.getRootNode();
                    current =
                      current.parentElement ?? (root instanceof ShadowRoot ? root.host : null);
                  }
                  pen.globalAlpha = 1;
                  pen.fillStyle = 'white';
                  pen.fillRect(0, 0, 1, 1);
                  for (const ancestor of [...ancestors].reverse()) {
                    pen.fillStyle = getComputedStyle(ancestor).backgroundColor;
                    pen.fillRect(0, 0, 1, 1);
                  }
                  const selection =
                    selected && node.closest('.cm-content')
                      ? getComputedStyle(node, '::selection')
                      : null;
                  if (selection) {
                    pen.clearRect(0, 0, 1, 1);
                    pen.fillStyle = selection.backgroundColor;
                    pen.fillRect(0, 0, 1, 1);
                    if (pen.getImageData(0, 0, 1, 1).data[3] !== 255) {
                      throw new Error('Selected code needs an explicit opaque background');
                    }
                  }
                  const background = luminance();
                  pen.globalAlpha = ancestors.reduce(
                    (alpha, ancestor) => alpha * Number(getComputedStyle(ancestor).opacity),
                    1,
                  );
                  pen.fillStyle = selection?.color ?? getComputedStyle(node).color;
                  pen.fillRect(0, 0, 1, 1);
                  const foreground = luminance();
                  return (
                    (Math.max(foreground, background) + 0.05) /
                    (Math.min(foreground, background) + 0.05)
                  );
                });
            }, state.includes('selected'));
          expect(ratios.length).toBeGreaterThan(0);
          measurements.push({ state, ratios });
          if (directory) {
            await mkdir(directory, { recursive: true });
            await page.evaluate(() => document.fonts.ready);
            await page.mouse.move(0, 0);
            await page.screenshot({
              path: join(directory, `F06-shared-${state}-${colorScheme}.png`),
            });
          }
        }
        if (directory)
          await writeFile(
            join(directory, `F06-shared-${colorScheme}.json`),
            JSON.stringify(measurements, null, 2),
          );
        for (const measurement of measurements) {
          for (const ratio of measurement.ratios)
            expect(ratio, measurement.state).toBeGreaterThanOrEqual(4.5);
        }
      } finally {
        await page.close();
      }
    },
  );
});
