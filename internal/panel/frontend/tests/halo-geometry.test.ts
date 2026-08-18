import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * Two things are drawn to fit inside the halo: the ground the robot stands on, and the disc the
 * collapsed rail puts the expand glyph on. Both are sized in the host document from the SVG's own
 * drawn geometry, and nothing in the toolchain connects those numbers to the file they came from -
 * widening the canvas to make room for the ring's glow once left the rail's overlay a ring's width
 * too large, and the only thing that caught it was a comment.
 *
 * Checked as source, because the runtime here has no DOM and no cascade.
 */

const read = (path: string): string => readFileSync(new URL(path, import.meta.url), 'utf8');
const halo = read('../src/assets/smyklot-halo.svg');
const mark = read('../src/lib/components/BrandMark.svelte');
const rail = read('../src/lib/components/IdentityBar.svelte');

function number(source: string, pattern: RegExp): number {
  const found = pattern.exec(source);
  expect(found, `nothing matched ${pattern.source}`).not.toBeNull();
  return Number(found?.[1]);
}

const box = number(halo, /viewBox="[-\d.]+ [-\d.]+ ([\d.]+) /u);
const radius = number(halo, /id="solid-halo-arc"[^>]*\sr="([\d.]+)"/u);
const stroke = number(halo, /\.halo-ring-stroke\s*\{[^}]*stroke-width:\s*([\d.]+)px/u);
const size = number(mark, /size = (\d+),/u);

/** The ring's inner edge, which is where the interior stops. */
const interiorFraction = ((radius - stroke / 2) * 2) / box;

describe('the ground inside the ring', () => {
  it('is the circle the SVG would have filled', () => {
    // The mark is an `<img>`, so the file's own `#halo-interior-background` toggle cannot be
    // reached from here and the disc is reproduced behind it. Same circle, or it shows.
    const drawn = number(mark, /\.mark-well\.grounded::before[^}]*width:\s*([\d.]+)%/u);

    expect(drawn).toBeCloseTo(interiorFraction * 100, 1);
    expect(mark, 'the disc must be square').toContain(`height: ${drawn}%`);
  });

  it('paints the colour the file names for it', () => {
    const fill = /id="halo-interior-solid-color"[^>]*\scolor="(?<hex>#[0-9A-Fa-f]{6})"/u
      .exec(halo)
      ?.groups?.hex?.toLowerCase();

    expect(fill).toBeDefined();
    expect(mark).toContain(`background: ${fill}`);
  });
});

describe('the collapsed rail overlay', () => {
  it('sits inside the halo rather than over it', () => {
    // It used to be the ring's outer figure with a grey ring of its own, so hovering swapped the
    // rainbow halo for a plain circle. The disc has to cover the interior and stop there.
    const interior = interiorFraction * size;
    const ring = (stroke / box) * size;
    const disc = number(
      rail,
      /\.collapsed :global\(\.sidebar-collapse-trigger::before\)[^}]*height:\s*([\d.]+)px/u,
    );

    expect(
      disc,
      `the disc must cover the ${interior.toFixed(2)}px interior`,
    ).toBeGreaterThanOrEqual(interior);
    // Anything past half the ring's width starts eating the halo visibly.
    expect(disc, 'the disc must not eat into the ring').toBeLessThan(interior + ring / 2);
  });

  it('has no ring of its own', () => {
    // The halo's ring is the ring. A second one drawn on top is what this replaced.
    const block =
      /\.collapsed :global\(\.sidebar-collapse-trigger::before\)\s*\{(?<body>[^}]*)\}/u.exec(rail)
        ?.groups?.body ?? '';

    expect(block.length).toBeGreaterThan(0);
    // `border-radius` is what makes it a circle; any other border is a second ring.
    expect(block).not.toMatch(/border(?!-radius)(-\w+)?:/u);
  });

  it('keeps its own target invisible', () => {
    // The target is the whole row and is drawn over the mark, so a background on the button is a
    // background over the halo - which is how hovering wiped the ring off the rail.
    expect(rail).toMatch(
      /\.collapsed :global\(\.sidebar-collapse-trigger:hover\),[\s\S]{0,200}?\{\s*background: transparent;/u,
    );
  });
});
