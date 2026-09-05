import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Page } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

describe('configured file formatting in the development panel', () => {
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
          .getByRole('radio', { name: 'Content adjustment', exact: true })
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
        .getByRole('radio', { name: 'Content adjustment', exact: true })
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
              .getByRole('radio', { name: 'Content adjustment', exact: true })
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
              start: formatting.top - tabs.bottom,
              contentHeight: formatting.height,
            };
          });
          expect(geometry.left).toBeGreaterThanOrEqual(0);
          expect(geometry.right).toBeLessThanOrEqual(geometry.width + 1);
          expect(geometry.overflow).toBeLessThanOrEqual(1);
          expect(geometry.start).toBeGreaterThanOrEqual(15);
          expect(geometry.contentHeight).toBeGreaterThan(400);
          await page.screenshot({
            path: `../../../.bart/sync-redesign/after/inspector-${width}-${colorScheme}.png`,
          });
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
