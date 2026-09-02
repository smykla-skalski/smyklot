import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { contrast, deltaE, oklch, over, stateBand } from './color';
import { palettes, stylesheets, type Palette } from './theme';

/**
 * The surfaces a table and its controls are built from, in all four palettes.
 *
 * A page is mostly these: a header strip, rows that answer the pointer, a filler area where the
 * rows run out, and the fields above them. They were set one at a time, so a hovered row moved
 * half as far as a hovered anything-else and landed on the exact colour the filler paints - the
 * pointer said "this row" in the same voice the table used for "nothing here".
 */

/** One state change, per ground. The shared band, not a third copy of the sidebar's pair. */
const step = (ground: string): [number, number] => stateBand(ground).hover;

/**
 * A TABLE ROW'S step, which is deliberately quieter than a control's.
 *
 * The shared band is the interactive one - what a button, a menu row, a sync tile and a segmented
 * option all move by. A table row is not one of those: it is the full width of the pane, and the
 * veil that reads as a state on a 90px control reads as a stripe across the page on a row. So the
 * row keeps roughly half the step, which is the ratio it has always had, and this says so rather
 * than leaving one number covering two intentions.
 */
const rowStep = (ground: string): [number, number] => (oklch(ground).L > 0.5 ? [2, 3] : [4.5, 5.5]);

/** The chroma a tint may carry before it stops reading as the same material as the surfaces. */
function chromaCeiling(palette: Palette): number {
  const neutrals = ['canvas', 'surface-base', 'surface-raised', 'surface-inset'].map(
    (name) => oklch(palette.color(name)).C,
  );
  return (neutrals.reduce((total, value) => total + value, 0) / neutrals.length) * 3;
}

describe.each(palettes.map((palette) => [palette.name, palette] as const))(
  '%s surfaces',
  (_name, palette) => {
    const surface = palette.color('surface-base');

    /* The row veils are ink mixed over TRANSPARENT - a flat surface mix
       vanished on canvas-toned strips - so what a reader sees is the veil
       composited on whatever ground the row has. The invariants below are
       stated on the veil's landing on the base surface, which is where the
       old flat tokens lived. */
    const grounded = (token: string): string => {
      const veil = palette.paint(token);
      return over(veil.color, surface, veil.alpha);
    };

    it('moves a hovered row by one state change', () => {
      const hover = grounded('row-hover');
      const moved = deltaE(surface, hover);
      const [floor, ceiling] = rowStep(surface);
      expect(moved).toBeGreaterThanOrEqual(floor);
      expect(moved).toBeLessThanOrEqual(ceiling);
    });

    it('lifts on a dark ground and drops on a light one', () => {
      const hover = grounded('row-hover');
      const lighter = oklch(hover).L > oklch(surface).L;
      expect(lighter).toBe(oklch(surface).L <= 0.5);
    });

    it.each(['pagination-bg', 'input-bg', 'control-bg'])(
      'keeps %s the same material as the surface it sits on',
      (token) => {
        // Visible as a change of surface, quiet enough not to read as a coloured slab, and inside
        // the chroma the neutral surfaces carry so it stays the same material as them.
        const value = palette.color(token);
        expect(deltaE(surface, value)).toBeLessThan(6);
        expect(oklch(value).C).toBeLessThanOrEqual(chromaCeiling(palette));
      },
    );

    it('answers the pointer on a control by one state change, and a press by two', () => {
      // The pickers above the tables hovered to --surface-raised, 0.55 dE00 from their own ground:
      // half a just-noticeable difference, so they did not visibly answer at all. The press used a
      // menu-row fill that landed within half a JND of this hover on both dark palettes.
      const ground = palette.color('control-bg');
      const one = deltaE(ground, palette.color('control-bg-hover'));
      const two = deltaE(ground, palette.color('control-bg-pressed'));
      const [floor, ceiling] = step(ground);
      expect(one).toBeGreaterThanOrEqual(floor);
      expect(one).toBeLessThanOrEqual(ceiling);
      const press = stateBand(ground).press;
      expect(two).toBeGreaterThanOrEqual(press[0]);
      expect(two).toBeLessThanOrEqual(press[1]);
      // Both move the same way, so hover and press are one gesture at two depths.
      const direction = (state: string): number => Math.sign(oklch(state).L - oklch(ground).L);
      expect(direction(palette.color('control-bg-pressed'))).toBe(
        direction(palette.color('control-bg-hover')),
      );
    });

    it('keeps a control label legible once its ground moves', () => {
      // These controls carry muted ink at rest, which falls under AA on the pressed fill - so the
      // component darkens it to secondary, and that is the pair that has to hold.
      for (const ground of ['control-bg-hover', 'control-bg-pressed']) {
        expect(
          contrast(palette.color('text-secondary'), palette.color(ground)),
        ).toBeGreaterThanOrEqual(4.5);
      }
      expect(
        contrast(palette.color('text-muted'), palette.color('control-bg')),
      ).toBeGreaterThanOrEqual(4.5);
    });

    it.each([
      ['text-primary', 'row-hover'],
      ['text-primary', 'input-bg'],
      ['text-muted', 'input-bg'],
    ])('keeps %s readable on %s', (ink, ground) => {
      expect(contrast(palette.color(ink), grounded(ground))).toBeGreaterThanOrEqual(4.5);
    });
  },
);

