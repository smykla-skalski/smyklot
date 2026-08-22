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

/** A component's source, read once per file rather than once per row. */
const sources = new Map<string, string>();

function sourceOf(file: string): string {
  const known = sources.get(file);
  if (known !== undefined) return known;

  const text = readFileSync(new URL(`../src/lib/components/${file}`, import.meta.url), 'utf8');
  sources.set(file, text);

  return text;
}

/** What paints an inherited ground - a component, or the shared sheet the plates come from. */
function hostSource(file: string): string {
  return file.endsWith('.css')
    ? readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8')
    : sourceOf(file);
}

/**
 * Every `background:` colour one class has AT REST, in declaration order.
 *
 * Per class rather than per file, because two of these surfaces live in one file and share its
 * `<style>` - a whole-file scan reports the tile's ground against the legend row and passes.
 *
 * At rest, because a ground is what is under the state layer, and a class carries states as well:
 * a pressed legend row paints a real background, and counting that would report a transparent row
 * as painting its own ground. So a rule reaching the class through anything further - `:active`,
 * `[aria-pressed]`, `.is-refused` - is a state and is not read here.
 *
 * `background-image` is not read either: that is where both state layers are painted, over
 * whatever ground is already there, and counting them would report every pressable surface as
 * painting its own.
 */
function grounds(source: string, className: string): string[] {
  // `(?![\w-])` rather than `\b`, or `.tile` would claim `.tile-well`: a hyphen ends a word to a
  // regular expression and does not end a class name. `(?![:.[])` is what leaves the states out.
  const atRest = new RegExp(String.raw`\.${className}(?![\w-])(?![:.[])`);

  const painted: string[] = [];
  for (const rule of source.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    if (!atRest.test(rule[1] as string)) continue;
    for (const declaration of (rule[2] as string).matchAll(/(?<!-)\bbackground:\s*([^;]+);/g)) {
      painted.push((declaration[1] as string).trim());
    }
  }

  return painted;
}

/**
 * The surfaces sync draws that a reader can press, and the ground each rests on.
 *
 * `ground` is the colour actually under the state layer, not the page behind the card: a hover
 * layer is composited over whatever is already there, so measuring against anything else measures
 * a colour nobody sees.
 *
 * `paints` is how that ground arrives, and it is checked rather than trusted. Three of these four
 * rows name the same token, so the colour arithmetic below is the same sum three times - which
 * means the arithmetic alone cannot tell a component painting the right layer over the WRONG
 * ground from one doing it properly. What separates them is this: a surface either declares its
 * own ground, and the file has to say so, or it declares none and takes its host's, and then the
 * file has to be transparent and the host has to paint what the row claims.
 */
const SURFACES: readonly {
  what: string;
  file: string;
  /** The class the pressable surface wears, so two surfaces in one file are told apart. */
  className: string;
  ground: string;
  /**
   * `declared` - the class sets `background: var(--<ground>)` on itself.
   * `inherited` - the class paints no ground at all, and `from` is what does.
   */
  paints: 'declared' | 'inherited';
  /** Where an inherited ground comes from: a file, and the declaration in it. */
  from?: { file: string; declares: string };
}[] = [
  {
    what: 'a board tile',
    file: 'SyncOverview.svelte',
    className: 'tile',
    ground: 'tile-face',
    paints: 'declared',
  },
  {
    // Transparent by design: the legend sits inside the board's own plate, and a second ground
    // under it would draw a band across a card that is meant to read as one surface.
    what: 'a legend row',
    file: 'SyncOverview.svelte',
    className: 'legend-row',
    ground: 'surface-base',
    paints: 'inherited',
    from: { file: 'SyncOverview.svelte', declares: 'background: var(--surface-base)' },
  },
  {
    what: 'a kind card',
    file: 'SyncOverview.svelte',
    className: 'kind-card',
    ground: 'surface-base',
    paints: 'declared',
  },
  {
    // A row inside the files card's own plate, which is what paints the ground under it.
    what: 'a named object row',
    file: 'SyncFilesPage.svelte',
    className: 'object-row',
    ground: 'surface-base',
    paints: 'inherited',
    from: { file: 'SyncFilesPage.svelte', declares: 'background: var(--surface-base)' },
  },
];

describe('sync surfaces [Unit]', () => {
  /**
   * The ground each row names is the ground its component actually has.
   *
   * Without this the colour arithmetic below proves only that `--surface-base` plus the hover layer
   * lands in the band - which stays true after a component is changed to paint `--surface-raised`
   * and quietly stops matching the row that measures it. Read from the source, per row, so a fourth
   * surface cannot be added by copying a line.
   */
  describe('rests on the ground its row names', () => {
    for (const surface of SURFACES) {
      it(`${surface.what} paints its ground as ${surface.paints}`, () => {
        const painted = grounds(sourceOf(surface.file), surface.className);

        if (surface.paints === 'declared') {
          expect(painted).toContain(`var(--${surface.ground})`);

          return;
        }

        // Transparent: `background: none` is a declaration, and the only other one permitted.
        expect(painted.filter((value) => value !== 'none')).toEqual([]);

        const host = surface.from as { file: string; declares: string };
        expect(hostSource(host.file)).toContain(host.declares);
      });
    }
  });

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
      if (!source.includes(':hover')) missing.push(`${file}: no hover`);
      if (!source.includes(':active')) missing.push(`${file}: no press`);
      /* The panel's press voice: the inset shadow and the 1px seat, the same
         pair every pressed control in the product wears - not a scale. */
      if (!source.includes('--pressed-inset')) missing.push(`${file}: no press depth`);
    }
    expect(missing).toEqual([]);
  });
});
