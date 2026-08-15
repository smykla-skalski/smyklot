import { describe, expect, it } from 'vitest';

import { fadeBlend, mixInk } from '../src/lib/ink';

/**
 * The blend a retiring flight paints with after a dark-to-light switch:
 * starlight in the sky's night, dark on the page, a ramp across the fade.
 */
describe('ink [Unit]', () => {
  it('returns each end of the blend at its end', () => {
    expect(mixInk([216, 232, 255], [40, 51, 74], 0)).toBe('rgb(216 232 255)');
    expect(mixInk([216, 232, 255], [40, 51, 74], 1)).toBe('rgb(40 51 74)');
  });

  it('clamps past the ends and carries alpha when asked', () => {
    expect(mixInk([10, 10, 10], [20, 20, 20], -3)).toBe('rgb(10 10 10)');
    expect(mixInk([10, 10, 10], [20, 20, 20], 7, 45)).toBe('rgb(20 20 20 / 45%)');
  });

  it('ramps position across the fade and holds flat outside it', () => {
    expect(fadeBlend(0, 100, 200)).toBe(0);
    expect(fadeBlend(150, 100, 200)).toBeCloseTo(0.5, 9);
    expect(fadeBlend(300, 100, 200)).toBe(1);
    // A degenerate fade answers night rather than dividing by nothing.
    expect(fadeBlend(50, 200, 100)).toBe(0);
  });
});
