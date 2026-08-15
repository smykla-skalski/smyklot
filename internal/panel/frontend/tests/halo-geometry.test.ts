import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

/**
 * The collapsed rail draws a disc over the mark, sized in pixels from the halo's drawn geometry so
 * its own ring lands on the halo's rather than inside it. Nothing in the toolchain connects that
 * number to the file it was measured from: widening the SVG's canvas to make room for the ring's
 * glow left the overlay a ring's width too large, and the only thing that caught it was a comment.
 *
 * Checked as source, because the runtime here has no DOM and no cascade.
 */

const read = (path: string): string => readFileSync(new URL(path, import.meta.url), 'utf8');
const halo = read('../src/assets/smyklot-halo.svg');
const mark = read('../src/components/BrandMark.svelte');
const rail = read('../src/components/IdentityBar.svelte');

function number(source: string, pattern: RegExp): number {
  const found = pattern.exec(source);
  expect(found, `nothing matched ${pattern.source}`).not.toBeNull();
  return Number(found?.[1]);
}

describe('the collapsed rail overlay', () => {
  it('is the halo ring at the size the mark renders it', () => {
    const box = number(halo, /viewBox="[-\d.]+ [-\d.]+ ([\d.]+) /u);
    const radius = number(halo, /id="solid-halo-arc"[^>]*\sr="([\d.]+)"/u);
    const stroke = number(halo, /\.halo-ring-stroke\s*\{[^}]*stroke-width:\s*([\d.]+)px/u);
    const size = number(mark, /size = (\d+),/u);

    const round = (value: number): string => value.toFixed(2).replace(/\.?0+$/u, '');
    const disc = round(((radius * 2 + stroke) / box) * size);
    const ring = round((stroke / box) * size);

    expect(rail, `the overlay disc should be ${disc}px`).toContain(`height: ${disc}px`);
    expect(rail, `the overlay disc should be ${disc}px`).toContain(`width: ${disc}px`);
    expect(rail, `the overlay ring should be ${ring}px`).toContain(`border: ${ring}px solid`);
  });
});
