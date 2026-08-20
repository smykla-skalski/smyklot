import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { contrast, deltaE, oklch, over } from './color';
import { palettes, type Palette } from './theme';

/**
 * The surfaces a table and its controls are built from, in all four palettes.
 *
 * A page is mostly these: a header strip, rows that answer the pointer, a filler area where the
 * rows run out, and the fields above them. They were set one at a time, so a hovered row moved
 * half as far as a hovered anything-else and landed on the exact colour the filler paints - the
 * pointer said "this row" in the same voice the table used for "nothing here".
 */

/** One state change on a ground, as the sidebar has drawn it since before any of this. */
const step = (ground: string): number => (oklch(ground).L > 0.5 ? 2.51 : 5.07);

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
      const hover = grounded('table-row-hover');
      const moved = deltaE(surface, hover);
      expect(moved).toBeGreaterThan(step(surface) - 0.5);
      expect(moved).toBeLessThan(step(surface) + 0.5);
    });

    it('keeps a hovered row distinct from the filler behind it', () => {
      // Both were --surface-raised, so the pointer and "no rows here" painted the same colour.
      expect(deltaE(grounded('table-row-hover'), palette.color('table-filler-bg'))).toBeGreaterThan(
        1,
      );
    });

    it('lifts on a dark ground and drops on a light one', () => {
      const hover = grounded('table-row-hover');
      const lighter = oklch(hover).L > oklch(surface).L;
      expect(lighter).toBe(oklch(surface).L <= 0.5);
    });

    it.each(['table-header-bg', 'table-footer-bg', 'input-bg', 'control-bg', 'table-filler-bg'])(
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
      expect(one).toBeGreaterThan(step(ground) - 0.5);
      expect(one).toBeLessThan(step(ground) + 0.5);
      expect(two).toBeGreaterThan(one * 1.5);
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
      ['text-secondary', 'table-header-bg'],
      ['text-muted', 'table-header-bg'],
      ['text-primary', 'table-row-hover'],
      ['text-primary', 'input-bg'],
      ['text-muted', 'input-bg'],
    ])('keeps %s readable on %s', (ink, ground) => {
      expect(contrast(palette.color(ink), grounded(ground))).toBeGreaterThanOrEqual(4.5);
    });
  },
);

describe('the palette', () => {
  const css = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8');
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

    const base = variables('fieldset');
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
