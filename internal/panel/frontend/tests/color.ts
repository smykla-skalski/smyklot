/**
 * The colour arithmetic the theme's own rules are checked against.
 *
 * One implementation, because a palette whose rules are asserted by two copies of the maths is a
 * palette with two definitions of "readable". Nothing in the running app imports this; it exists so
 * that a claim about the design system - the contrast floor, the size of a state step, the chroma
 * ceiling a tint has to stay under - is a number a test can recompute rather than a comment.
 */

/** A `#rrggbb` string as three 0..1 channels. */
export function channels(color: string): [number, number, number] {
  const hex = color.replace('#', '');
  const pairs = hex.match(/.{2}/gu);
  if (pairs === null || pairs.length !== 3) throw new Error(`not a #rrggbb colour: ${color}`);
  const [red = 0, green = 0, blue = 0] = pairs.map((pair) => Number.parseInt(pair, 16) / 255);
  return [red, green, blue];
}

function toHex(rgb: readonly number[]): string {
  return `#${rgb
    .map((channel) =>
      Math.round(Math.min(1, Math.max(0, channel)) * 255)
        .toString(16)
        .padStart(2, '0'),
    )
    .join('')}`;
}

/** sRGB's transfer function, undone. Every perceptual quantity below needs linear light. */
const linear = (channel: number): number =>
  channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;

const gamma = (channel: number): number =>
  channel <= 0.0031308 ? channel * 12.92 : 1.055 * channel ** (1 / 2.4) - 0.055;

/**
 * What `color-mix(in srgb, top share%, base)` computes, so a test reads the real recipe rather
 * than a hand-copied result.
 */
export function mix(top: string, base: string, share: number): string {
  const over = channels(top);
  const under = channels(base);
  return toHex(over.map((channel, index) => channel * share + (under[index] ?? 0) * (1 - share)));
}

/**
 * `top` at `alpha` composited over `base`, which is what a translucent overlay actually paints.
 * Every pressed state in this theme is an overlay rather than a fill, so its measured colour only
 * exists once you know what it sat on.
 */
export const over = (top: string, base: string, alpha: number): string => mix(top, base, alpha);

export function relativeLuminance(color: string): number {
  const [red, green, blue] = channels(color).map(linear) as [number, number, number];
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
}

/** WCAG 2.x contrast ratio, whose floor for body text is 4.5 and for a boundary 3. */
export function contrast(foreground: string, background: string): number {
  const [high = 0, low = 0] = [relativeLuminance(foreground), relativeLuminance(background)].sort(
    (first, second) => second - first,
  );
  return (high + 0.05) / (low + 0.05);
}

function lab(color: string): [number, number, number] {
  const [red, green, blue] = channels(color).map(linear) as [number, number, number];
  const white = [0.95047, 1, 1.08883];
  const raw = [
    red * 0.4124564 + green * 0.3575761 + blue * 0.1804375,
    red * 0.2126729 + green * 0.7151522 + blue * 0.072175,
    red * 0.0193339 + green * 0.119192 + blue * 0.9503041,
  ];
  const f = (value: number): number =>
    value > 216 / 24389 ? Math.cbrt(value) : (841 / 108) * value + 4 / 29;
  const [x = 0, y = 0, z = 0] = raw.map((value, index) => f(value / (white[index] ?? 1)));
  return [116 * y - 16, 500 * (x - y), 200 * (y - z)];
}

/**
 * CIEDE2000, whose own scale puts a just-noticeable difference at 1.0.
 *
 * Used rather than a plain OKLab distance because the literature's thresholds are quoted on this
 * scale, so "2.5 is one state change" is a figure with a source rather than a taste.
 */
