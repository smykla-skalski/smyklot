import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { contrast, deltaE, mix, oklch, simulate } from './color';
import { palettes, type Palette } from './theme';

/**
 * Hover, press and selection as colour rather than as text, in all four palettes a control renders
 * in.
 *
 * These values were picked by measurement and they are easy to undo by eye, because the wrong
 * direction looks fine on its own: a hovered option lifted toward white reads as a perfectly good
 * hover until you notice the selected thumb is also white. What that costs is the distance between
 * "hovered" and "selected", so that distance is what this pins - along with the hierarchy it sits
 * in, because a state that shouts louder than the state it acknowledges is the same defect wearing
 * different numbers.
 */

/** CIEDE2000 puts a just-noticeable difference at 1.0. */
const JND = 1;

/**
 * One state change and two, as the sidebar has drawn them since before any of this: 2.51 and 5.09
 * dE00 on its light ground, 5.07 and 8.92 on its dark one. The band is per ground rather than per
 * palette because the Root console pairs a light content area with a dark sidebar, and CIEDE2000
 * does not report the same number for the same perceived step at both ends of the lightness range.
 */
function band(ground: string): { hover: [number, number]; press: [number, number] } {
  return oklch(ground).L > 0.5
    ? { hover: [2, 3], press: [4.5, 5.7] }
    : // The dark floor is 7.0 rather than 7.5 because a press steps toward the darkest ground the
      // palette has, and one track - the sidebar popover's, on the dark panel - reaches black
      // before it reaches 8.92. Black is the end of the ramp, not a value chosen short of it.
      { hover: [4.5, 6], press: [7, 9.5] };
}

/** The share a component's own stylesheet mixes at, read from the file rather than copied. */
function shares(file: string, pattern: RegExp): number[] {
  const source = readFileSync(new URL(`../src/lib/components/${file}`, import.meta.url), 'utf8');
  const found = [...source.matchAll(pattern)].map((match) => Number(match.groups?.share) / 100);
  if (found.length !== 2) throw new Error(`${file} no longer has a hover and a press mix`);
  return found;
}

interface Control {
  readonly what: string;
  /** The ground an unselected option rests on. */
  readonly track: (palette: Palette) => string;
  /** The selected option's fill. */
  readonly thumb: (palette: Palette) => string;
  readonly hover: (palette: Palette) => string;
  readonly pressed: (palette: Palette) => string;
  /** The hairline in the thumb's shadow, composited over the track. */
  readonly ring: (palette: Palette) => string;
  /** The ink on the selected option, which has to stay legible on the thumb in every state. */
  readonly selectedText: (palette: Palette) => string;
  /** The shares the component mixes into the thumb for the selected option's own hover and press. */
  readonly thumbStates: () => number[];
  /** The resting label on an unselected option. */
  readonly restingText: (palette: Palette) => string;
  /** The ink the component switches that label to once the ground darkens under the pointer. */
  readonly hoverText: (palette: Palette) => string;
}

function ringOver(palette: Palette, token: string): string {
  const shadow = palette.declaration(token);
  const hairline = shadow.match(
    /0 0 0 0\.5px rgb\((?<r>\d+) (?<g>\d+) (?<b>\d+)\s*\/\s*(?<a>[\d.]+)%\)/u,
  );
  if (hairline === null) throw new Error(`--${token} has lost its hairline ring`);
  const channel = (raw: string | undefined): string =>
    Number(raw ?? 0)
      .toString(16)
      .padStart(2, '0');
  const color = `#${channel(hairline.groups?.r)}${channel(hairline.groups?.g)}${channel(hairline.groups?.b)}`;
  return color;
}

