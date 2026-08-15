/**
 * A few stars that pulse on their own. The star layers breathe together -
 * whole sheets dimming and returning on long periods - and that evenness is
 * what these break: a handful of individuals, each with its own rhythm, its
 * own depth of dip and its own phase, dealt once per mount. They are sky
 * decoration like the galaxies, not easter eggs: no seat, no scheduler,
 * just CSS animating opacity.
 */

export type PulsarHue = 'white' | 'ice' | 'amber' | 'rose';

export interface Pulsar {
  /** Centre, in percent of the sky's box. */
  x: number;
  y: number;
  /** The star's box in px; core, bloom and haze scale inside it. */
  size: number;
  /** One full pulse, seconds. */
  duration: number;
  /** How far into its cycle it starts, so no two beat together. */
  phase: number;
  /** The dimmest it gets - some barely flicker, some nearly go out. */
  floor: number;
  hue: PulsarHue;
}

export function rollPulsars(random: () => number): Pulsar[] {
  const count = 5 + Math.floor(random() * 4);
  const pulsars: Pulsar[] = [];
  for (let i = 0; i < count; i += 1) {
    const hueRoll = random();
    pulsars.push({
      x: 3 + random() * 94,
      // The band a reader can see: below the window's top edge, above the
      // fade - the same reasoning as the galaxies'.
      y: 36 + random() * 26,
      size: 10 + random() * 16,
      duration: 1.6 + random() * 2.9,
      phase: random() * 4,
      floor: 0.15 + random() * 0.4,
      hue: hueRoll < 0.45 ? 'white' : hueRoll < 0.8 ? 'ice' : hueRoll < 0.92 ? 'amber' : 'rose',
    });
  }
  return pulsars;
}
