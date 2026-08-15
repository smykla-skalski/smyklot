import { describe, expect, it } from 'vitest';

import { rollGalaxies } from '../src/lib/galaxies';

/**
 * The deep-field roll: rare, bounded, and never in the reader's way. The
 * counts hang on the first draw, so a queued random pins them exactly; the
 * bounds are swept with a seeded generator.
 */

/** mulberry32, as everywhere in these tests. */
function rng(seed: number): () => number {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** Hands out the queued values, then a flat 0.5 forever. */
function queued(...values: number[]): () => number {
  let i = 0;
  return () => values[(i += 1) - 1] ?? 0.5;
}

describe('rollGalaxies [Unit]', () => {
  it('deals none most of the time, one sometimes, a pair rarely', () => {
    expect(rollGalaxies(queued(0.5))).toHaveLength(0);
    expect(rollGalaxies(queued(0.35))).toHaveLength(0);
    expect(rollGalaxies(queued(0.2))).toHaveLength(1);
    expect(rollGalaxies(queued(0.05))).toHaveLength(2);
  });

  it('keeps every galaxy off the middle, in the solid night, and quiet', () => {
    for (let seed = 1; seed <= 300; seed += 1) {
      for (const galaxy of rollGalaxies(rng(seed))) {
        const offMiddle = (galaxy.x >= 8 && galaxy.x <= 36) || (galaxy.x >= 64 && galaxy.x <= 92);
        expect(offMiddle).toBe(true);
        expect(galaxy.y).toBeGreaterThanOrEqual(38);
        expect(galaxy.y).toBeLessThanOrEqual(58);
        expect(galaxy.size).toBeGreaterThanOrEqual(70);
        expect(galaxy.size).toBeLessThanOrEqual(150);
        expect(Math.abs(galaxy.tilt)).toBeLessThanOrEqual(55);
        expect(galaxy.glow).toBeGreaterThanOrEqual(0.45);
        expect(galaxy.glow).toBeLessThanOrEqual(0.85);
      }
    }
  });

  it('sends a pair to opposite shoulders', () => {
    let pairs = 0;
    for (let seed = 1; seed <= 500; seed += 1) {
      const galaxies = rollGalaxies(rng(seed));
      if (galaxies.length !== 2) continue;
      pairs += 1;
      const [a, b] = galaxies;
      if (a === undefined || b === undefined) continue;
      expect(a.x < 50).not.toBe(b.x < 50);
    }
    // The rarity is the point, but the rule must actually have been checked.
    expect(pairs).toBeGreaterThan(5);
  });
});