const controls: readonly Control[] = [
  {
    what: 'segmented control',
    track: (palette) => palette.color('segment-track'),
    thumb: (palette) => palette.color('segment-thumb'),
    hover: (palette) => palette.color('segment-hover'),
    pressed: (palette) => palette.color('segment-pressed'),
    ring: (palette) =>
      mix(
        ringOver(palette, 'segment-shadow'),
        palette.color('segment-track'),
        Number(
          palette.declaration('segment-shadow').match(/0\.5px rgb\([\d\s]+\/\s*([\d.]+)%\)/u)?.[1],
        ) / 100,
      ),
    selectedText: (palette) => palette.color('brand-action-text'),
    thumbStates: () =>
      shares(
        'SegmentedControl.svelte',
        /var\(--selected-text\) (?<share>[\d.]+)%, var\(--selected-bg\)/gu,
      ),
    restingText: (palette) => palette.color('text-muted'),
    hoverText: (palette) => palette.color('text-primary'),
  },
  {
    what: 'sidebar navigation',
    track: (palette) => palette.color('sidebar-bg'),
    thumb: (palette) => palette.color('sidebar-thumb'),
    hover: (palette) => palette.color('sidebar-item-hover'),
    pressed: (palette) => palette.color('sidebar-item-pressed'),
    ring: (palette) =>
      mix(
        ringOver(palette, 'sidebar-thumb-shadow'),
        palette.color('sidebar-bg'),
        Number(
          palette
            .declaration('sidebar-thumb-shadow')
            .match(/0\.5px rgb\([\d\s]+\/\s*([\d.]+)%\)/u)?.[1],
        ) / 100,
      ),
    selectedText: (palette) => palette.color('sidebar-item-active-text'),
    /* The shell redesign moved the selected row's pointer answers to elevation
       alone - the thumb lifts on hover and lands on press, fill untouched - so
       there are no mixes to read; the thumb's own colour is the only ground
       the selected ink ever stands on. */
    thumbStates: () => [],
    restingText: (palette) => palette.color('sidebar-text-muted'),
    hoverText: (palette) => palette.color('sidebar-text'),
  },
  {
    // The same component, drawing on a sidebar popover's surfaces instead of the page's.
    what: 'segmented control on a sidebar surface',
    track: (palette) => palette.color('sidebar-seg-track'),
    thumb: (palette) => palette.color('sidebar-seg-thumb'),
    hover: (palette) => palette.color('sidebar-seg-hover'),
    pressed: (palette) => palette.color('sidebar-seg-pressed'),
    ring: (palette) =>
      mix(
        ringOver(palette, 'sidebar-seg-shadow'),
        palette.color('sidebar-seg-track'),
        Number(
          palette
            .declaration('sidebar-seg-shadow')
            .match(/0\.5px rgb\([\d\s]+\/\s*([\d.]+)%\)/u)?.[1],
        ) / 100,
      ),
    selectedText: (palette) => palette.color('sidebar-menu-text'),
    thumbStates: () =>
      shares(
        'SegmentedControl.svelte',
        /var\(--selected-text\) (?<share>[\d.]+)%, var\(--selected-bg\)/gu,
      ),
    restingText: (palette) => palette.color('sidebar-menu-muted'),
    hoverText: (palette) => palette.color('sidebar-menu-text'),
  },
];

