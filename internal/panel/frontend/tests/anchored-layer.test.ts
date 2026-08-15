import { describe, expect, it } from 'vitest';

import { placeLayer, type LayerRect, type LayerSize } from '../src/lib/anchored-layer';

/**
 * The cases here are the ones the hand-rolled placements got wrong, one per
 * component that had its own copy.
 */

const VIEWPORT: LayerSize = { height: 800, width: 1200 };

/** A trigger, described the way `getBoundingClientRect` would. */
function trigger(left: number, top: number, width: number, height: number): LayerRect {
  return { bottom: top + height, height, left, right: left + width, top, width };
}

describe('an anchored layer', () => {
  it('hangs below its trigger when there is room', () => {
    const at = placeLayer(trigger(100, 100, 120, 30), { height: 200, width: 160 }, VIEWPORT);

    expect(at.side).toBe('below');
    expect(at.top).toBe(136);
  });

  it('flips above when below would run off the bottom', () => {
    const at = placeLayer(trigger(100, 700, 120, 30), { height: 200, width: 160 }, VIEWPORT);

    expect(at.side).toBe('above');
    expect(at.top).toBe(494);
  });

  it('stays below when neither side fits', () => {
    // A layer taller than the window has to overflow something. Overflowing the
    // way it was asked for keeps its first row where the reader is looking; the
    // old code flipped regardless and put the first row off the top instead.
    const at = placeLayer(trigger(100, 400, 120, 30), { height: 900, width: 160 }, VIEWPORT);

    expect(at.side).toBe('below');
  });

  it('flips downwards too, not only up', () => {
    // Asked for above, with 26px of room up there and 716 below. Every
    // hand-rolled copy of this could only ever flip one way, so a layer that
    // wanted to open upwards opened off the top of the window instead.
    const at = placeLayer(trigger(100, 40, 120, 30), { height: 200, width: 160 }, VIEWPORT, {
      side: 'above',
    });

    expect(at.side).toBe('below');
    expect(at.top).toBe(76);
  });

  it('reports the room it has, so a long list can cap itself', () => {
    const at = placeLayer(trigger(100, 100, 120, 30), { height: 200, width: 160 }, VIEWPORT);

    // 800 - 8 gutter - (130 bottom + 6 offset)
    expect(at.available).toBe(656);
  });

  describe('alignment', () => {
    const anchor = trigger(500, 100, 120, 30);
    const box: LayerSize = { height: 100, width: 200 };

    it('lines its start edge up with the trigger', () => {
      expect(placeLayer(anchor, box, VIEWPORT, { align: 'start' }).left).toBe(500);
    });

    it('lines its end edge up with the trigger', () => {
      expect(placeLayer(anchor, box, VIEWPORT, { align: 'end' }).left).toBe(420);
    });

    it('centres it on the trigger', () => {
      expect(placeLayer(anchor, box, VIEWPORT, { align: 'center' }).left).toBe(460);
    });
  });

  describe('to the side', () => {
    // What a collapsed sidebar needs: its menus fly out beside the rail rather
    // than hanging under it.
    it('sits beside the trigger, aligned to its top', () => {
      const at = placeLayer(trigger(60, 400, 48, 40), { height: 200, width: 260 }, VIEWPORT, {
        side: 'right',
      });

      expect(at.side).toBe('right');
      expect(at.left).toBe(114);
      expect(at.top).toBe(400);
    });

    it('flips to the other side when there is no room beside it', () => {
      const at = placeLayer(trigger(1100, 400, 48, 40), { height: 200, width: 260 }, VIEWPORT, {
        side: 'right',
      });

      expect(at.side).toBe('left');
      expect(at.left).toBe(834);
    });

    it('lines the bottoms up when end-aligned', () => {
      // The account menu's shape: it sits beside the rail with its foot level
      // with the trigger's, so 540 + 200 is the trigger's own bottom at 740.
      const at = placeLayer(trigger(60, 700, 48, 40), { height: 200, width: 260 }, VIEWPORT, {
        align: 'end',
        side: 'right',
      });

      expect(at.top).toBe(540);
    });

    it('holds a side-placed layer inside the bottom gutter', () => {
      // End-aligned against a trigger on the floor, the layer would start at 600
      // and run past the viewport, so it is pulled back to 592.
      const at = placeLayer(trigger(60, 760, 48, 40), { height: 200, width: 260 }, VIEWPORT, {
        align: 'end',
        side: 'right',
      });

      expect(at.top).toBe(592);
    });

    it('reports the cross-axis room a side-placed layer has for its height', () => {
      const at = placeLayer(trigger(60, 400, 48, 40), { height: 200, width: 260 }, VIEWPORT, {
        side: 'right',
      });

      expect(at.crossAvailable).toBe(784);
    });
  });

  describe('viewport edges', () => {
    it('never crosses the right gutter', () => {
      // Aligned to a trigger's start edge, a wide menu would reach past the window.
      const at = placeLayer(trigger(1100, 100, 60, 30), { height: 100, width: 300 }, VIEWPORT);

      expect(at.left).toBe(892);
    });

    it('never crosses the left gutter', () => {
      // End-aligned off a narrow trigger near the left edge: the layer's start
      // edge lands at -220 before it is clamped.
      const at = placeLayer(trigger(20, 100, 60, 30), { height: 100, width: 300 }, VIEWPORT, {
        align: 'end',
      });

      expect(at.left).toBe(8);
    });

    it('keeps the near edge visible when the layer is wider than the window', () => {
      const at = placeLayer(trigger(100, 100, 60, 30), { height: 100, width: 2000 }, VIEWPORT);

      expect(at.left).toBe(8);
    });
  });
});
