/**
 * The sky's rare deep-field visitors. Most skies have no galaxy, some have
 * one, a few have a pair - rolled once per mount, so a reload deals a new
 * sky. Injected `random` for the same reason as everything else in this
 * corner: the odds and the bounds are provable in a unit test.
 */

export interface Galaxy {
  /** Centre, in percent of the sky's box. */
  x: number;
  y: number;
  /** Major-axis width in px; the drawn disc's height follows from it. */
  size: number;
  /** Degrees off horizontal. */
  tilt: number;
  /** Resting opacity - some are faint, none is loud. */
  glow: number;
  /** A warmer, older population; most run cool and blue. */
  warm: boolean;
}

export function rollGalaxies(random: () => number): Galaxy[] {
  const roll = random();
  const count = roll < 0.07 ? 2 : roll < 0.35 ? 1 : 0;
  if (count === 0) return [];
  const firstLeft = random() < 0.5;
  const galaxies: Galaxy[] = [];
  for (let i = 0; i < count; i += 1) {
    // Off the middle, where the mark stands on the sky's own core colour,
    // and in the band the reader can actually see: the sky's top third
    // hangs above the window on purpose, so "upper half" in sky terms is
    // off screen - this range sits below the window's top edge and above
    // the fade, where the ground is still solidly night. A pair takes
    // opposite sides rather than crowding one shoulder.
    const left = i === 0 ? firstLeft : !firstLeft;
    galaxies.push({
      x: left ? 8 + random() * 28 : 64 + random() * 28,
      y: 38 + random() * 20,
      size: 70 + random() * 80,
      tilt: -55 + random() * 110,
      glow: 0.45 + random() * 0.4,
      warm: random() < 0.25,
    });
  }
  return galaxies;
}
