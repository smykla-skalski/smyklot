import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Locator, Page } from 'playwright-core';

import { startPanel, visit, type Panel } from './harness';

let panel: Panel;

beforeAll(async () => {
  panel = await startPanel();
});

afterAll(async () => {
  await panel?.close();
});

describe('configured file formatting in the development panel', () => {
  it('keeps the exact backend output beside each contextual editor', async () => {
    const page: Page = await panel.browser.newPage({ viewport: { width: 1440, height: 1000 } });
    const crashes: string[] = [];
    page.on('pageerror', (error) => crashes.push(error.message));

    try {
      await visit(page, `${panel.origin}/i/${panel.account}/sync/files/renovate.json`, {
        ready: '.format-status',
      });

      const template = page.locator('.card').first();
      await template
        .getByText('This template does not match configured formatting', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });

      await template.getByRole('button', { name: 'View diff' }).click();
      await template.locator('.format-diff').waitFor({ state: 'visible' });
      await expectVisibleText(template, 'Draft template');
      await expectVisibleText(template, 'Final output');

      await template.getByRole('button', { name: 'Format', exact: true }).click();
      await template
        .getByText('Matches configured formatting', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });
      await template.getByRole('button', { name: 'Undo' }).click();
      await template
        .getByText('This template does not match configured formatting', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });

      await template.locator('.cm-content').focus();
      await page.keyboard.press('Alt+Shift+f');
      await template
        .getByText('Matches configured formatting', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });

      const templateFormatting = page.locator('.formatting-editor').first();
      await templateFormatting.getByRole('region', { name: 'Common' }).waitFor();
      await templateFormatting.getByRole('region', { name: 'JSON', exact: true }).waitFor();
      expect(await templateFormatting.getByRole('region', { name: 'YAML' }).count()).toBe(0);
      expect(await templateFormatting.getByRole('region', { name: 'TOML' }).count()).toBe(0);
      expect(
        await templateFormatting.getByText('From Template', { exact: true }).count(),
      ).toBeGreaterThan(0);
      await expectVisibleText(templateFormatting, 'Where formatting comes from');

      const repository = page
        .locator('.adjuster')
        .filter({ has: page.getByRole('button', { name: /^smyklot changes/u }) });
      await repository.getByRole('button', { name: /^smyklot changes/u }).click();
      await repository
        .getByText('Backend rendered', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });

      const previewGeometry = await repository
        .locator('.repository-preview-grid')
        .evaluate((grid) => {
          const [editing, output] = [...grid.children].map((child) =>
            (child as HTMLElement).getBoundingClientRect(),
          );
          return {
            editingWidth: editing?.width ?? 0,
            outputWidth: output?.width ?? 0,
            outputFollowsEditing: (output?.left ?? 0) > (editing?.left ?? 0),
          };
        });
      expect(previewGeometry.editingWidth).toBeGreaterThan(300);
      expect(previewGeometry.outputWidth).toBeGreaterThan(300);
      expect(previewGeometry.outputFollowsEditing).toBe(true);

      const formatting = repository.locator('.repository-formatting');
      expect(
        await formatting
          .getByRole('group', { name: 'Line Ending' })
          .getByRole('radio', { name: 'Crlf' })
          .isChecked(),
      ).toBe(true);
      expect(
        await formatting
          .getByRole('region', { name: 'JSON', exact: true })
          .getByRole('group', { name: 'Arrays' })
          .getByRole('radio', { name: 'Compact' })
          .isChecked(),
      ).toBe(true);
      expect(
        await formatting.getByText('From File override', { exact: true }).count(),
      ).toBeGreaterThan(0);
      expect(await formatting.getByRole('region', { name: 'YAML' }).count()).toBe(0);
      expect(await formatting.getByRole('region', { name: 'TOML' }).count()).toBe(0);

      const exact = await repository.locator('.exact-output').innerText();
      expect(exact).toContain('"schedule": ["* 4 * * 6"]');
      expect(exact).toContain('"ignorePaths": ["crates/harness-codex-acp/**"]');
      expect(exact).toContain('\r\n');

      const geometry = await formatting.evaluate((editor) => {
        const controls = [...editor.querySelectorAll<HTMLElement>('.policy-row fieldset')];
        const control = (name: string): HTMLElement =>
          controls
            .filter((control) => control.querySelector('legend')?.textContent?.trim() === name)
            .at(0)!;
        const arrays = control('Arrays');
        const keyOrder = control('Key Order');
        const arraySegments = [...arrays.querySelectorAll<HTMLElement>('label')].map(
          (label) => label.getBoundingClientRect().width,
        );
        const wrapped = controls.flatMap((control) =>
          [...control.querySelectorAll<HTMLElement>('.band-trim')]
            .filter((label) => {
              const range = document.createRange();
              range.selectNodeContents(label);
              return range.getClientRects().length > 1;
            })
            .map((label) => label.textContent?.trim() ?? ''),
        );
        const thumb = arrays.querySelector<HTMLElement>('.selection-indicator')!;
        const selected = arrays.querySelector<HTMLInputElement>('input:checked')!.closest('label')!;
        const thumbBox = thumb.getBoundingClientRect();
        const selectedBox = selected.getBoundingClientRect();
        const segmentCorners = (style: CSSStyleDeclaration): string[] => [
          style.borderStartStartRadius,
          style.borderStartEndRadius,
          style.borderEndEndRadius,
          style.borderEndStartRadius,
        ];
        const arrayLabels = [...arrays.querySelectorAll('label')];

        return {
          arraysWidth: arrays.getBoundingClientRect().width,
          firstFillCorners: segmentCorners(getComputedStyle(arrayLabels[0]!, '::before')),
          keyOrderWidth: keyOrder.getBoundingClientRect().width,
          lastFillCorners: segmentCorners(getComputedStyle(arrayLabels.at(-1)!, '::before')),
          middleFillCorners: segmentCorners(getComputedStyle(selected, '::before')),
          segmentSpread: Math.max(...arraySegments) - Math.min(...arraySegments),
          thumbCorners: segmentCorners(getComputedStyle(thumb)),
          thumbLeftDelta: Math.abs(thumbBox.left - selectedBox.left),
          thumbWidthDelta: Math.abs(thumbBox.width - selectedBox.width),
          wrapped,
        };
      });
      expect(geometry.arraysWidth).toBeGreaterThan(geometry.keyOrderWidth);
      expect(geometry.segmentSpread).toBeGreaterThan(1);
      expect(geometry.thumbLeftDelta).toBeLessThanOrEqual(0.05);
      expect(geometry.thumbWidthDelta).toBeLessThanOrEqual(0.05);
      expect(geometry.middleFillCorners).toEqual(['0px', '0px', '0px', '0px']);
      expect(geometry.thumbCorners).toEqual(['0px', '0px', '0px', '0px']);
      expect(geometry.firstFillCorners[0]).not.toBe('0px');
      expect(geometry.firstFillCorners.slice(1)).toEqual([
        '0px',
        '0px',
        geometry.firstFillCorners[0],
      ]);
      expect(geometry.lastFillCorners.slice(0, 3)).toEqual([
        '0px',
        geometry.lastFillCorners[2],
        geometry.lastFillCorners[2],
      ]);
      expect(geometry.lastFillCorners[3]).toBe('0px');
      expect(geometry.wrapped).toEqual([]);
      expect(crashes).toEqual([]);
    } finally {
      await page.close();
    }
  });
});

async function expectVisibleText(scope: Locator, text: string): Promise<void> {
  await scope.getByText(text, { exact: true }).waitFor({ state: 'visible' });
}
