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
      await visit(page, `${panel.origin}/i/${panel.account}/sync/files/renovate.json`, {
        ready: '.format-status',
      });

      const template = page.locator('.card').first();
      await template
        .getByText('This template does not match configured formatting', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });

      await template.getByRole('button', { name: 'View diff' }).click();
      await template.locator('.format-diff').waitFor({ state: 'visible' });

      await template.getByRole('button', { name: 'Format template' }).click();
      await template
        .getByText('Matches configured formatting', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });
      await template.getByRole('button', { name: 'Undo' }).click();
      await template
        .getByText('This template does not match configured formatting', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });

      const repository = page
        .locator('.adjuster')
        .filter({ has: page.getByRole('button', { name: /^smyklot changes/u }) });
      await repository.getByRole('button', { name: /^smyklot changes/u }).click();
      await repository
        .getByText('Backend rendered', { exact: true })
        .waitFor({ state: 'visible', timeout: 30_000 });

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

      const exact = await repository.locator('.exact-output').innerText();
      expect(exact).toContain('"schedule": ["* 4 * * 6"]');
      expect(exact).toContain('"ignorePaths": ["crates/harness-codex-acp/**"]');
      expect(exact).toContain('\r\n');

      const editor = page.locator('.formatting-editor').first();
      const tomlQuoteStyle = editor.getByRole('group', { name: 'Quote Style' }).nth(1);
      await tomlQuoteStyle
        .getByRole('radio', { name: 'Prefer Basic' })
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
            .filter(
              (control) => control.querySelector('legend')?.textContent?.trim() === 'Quote Style',
            )
            .at(1)!
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
          .filter(
            (control) => control.querySelector('legend')?.textContent?.trim() === 'Quote Style',
          )
          .at(1)!;
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

        return {
          arraysWidth: width('Arrays'),
          firstFillCorners: segmentCorners(getComputedStyle(quoteLabels[0]!, '::before')),
          keyOrderWidth: width('Key Order'),
          lastFillCorners: segmentCorners(getComputedStyle(quoteLabels.at(-1)!, '::before')),
          middleFillCorners: segmentCorners(getComputedStyle(selected, '::before')),
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