describe('the palette', () => {
  const css = stylesheets;
  const sources = readdirSync(new URL('../src/lib/components', import.meta.url))
    .filter((file) => file.endsWith('.svelte'))
    .map((file) => readFileSync(new URL(`../src/lib/components/${file}`, import.meta.url), 'utf8'))
    .concat(css, readFileSync(new URL('../src/routes/+layout.svelte', import.meta.url), 'utf8'));

  it('declares nothing it does not paint', () => {
    // Four tokens were declared in every palette and referenced by nothing: --table-sorted-bg,
    // --sidebar-item-active and the two --interactive-selected-*. A colour nobody reads is a
    // colour nobody maintains, and it still has to be answered for every time the palette grows a
    // theme. This is the check that would have caught them.
    const declared = new Set(
      [...css.matchAll(/^\s*--(?<name>[\w-]+):/gmu)].map((match) => match.groups?.name ?? ''),
    );
    const used = new Set(
      sources.flatMap((source) =>
        [...source.matchAll(/var\(--(?<name>[\w-]+)/gu)].map((match) => match.groups?.name ?? ''),
      ),
    );
    // Set by script rather than by a `var()` reference, so they are read where a grep cannot see.
    const runtime = new Set(['segment-left', 'segment-width', 'nav-thumb-top', 'nav-thumb-height']);
    // Only colours. A spacing or type step that nothing reaches for yet is a scale with a gap in
    // it, which is a different argument from a colour nobody paints.
    const colour = (name: string): boolean =>
      /^(primitive|surface|text|border|brand|sidebar|table|control|input|popover|dialog|segment|interactive|role-chip|switcher|unread|identity|code)/u.test(
        name,
      ) ||
      ['canvas', 'focus', 'info', 'success', 'warning', 'danger', 'scrim', 'pending'].some(
        (stem) => name === stem || name.startsWith(`${stem}-`),
      );
    const orphans = [...declared]
      .filter((name) => !used.has(name) && !runtime.has(name) && colour(name))
      .sort();
    expect(orphans).toEqual([]);
  });

  it('re-skins a segmented control completely, or not at all', () => {
    /* A surface block on this control is a whole palette, not a patch. `fieldset` declares the nine
       variables the rest of the stylesheet paints with, and `fieldset.on-sidebar` and
       `fieldset.on-night` each re-point all nine at the family their ground belongs to. Leave one
       out and it does not fall back to something neutral - it keeps the *page's* answer, which is
       a different ground entirely.

       `--seg-text` was left out of the sidebar block. It is only read for the hover ink on an
       unchecked option, so three palettes looked fine and the fourth put the light theme's
       near-black label on the Root menu's near-black track at 1.09:1. Nothing else in the block
       was wrong, and the one that was is the one nobody reads until they hover. */
    const control = readFileSync(
      new URL('../src/lib/components/SegmentedControl.svelte', import.meta.url),
      'utf8',
    );
    const body = (selector: string): string => {
      const start = control.indexOf(`\n  ${selector} {`);
      if (start === -1) throw new Error(`SegmentedControl has no \`${selector}\` rule`);
      const open = control.indexOf('{', start);
      const end = control.indexOf('\n  }', open);
      if (end === -1) throw new Error(`\`${selector}\` is unterminated`);
      return control.slice(open, end);
    };
    const variables = (rule: string): Set<string> =>
      new Set(
        [...body(rule).matchAll(/^\s*--(?<name>[\w-]+):/gmu)].map(
          (match) => match.groups?.name ?? '',
        ),
      );

    /**
     * What a surface owes, and only that.
     *
     * One question, asked of the value rather than the name: does it reach the PAGE's
     * own ground or ink? Those are the things a different ground replaces, so a token
     * built on them is wrong the moment the control is put somewhere else, and the
     * skin owes it.
     *
     * Everything else is owed nothing, and for two different reasons. Geometry - a
     * radius, an inset - is the control's own and identical on every ground; asking
     * for it per skin is three more places to forget it. And a value the PALETTE
     * already re-resolves is not the page's: the Root console swaps `--brand-action`
     * and the whole `--segment-*` family for itself, so a token built only out of
     * those arrives correct anywhere without the skin saying a word.
     *
     * Told apart by what the value REACHES, resolved transitively through the rule's
     * own variables, rather than by whether it carries a unit. A shadow is ground-
     * dependent and is written in pixels; a `color-mix` is ground-dependent and is
     * written in per cent - so "has a unit" called both of them geometry and quietly
     * stopped asking a skin for either.
     *
     * `--seg-text` was the one that went missing: it is read only for the hover ink on
     * an unchecked option, so three palettes looked fine and the fourth put the light
     * theme's near-black label on the Root menu's near-black track at 1.09:1.
     */
    const declarations = (rule: string): Map<string, string> =>
      new Map(
        [...body(rule).matchAll(/^\s*--(?<name>[\w-]+):\s*(?<value>[^;]+);/gmu)].map((match) => [
          match.groups?.name ?? '',
          match.groups?.value ?? '',
        ]),
      );
    /** The page's own ground and ink: what a different surface replaces. */
    const pageGround = /^(?:surface-|text-|shadow-color$|canvas$|border-)/u;
    const owed = (rule: string): Set<string> => {
      const own = declarations(rule);
      const resolve = (value: string, seen = new Set<string>()): string =>
        value.replace(/var\(--(?<name>[\w-]+)\)/gu, (whole, name: string) => {
          if (seen.has(name) || !own.has(name)) return whole;

          return resolve(own.get(name) ?? '', new Set(seen).add(name));
        });

      return new Set(
        [...own]
          .filter(([, value]) => {
            const reaches = [...resolve(value).matchAll(/var\(--(?<name>[\w-]+)\)/gu)].map(
              (match) => match.groups?.name ?? '',
            );

            return reaches.some((name) => pageGround.test(name));
          })
          .map(([name]) => name),
      );
    };

    const base = owed('fieldset');
    const surfaces = [
      ...new Set(
        [...control.matchAll(/^\s{2}(?<rule>fieldset\.on-[\w-]+) \{/gmu)].map(
          (match) => match.groups?.rule ?? '',
        ),
      ),
    ];

    // Preconditions, so a stylesheet this can no longer parse fails rather than passes empty.
    expect(base.size, 'the base fieldset rule declares no variables').toBeGreaterThan(5);
    expect(surfaces.length, 'no fieldset.on-* surfaces were found').toBeGreaterThan(1);

    for (const surface of surfaces) {
      const missing = [...base].filter((name) => !variables(surface).has(name)).sort();
      expect(
        missing,
        `${surface} leaves ${missing.join(', ')} pointing at the page it stands on`,
      ).toEqual([]);
    }
  });
});
