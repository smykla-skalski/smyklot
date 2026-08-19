import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { deltaE, oklch, over } from './color';
import { palettes, type Palette } from './theme';

/**
 * The sync surfaces answer a hover and a press the way the rest of the panel does.
 *
 * These were the one part of the product that did not. A tile answered a hover by rising a single
 * pixel and changing no colour at all; a kind card swapped `--surface-base` for `--surface-raised`,
 * which is 0.55 dE00 apart on three palettes and invisible on all of them; and nothing on the page
 * had a pressed state, so the whole board was a set of controls that never acknowledged being
 * used. Everywhere else in the panel a state is carried by colour, over whatever ground the surface
 * already has, and a press shrinks toward the surface's own centre.
 *
 * What this pins is the step, per ground, because CIEDE2000 does not report the same number for the
 * same perceived move at both ends of the lightness range. The bands are `control-states.test.ts`'s
 * own, so a sync surface and a sidebar row are held to one standard rather than two.
 */

/** The bands the sidebar's approved pair set, read as a shared standard rather than restated. */
function band(ground: string): { hover: [number, number]; press: [number, number] } {
  return oklch(ground).L > 0.5
    ? { hover: [2, 3], press: [4.5, 5.7] }
    : { hover: [4.5, 6], press: [7, 9.5] };
}

/** A translucent layer's colour and the alpha it paints at, from its declaration. */
function layer(palette: Palette, token: string): { color: string; alpha: number } {
  const paint = palette.paint(token);

  return { color: paint.color, alpha: paint.alpha };
}

/**
 * The surfaces sync draws that a reader can press, and the ground each rests on.
 *
 * `ground` is the token the component paints itself with, not the page behind it: a hover layer is
 * composited over whatever is already there, so measuring against anything else measures a colour
 * nobody sees.
 */
const SURFACES: readonly { what: string; file: string; ground: string }[] = [
  { what: 'a board tile', file: 'SyncBoard.svelte', ground: 'tile-face' },
  { what: 'a legend row', file: 'SyncBoard.svelte', ground: 'surface-base' },
  { what: 'a kind card', file: 'SyncKindCard.svelte', ground: 'surface-base' },
  { what: 'a named object row', file: 'ObjectRow.svelte', ground: 'surface-base' },
];

describe('sync surfaces [Unit]', () => {
  for (const palette of palettes) {
    describe(palette.name, () => {
      for (const surface of SURFACES) {
        describe(surface.what, () => {
          const ground = palette.color(surface.ground);
          const bounds = band(ground);
          const hovered = ((): string => {
            const { color, alpha } = layer(palette, 'interactive-hover-layer');

            return over(color, ground, alpha);
          })();
          const pressed = ((): string => {
            const { color, alpha } = layer(palette, 'press');

            return over(color, ground, alpha);
          })();

          it('moves one state change on hover', () => {
            const moved = deltaE(ground, hovered);
            expect(moved).toBeGreaterThanOrEqual(bounds.hover[0]);
            expect(moved).toBeLessThanOrEqual(bounds.hover[1]);
          });

          it('moves further on press than on hover', () => {
            expect(deltaE(ground, pressed)).toBeGreaterThan(deltaE(ground, hovered));
          });

          // A press that reversed direction would read as a second, different control rather than
          // as more of the same one.
          it('presses the way it hovers', () => {
            const towards = (value: string): boolean => oklch(value).L > oklch(ground).L;
            expect(towards(pressed)).toBe(towards(hovered));
          });
        });
      }
    });
  }

  /**
   * Both states are declared, in every file that draws a pressable sync surface.
   *
   * The states are easy to add to one component and forget in the next, and the result looks
   * finished: the page still works, one card just never acknowledges the hand. Read from the files
   * so that a component added later has to answer this too.
   */
  it('declares a hover and a press wherever it draws a pressable surface', () => {
    const missing: string[] = [];
    for (const file of [...new Set(SURFACES.map((surface) => surface.file))]) {
      const source = readFileSync(
        new URL(`../src/lib/components/${file}`, import.meta.url),
        'utf8',
      );
      if (!source.includes('--interactive-hover-layer')) missing.push(`${file}: no hover layer`);
      if (!source.includes(':active')) missing.push(`${file}: no press`);
      if (!source.includes('--press-scale')) missing.push(`${file}: no press scale`);
    }
    expect(missing).toEqual([]);
  });
});
