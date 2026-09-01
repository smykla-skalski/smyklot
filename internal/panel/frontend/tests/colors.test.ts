import { describe, expect, it } from 'vitest';

import { contrast, deltaE, mix, relativeLuminance as relativeLuminanceOf } from './color';
import { palettes } from './theme';

/**
 * The palette's contrast floors, in every combination a page renders in.
 *
 * This used to build its own two palettes with `/:root\s*\{/`, which matches `:root {` and nothing
 * else - `:root[data-theme='dark']` has an attribute selector in the way. So the second palette was
 * a copy of the first, and every "dark" case here was re-checking light. Palette resolution lives
 * in `./theme` now, which walks the cascade properly and knows about the Root console as well.
 */

describe.each(palettes.map((palette) => [palette.name, palette] as const))(
  '%s palette',
  (_name, palette) => {
    const color = (token: string): string => palette.color(token);

    it.each([
      ['text-primary', 'surface-base'],
      ['text-primary', 'surface-control'],
      ['text-primary', 'input-bg'],
      ['text-muted', 'surface-base'],
      ['text-muted', 'surface-control'],
      ['text-muted', 'input-bg'],
      ['text-muted', 'surface-raised'],
      ['focus', 'surface-base'],
      ['info', 'surface-base'],
      ['info', 'info-tint'],
      ['success', 'success-tint'],
      ['warning', 'warning-tint'],
      ['danger', 'danger-tint'],
      ['brand-action-text', 'surface-base'],
      ['brand-action-text', 'brand-action-tint'],
      ['on-brand-action', 'brand-action'],
      // A tone is a text colour on one side of the theme and a fill on the other, so the ink over
      // it cannot be named at the call site. .btn-stop named white and carried it into both dark
      // palettes, where --danger is a pale pink: 2.04:1, under AA and under AA-large, on every
      // destructive confirmation in the product. Only danger and the brand have a partner: no
      // control fills with info, success or warning, and the three inks that answered for them
      // are gone - the tones themselves appear as text and as dots, both measured elsewhere.
      ['on-danger', 'danger'],
      ['sidebar-text', 'sidebar-bg'],
      ['sidebar-text-muted', 'sidebar-bg'],
    ])('keeps %s readable on %s', (foreground, background) => {
      expect(contrast(color(foreground), color(background))).toBeGreaterThanOrEqual(4.5);
    });

    it('keeps structural rules deliberately subtle', () => {
      const ratio = contrast(color('border-subtle'), color('surface-base'));
      expect(ratio).toBeGreaterThanOrEqual(1.2);
      expect(ratio).toBeLessThan(2);
    });

    it('keeps resting control boundaries quiet but visible', () => {
      const controlRatio = contrast(color('control-border'), color('control-bg'));
      const inputRatio = contrast(color('control-border'), color('input-bg'));
      expect(controlRatio).toBeGreaterThanOrEqual(1.5);
      expect(controlRatio).toBeLessThan(2.25);
      expect(inputRatio).toBeGreaterThanOrEqual(1.4);
      expect(inputRatio).toBeLessThan(2.25);
    });

    it.each(['canvas', 'surface-base', 'surface-control', 'input-bg'])(
      'keeps the focus indicator visible on %s',
      (background) => {
        expect(contrast(color('focus'), color(background))).toBeGreaterThanOrEqual(3);
      },
    );

    it('keeps active navigation legible without an extra rail', () => {
      /* The selection is a solid pair - a fill and the ink that goes on it - rather
         than a near-white thumb carrying whatever the palette's active ink happened to
         be. That pairing is what the old shape could not state: a pale fill under an
         inverse ink is white on white, which is what the rail's console shield had
         become. Both halves come from the palette, so this asks the pair. */
      expect(
        contrast(color('sidebar-item-active-text'), color('sidebar-active-bg')),
      ).toBeGreaterThanOrEqual(4.5);
    });

    it('makes the sidebar selection the loudest thing in its column', () => {
      /* The fill leads the ladder. A press on a neighbouring row is the loudest state
         the column has apart from the selection, so the selection has to clear it -
         otherwise pressing an unselected row shouts over the row that is chosen. */
      const rail = color('sidebar-bg');
      expect(deltaE(color('sidebar-active-bg'), rail)).toBeGreaterThan(
        deltaE(color('sidebar-item-pressed'), rail),
      );
    });

    it('keeps drift counts readable on their tinted board tile', () => {
      const drift = color('diff-chg-ink');
      const tintedTile = mix(drift, color('tile-face'), 0.14);

      expect(contrast(drift, tintedTile)).toBeGreaterThanOrEqual(4.5);
    });

    it('keeps control fills from becoming focal surfaces', () => {
      expect(contrast(color('control-bg'), color('surface-base'))).toBeLessThan(1.5);
    });

    it('gives editable command fields a distinct inset surface', () => {
      const ratio = contrast(color('input-bg'), color('surface-base'));
      expect(ratio).toBeGreaterThanOrEqual(1.05);
      expect(ratio).toBeLessThan(1.3);
    });

    /* Hover lightens on a dark ground and darkens on a light one - the rule --brand-action-hover
       and --interactive-hover-layer are both written to. A filled tone that darkened on both sent
       hover and press in opposite directions on the dark palettes, since the press overlay follows
       the theme and the hover did not. */
    it('moves a hovered destructive fill the way the theme moves', () => {
      const dark = relativeLuminanceOf(color('canvas')) < 0.5;
      const moved =
        relativeLuminanceOf(color('danger-hover')) - relativeLuminanceOf(color('danger'));
      expect(Math.sign(moved)).toBe(dark ? 1 : -1);
      expect(contrast(color('on-danger'), color('danger-hover'))).toBeGreaterThanOrEqual(4.5);
    });

    it('keeps segmented-control labels readable and selected fills restrained', () => {
      expect(contrast(color('text-secondary'), color('surface-inset'))).toBeGreaterThanOrEqual(4.5);
      for (const fill of ['surface-base', 'brand-action-tint', 'success-tint', 'danger-tint']) {
        const ratio = contrast(color(fill), color('surface-inset'));
        expect(ratio).toBeGreaterThan(1);
        expect(ratio).toBeLessThan(1.5);
      }
    });
  },
);
