/**
 * Ink that answers for its ground.
 *
 * The easter eggs draw in starlight, which is right everywhere the page is
 * night - the whole page in the dark theme, the sky's patch in the light
 * one. The one moment that fails is a retiring flight on a freshly light
 * page: a theme switch turns the ground white under a rocket still flying
 * its way out, and starlight on white is invisible. So a retiring flight
 * blends its ink toward a dark one as it moves below the sky's fade - by
 * position, smoothly, never a snap at some invisible line.
 */

export type Ink = readonly [number, number, number];

/**
 * Linear blend between two inks; `t` clamps to 0..1. With `alphaPct` the
 * result carries that alpha, for gradient stops that need it in-string.
 */
export function mixInk(from: Ink, to: Ink, t: number, alphaPct?: number): string {
  const k = Math.min(1, Math.max(0, t));
  const r = Math.round(from[0] + (to[0] - from[0]) * k);
  const g = Math.round(from[1] + (to[1] - from[1]) * k);
  const b = Math.round(from[2] + (to[2] - from[2]) * k);
  return alphaPct === undefined ? `rgb(${r} ${g} ${b})` : `rgb(${r} ${g} ${b} / ${alphaPct}%)`;
}

/**
 * How far out of the night a point is: 0 well inside the sky's dark, 1 on
 * the light page, ramping across the fade between `startY` and `endY`.
 */
export function fadeBlend(y: number, startY: number, endY: number): number {
  if (endY <= startY) return 0;
  return Math.min(1, Math.max(0, (y - startY) / (endY - startY)));
}