export function deltaE(first: string, second: string): number {
  const [L1, a1, b1] = lab(first);
  const [L2, a2, b2] = lab(second);
  const rad = Math.PI / 180;
  const meanC = (Math.hypot(a1, b1) + Math.hypot(a2, b2)) / 2;
  const G = 0.5 * (1 - Math.sqrt(meanC ** 7 / (meanC ** 7 + 25 ** 7)));
  const ap1 = a1 * (1 + G);
  const ap2 = a2 * (1 + G);
  const Cp1 = Math.hypot(ap1, b1);
  const Cp2 = Math.hypot(ap2, b2);
  const angle = (y: number, x: number): number => {
    if (y === 0 && x === 0) return 0;
    const degrees = Math.atan2(y, x) / rad;
    return degrees >= 0 ? degrees : degrees + 360;
  };
  const hp1 = angle(b1, ap1);
  const hp2 = angle(b2, ap2);
  const dL = L2 - L1;
  const dC = Cp2 - Cp1;
  let dh = 0;
  if (Cp1 * Cp2 !== 0) {
    dh = hp2 - hp1;
    if (dh > 180) dh -= 360;
    else if (dh < -180) dh += 360;
  }
  const dH = 2 * Math.sqrt(Cp1 * Cp2) * Math.sin((dh * rad) / 2);
  const meanL = (L1 + L2) / 2;
  const meanCp = (Cp1 + Cp2) / 2;
  let meanH = hp1 + hp2;
  if (Cp1 * Cp2 !== 0) {
    if (Math.abs(hp1 - hp2) > 180) meanH += hp1 + hp2 < 360 ? 360 : -360;
    meanH /= 2;
  }
  const T =
    1 -
    0.17 * Math.cos((meanH - 30) * rad) +
    0.24 * Math.cos(2 * meanH * rad) +
    0.32 * Math.cos((3 * meanH + 6) * rad) -
    0.2 * Math.cos((4 * meanH - 63) * rad);
  const Sl = 1 + (0.015 * (meanL - 50) ** 2) / Math.sqrt(20 + (meanL - 50) ** 2);
  const Sc = 1 + 0.045 * meanCp;
  const Sh = 1 + 0.015 * meanCp * T;
  const Rt =
    -2 *
    Math.sqrt(meanCp ** 7 / (meanCp ** 7 + 25 ** 7)) *
    Math.sin(60 * Math.exp(-(((meanH - 275) / 25) ** 2)) * rad);
  return Math.sqrt((dL / Sl) ** 2 + (dC / Sc) ** 2 + (dH / Sh) ** 2 + Rt * (dC / Sc) * (dH / Sh));
}

/** OKLCh, for reading a colour's lightness and how much chroma it is carrying. */
export function oklch(color: string): { L: number; C: number; H: number } {
  const [red, green, blue] = channels(color).map(linear) as [number, number, number];
  const long = Math.cbrt(0.4122214708 * red + 0.5363325363 * green + 0.0514459929 * blue);
  const medium = Math.cbrt(0.2119034982 * red + 0.6806995451 * green + 0.1073969566 * blue);
  const short = Math.cbrt(0.0883024619 * red + 0.2817188376 * green + 0.6299787005 * blue);
  const L = 0.2104542553 * long + 0.793617785 * medium - 0.0040720468 * short;
  const a = 1.9779984951 * long - 2.428592205 * medium + 0.4505937099 * short;
  const b = 0.0259040371 * long + 0.7827717662 * medium - 0.808675766 * short;
  const hue = (Math.atan2(b, a) * 180) / Math.PI;
  return { L, C: Math.hypot(a, b), H: hue < 0 ? hue + 360 : hue };
}

export type Dichromacy = 'protan' | 'deutan' | 'tritan';

/**
 * Viénot, Brettel and Mollon 1999.
 *
 * Here to answer one question: is a step carried by lightness or by hue? A step a dichromat sees
 * at nearly full size is carried by lightness; one that halves under deuteranopia was leaning on
 * chroma, and roughly one man in twelve will not see what it was saying.
 */
export function simulate(color: string, kind: Dichromacy): string {
  const [red, green, blue] = channels(color).map(linear) as [number, number, number];
  const L = 17.8824 * red + 43.5161 * green + 4.11935 * blue;
  const M = 3.45565 * red + 27.1554 * green + 3.86714 * blue;
  const S = 0.0299566 * red + 0.184309 * green + 1.46709 * blue;
  const long = kind === 'protan' ? 2.02344 * M - 2.52581 * S : L;
  const medium = kind === 'deutan' ? 0.494207 * L + 1.24827 * S : M;
  const short = kind === 'tritan' ? -0.395913 * L + 0.801109 * M : S;
  return toHex(
    [
      0.080944 * long - 0.130504 * medium + 0.116721 * short,
      -0.0102485 * long + 0.0540194 * medium - 0.113615 * short,
      -0.000365294 * long - 0.00412163 * medium + 0.693513 * short,
    ].map(gamma),
  );
}
