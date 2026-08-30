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

      /* A fill squares the side facing another segment and keeps the track's curve where
         it faces the track's end: a hover has to cover its whole segment, and rounded
         against a neighbour it would leave two crescents of bare track at the join a
         reader is looking straight at. Corners read in logical order: start-start,
         start-end, end-end, end-start. */
      for (const [index, fill] of geometry.fills.entries()) {
        expect(fill.corners, `segment ${index} corners`).toEqual([
          fill.first ? radius : '0px',
          fill.last ? radius : '0px',
          fill.last ? radius : '0px',
          fill.first ? radius : '0px',
        ]);
        /* And against the thumb it runs on underneath by that same radius, because the
           thumb is drawn over it and rounded, so its curve leaves a wedge belonging to
           neither box. Only there - a bleed under an ordinary neighbour would be a hover
           reaching into a segment nobody is pointing at. */
        expect(
          fill.insets,
          `segment ${index} bleed (thumb ${fill.thumbSide ?? 'not adjacent'})`,
        ).toEqual([
          fill.thumbSide === 'start' ? `-${radius}` : '0px',
          fill.thumbSide === 'end' ? `-${radius}` : '0px',
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
});
