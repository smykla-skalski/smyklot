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
const halo = read('../static/smyklot-halo.svg');
const mark = read('../src/lib/components/BrandMark.svelte');

function number(source: string, pattern: RegExp): number {
  const found = pattern.exec(source);
  expect(found, `nothing matched ${pattern.source}`).not.toBeNull();
  return Number(found?.[1]);
}

const box = number(halo, /viewBox="[-\d.]+ [-\d.]+ ([\d.]+) /u);
const radius = number(halo, /id="solid-halo-arc"[^>]*\sr="([\d.]+)"/u);
const stroke = number(halo, /\.halo-ring-stroke\s*\{[^}]*stroke-width:\s*([\d.]+)px/u);

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
