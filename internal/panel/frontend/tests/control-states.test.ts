import { describe, expect, it } from 'vitest';

import { contrast, deltaE, mix, oklch, over, simulate, stateBand } from './color';
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

/** One state change and two, as the sidebar draws them. Defined once, in `./color`. */
const band = stateBand;

interface Control {
  readonly what: string;
  /** The ground an unselected option rests on. */
  readonly track: (palette: Palette) => string;
  /** The selected option's fill. */
  readonly thumb: (palette: Palette) => string;
  readonly hover: (palette: Palette) => string;
  readonly pressed: (palette: Palette) => string;
  /** The ink on the selected option, which has to stay legible on the thumb in every state. */
  readonly selectedText: (palette: Palette) => string;
  /** What the selection's own ground becomes under the pointer, rest excluded. */
  readonly thumbStates: (palette: Palette) => string[];
  /** The resting label on an unselected option. */
  readonly restingText: (palette: Palette) => string;
  /** The ink the component switches that label to once the ground darkens under the pointer. */
  readonly hoverText: (palette: Palette) => string;
}

/**
 * A veiled surface as it actually renders: the veil composited over what it is laid on.
 *
 * The segmented control's track, hover and press are translucent - one veil over another over the
 * page - so reading their declared colours measures the INK the veil is made of rather than the
 * surface a reader sees. That is why this exists: `palette.color()` answers with the colour and
 * drops the alpha, which reported a 5% ink veil as near-black and would have passed every step
 * check on numbers nothing renders.
 */
function veiled(palette: Palette, token: string, ground: string): string {
  const paint = palette.paint(token);

  return over(paint.color, ground, paint.alpha);
}

const controls: readonly Control[] = [
  {
    what: 'segmented control',
    /* Measured on the canvas. The veils are mixed over `transparent` precisely so the step is the
       same on any ground, and the canvas is the one every control can stand on. */
    track: (palette) => veiled(palette, 'segment-track', palette.color('canvas')),
    thumb: (palette) => palette.color('segment-thumb'),
    hover: (palette) =>
      veiled(palette, 'segment-hover', veiled(palette, 'segment-track', palette.color('canvas'))),
    pressed: (palette) =>
      veiled(palette, 'segment-pressed', veiled(palette, 'segment-track', palette.color('canvas'))),
    /* The thumb is the palette's accent, so the ink on it is the accent's own inverse rather than
       the brand ink meant to be READ on a page. */
    selectedText: (palette) => palette.color('on-brand-action'),
    /* The selection answers the pointer on the accent's own ramp - hover, then press - which is
       what a filled accent does everywhere else in the shell. */
    thumbStates: (palette) => [
      palette.color('brand-action-hover'),
      palette.color('brand-action-pressed'),
    ],
    restingText: (palette) => palette.color('text-secondary'),
    hoverText: (palette) => palette.color('text-primary'),
  },
  {
    what: 'sidebar navigation',
    track: (palette) => palette.color('sidebar-bg'),
    /* The selection is a solid pair now - the console's accent under its own inverse ink - rather
       than a near-white thumb carrying whatever active ink the palette happened to hold. */
    thumb: (palette) => palette.color('sidebar-active-bg'),
    hover: (palette) => palette.color('sidebar-item-hover'),
    pressed: (palette) => palette.color('sidebar-item-pressed'),
    selectedText: (palette) => palette.color('sidebar-item-active-text'),
    /* The selected row's pointer answers are elevation alone - the thumb lifts on hover and lands
       on press, fill untouched - so its ink only ever stands on the one ground. */
    thumbStates: () => [],
    restingText: (palette) => palette.color('sidebar-text-muted'),
    hoverText: (palette) => palette.color('sidebar-text'),
  },
  {
    // The same component, drawing on a sidebar popover's surfaces instead of the page's.
    what: 'segmented control on a sidebar surface',
    track: (palette) => veiled(palette, 'sidebar-seg-track', palette.color('sidebar-popover-bg')),
    thumb: (palette) => palette.color('segment-thumb'),
    hover: (palette) =>
      veiled(
        palette,
        'sidebar-seg-hover',
        veiled(palette, 'sidebar-seg-track', palette.color('sidebar-popover-bg')),
      ),
    pressed: (palette) =>
      veiled(
        palette,
        'sidebar-seg-pressed',
        veiled(palette, 'sidebar-seg-track', palette.color('sidebar-popover-bg')),
      ),
    selectedText: (palette) => palette.color('on-brand-action'),
    thumbStates: (palette) => [
      palette.color('brand-action-hover'),
      palette.color('brand-action-pressed'),
    ],
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
          // The FILL alone, with no ring to help it. A near-white thumb on a near-white track
          // separated by less than a press on a neighbouring option moved, and was propped up by a
          // hairline; a saturated accent needs no propping, which is why the ring is gone. If this
          // ever fails again the answer is a louder selection, not a ring to disguise a quiet one.
          expect(fill).toBeGreaterThan(deltaE(track, pressed));
        });

        it("keeps the selection's own states quieter than the selection itself", () => {
          // Acknowledging a pointer must not outshout the state it is acknowledging, and the press
          // that changes nothing should be the faintest thing on the control. A control whose
          // selected thumb answers with elevation alone has no fills to compare - the lift and
          // the landing are its whole acknowledgment.
          const [onHover, onPress] = control.thumbStates(palette);
          if (onHover === undefined || onPress === undefined) return;
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
          for (const ground of [thumb, ...control.thumbStates(palette)]) {
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