describe.each(palettes.map((palette) => [palette.name, palette] as const))(
  '%s',
  (_name, palette) => {
    describe.each(controls.map((control) => [control.what, control] as const))(
      '%s',
      (_what, control) => {
        const track = control.track(palette);
        const thumb = control.thumb(palette);
        const hover = control.hover(palette);
        const pressed = control.pressed(palette);
        const fill = deltaE(track, thumb);
        const ring = deltaE(control.ring(palette), track);
        const limits = band(track);

        it('moves the states in one direction, press further than hover', () => {
          const toward = Math.sign(deltaE(track, hover) === 0 ? 0 : 1);
          expect(toward).toBe(1);
          expect(deltaE(track, pressed)).toBeGreaterThan(deltaE(track, hover));
          // Both on the same side of the track, so hover and press are one gesture at two depths.
          const direction = (state: string): number =>
            Math.sign(oklch(state).L - oklch(track).L) || 0;
          expect(direction(pressed)).toBe(direction(hover));
        });

        it('sizes hover and press to the step the rest of the shell uses', () => {
          expect(deltaE(track, hover)).toBeGreaterThanOrEqual(limits.hover[0]);
          expect(deltaE(track, hover)).toBeLessThanOrEqual(limits.hover[1]);
          expect(deltaE(track, pressed)).toBeGreaterThanOrEqual(limits.press[0]);
          expect(deltaE(track, pressed)).toBeLessThanOrEqual(limits.press[1]);
        });

        it.each(['protan', 'deutan', 'tritan'] as const)(
          'carries the hover step through %s',
          (kind) => {
            // Two questions, and both matter. Can a dichromat still see the state change at all -
            // the absolute floor - and is the step carried by lightness rather than by hue? The
            // panel's own steps are pure lightness and score 0.92 to 1.00 on every deficiency. The
            // Root sidebar's hover is deliberately violet and keeps 0.72 under tritanopia, which
            // still leaves 3.70 dE00 on the screen. A step leaning on the accent instead of the
            // ground would land near 0.3 and fail both.
            const retained = deltaE(simulate(track, kind), simulate(hover, kind));
            expect(retained).toBeGreaterThan(2);
            expect(retained / deltaE(track, hover)).toBeGreaterThan(0.6);
          },
        );

        it('keeps every state further from the selected fill than a JND', () => {
          // The whole point. A hovered or pressed option that lands on the selected one's colour
          // says "selected" while meaning "your pointer is here".
          expect(deltaE(hover, thumb)).toBeGreaterThan(JND);
          expect(deltaE(pressed, thumb)).toBeGreaterThan(JND);
        });

        it('leaves the selection louder than the loudest state beside it', () => {
          // The fill alone is often not enough: a white thumb on a near-white track separates by
          // less than a press on a neighbouring option moves. Where that is true the ring is what
          // puts the selected option back on top, and where the fill is already wide the ring is
          // free to stay quiet.
          expect(Math.max(fill, ring)).toBeGreaterThan(deltaE(track, pressed));
        });

        it("keeps the selection's own states quieter than the selection itself", () => {
          // Acknowledging a pointer must not outshout the state it is acknowledging, and the press
          // that changes nothing should be the faintest thing on the control. A control whose
          // selected thumb answers with elevation alone has no fills to compare - the lift and
          // the landing are its whole acknowledgment.
          const states = control.thumbStates();
          if (states.length === 0) return;
          const [onHover, onPress] = states.map((share) =>
            mix(control.selectedText(palette), thumb, share),
          );
          if (onHover === undefined || onPress === undefined)
            throw new Error('missing thumb state');
          expect(deltaE(thumb, onHover)).toBeLessThan(fill);
          expect(deltaE(thumb, onPress)).toBeLessThan(fill);
          expect(deltaE(thumb, onHover)).toBeLessThan(deltaE(thumb, onPress));
          // Still perceptible: a state nobody can see is not a state.
          expect(deltaE(thumb, onHover)).toBeGreaterThan(JND);
        });

        it('carries a label that meets WCAG AA on every ground it lands on', () => {
          // The resting option is muted on the plain track; hovering darkens the ground, so the
          // component darkens the label with it. Both halves of that pair have to pass.
          expect(contrast(control.restingText(palette), track)).toBeGreaterThanOrEqual(4.5);
          for (const ground of [hover, pressed]) {
            expect(contrast(control.hoverText(palette), ground)).toBeGreaterThanOrEqual(4.5);
          }
          const selected = control.selectedText(palette);
          for (const ground of [
            thumb,
            ...control.thumbStates().map((share) => mix(selected, thumb, share)),
          ]) {
            expect(contrast(selected, ground)).toBeGreaterThanOrEqual(4.5);
          }
        });
      },
    );

    describe('buttons', () => {
      const hoverLayer = palette.paint('interactive-hover-layer');
      const press = palette.paint('interactive-pressed');

      it('moves hover and press the same way', () => {
        // A hover that lifts and a press that darkens are two ideas, not one gesture at two
        // depths. The dark palettes lifted on hover and then dropped 30% black on press.
        const ground = palette.color('surface-raised');
        const hovered = mix(hoverLayer.color, ground, hoverLayer.alpha);
        const pressed = mix(press.color, ground, press.alpha);
        const direction = (state: string): number => Math.sign(oklch(state).L - oklch(ground).L);
        expect(direction(hovered)).toBe(direction(pressed));
        expect(deltaE(ground, pressed)).toBeGreaterThan(deltaE(ground, hovered));
      });

      it.each(['canvas', 'surface-base', 'surface-raised', 'surface-inset'])(
        'holds the same step on %s',
        (name) => {
          // These are overlays rather than fills precisely so that one value is right on every
          // ground a button can sit on. A flat token cannot be: --interactive-hover resolved to
          // --surface-raised on dark, which is the default button's own resting colour.
          const ground = palette.color(name);
          const limits = band(ground);
          const hovered = mix(hoverLayer.color, ground, hoverLayer.alpha);
          const pressed = mix(press.color, ground, press.alpha);
          expect(deltaE(ground, hovered)).toBeGreaterThanOrEqual(limits.hover[0]);
          expect(deltaE(ground, hovered)).toBeLessThanOrEqual(limits.hover[1]);
          expect(deltaE(ground, pressed)).toBeGreaterThanOrEqual(limits.press[0]);
          expect(deltaE(ground, pressed)).toBeLessThanOrEqual(limits.press[1]);
          expect(contrast(palette.color('text-primary'), hovered)).toBeGreaterThanOrEqual(4.5);
        },
      );

      it('walks the primary button one way along its ramp', () => {
        // Rest, hover, press: the fill has to keep going in the direction it started, or hovering
        // and then pressing moves it lighter and then darker than where it began.
        const [rest, hovered, pressed] = [
          palette.color('brand-action'),
          palette.color('brand-action-hover'),
          palette.color('brand-action-pressed'),
        ];
        const step = Math.sign(oklch(hovered).L - oklch(rest).L);
        expect(step).not.toBe(0);
        expect(Math.sign(oklch(pressed).L - oklch(hovered).L)).toBe(step);
        expect(deltaE(rest, pressed)).toBeGreaterThan(deltaE(rest, hovered));
        for (const ground of [rest, hovered, pressed]) {
          expect(contrast(palette.color('on-brand-action'), ground)).toBeGreaterThanOrEqual(4.5);
        }
      });
    });
  },
);
