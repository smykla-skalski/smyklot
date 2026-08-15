import { describe, expect, it } from 'vitest';

import { rollPulsars } from '../src/lib/pulsars';

/**
 * The lone beating stars: always a handful, always in the visible band,
 * every rhythm and depth inside its advertised range.
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

describe('rollPulsars [Unit]', () => {
  it('always deals a handful, never a crowd', () => {
    for (let seed = 1; seed <= 100; seed += 1) {
      const count = rollPulsars(rng(seed)).length;
      expect(count).toBeGreaterThanOrEqual(5);
      expect(count).toBeLessThanOrEqual(8);
    }
  });

  it('keeps every pulsar visible, on its own rhythm, above going dark', () => {
    const hues = new Set<string>();
    for (let seed = 1; seed <= 200; seed += 1) {
      for (const pulsar of rollPulsars(rng(seed))) {
        expect(pulsar.x).toBeGreaterThanOrEqual(3);
        expect(pulsar.x).toBeLessThanOrEqual(97);
        expect(pulsar.y).toBeGreaterThanOrEqual(36);
        expect(pulsar.y).toBeLessThanOrEqual(62);
        expect(pulsar.duration).toBeGreaterThanOrEqual(1.6);
        expect(pulsar.duration).toBeLessThanOrEqual(4.5);
        expect(pulsar.phase).toBeGreaterThanOrEqual(0);
        expect(pulsar.floor).toBeGreaterThanOrEqual(0.15);
        expect(pulsar.floor).toBeLessThanOrEqual(0.55);
        hues.add(pulsar.hue);
      }
    }
    // All four populations turn up across enough deals.
    expect(hues).toEqual(new Set(['white', 'ice', 'amber', 'rose']));
  });
});
