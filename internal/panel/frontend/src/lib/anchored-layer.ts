/**
 * Where a floating layer goes, given what it hangs off and how much room there is.
 *
 * Every layer in the panel that hangs off a control - the menus, the pickers, the
 * suggestion list, the tooltip - was answering this same question in its own way,
 * and each answer was slightly different and slightly wrong. Two never flipped at
 * all. One flipped upwards whether or not there was more room up there. None of
 * them capped their own height, so a long list opened near the bottom of the
 * window simply ran off it.
 *
 * The arithmetic lives here, apart from the DOM, because it is the part worth
 * testing: it takes measurements and returns numbers, and a browser is not needed
 * to say what those numbers should be.
 */

/** Where the layer sits relative to its trigger. */
export type LayerSide = 'above' | 'below' | 'left' | 'right';

/**
 * Where it lines up along the other axis: for a layer above or below, which of
 * its vertical edges meets the trigger's; for one to the left or right, which of
 * its horizontal ones.
 */
export type LayerAlign = 'start' | 'center' | 'end';

export interface LayerSize {
  height: number;
  width: number;
}

export interface LayerRect extends LayerSize {
  bottom: number;
  left: number;
  right: number;
  top: number;
}

export interface LayerPlacement {
  align?: LayerAlign;
  /** Closest the layer may come to a viewport edge. */
  gutter?: number;
  /** Gap between the trigger and the layer. */
  offset?: number;
  /** Side to use when it fits. */
  side?: LayerSide;
}

export interface LayerPosition {
  /** Room along the side it was given, for the layer to cap itself against. */
  available: number;
  /** Room across the other axis, which is the whole viewport less its gutters. */
  crossAvailable: number;
  left: number;
  side: LayerSide;
  top: number;
}

const DEFAULT_GUTTER = 8;
const DEFAULT_OFFSET = 6;

const OPPOSITE: Record<LayerSide, LayerSide> = {
  above: 'below',
  below: 'above',
  left: 'right',
  right: 'left',
};

export function isVertical(side: LayerSide): boolean {
  return side === 'above' || side === 'below';
}

function clamp(value: number, min: number, max: number): number {
  // `min` last, so a layer bigger than the space left for it sits at the near
  // edge and overflows the far one. Backwards, and it hangs off the top of the
  // window with its first row out of reach.
  return Math.max(min, Math.min(value, max));
}

/** Room between the trigger and the viewport edge on one side. */
function roomOn(
  side: LayerSide,
  trigger: LayerRect,
  viewport: LayerSize,
  gutter: number,
  offset: number,
): number {
  switch (side) {
    case 'above':
      return trigger.top - offset - gutter;
    case 'below':
      return viewport.height - gutter - (trigger.bottom + offset);
    case 'left':
      return trigger.left - offset - gutter;
    case 'right':
      return viewport.width - gutter - (trigger.right + offset);
  }
}

/** Where the aligned edge wants to be, before it is held inside the viewport. */
function alignedStart(align: LayerAlign, start: number, extent: number, boxExtent: number): number {
  if (align === 'start') return start;
  if (align === 'center') return start + (extent - boxExtent) / 2;
  return start + extent - boxExtent;
}

/**
 * The preferred side when the layer fits there, the other when it does not and
 * the other does, and the preferred one again when neither fits.
 *
 * That last case is the one worth being deliberate about: with no room either
 * way the layer has to overflow something, and overflowing in the direction it
 * was asked for is at least predictable. It is also why the caller gets
 * `available` back - a layer that caps itself never reaches this case.
 */
export function placeLayer(
  trigger: LayerRect,
  box: LayerSize,
  viewport: LayerSize,
  placement: LayerPlacement = {},
): LayerPosition {
  const {
    align = 'start',
    gutter = DEFAULT_GUTTER,
    offset = DEFAULT_OFFSET,
    side = 'below',
  } = placement;

  const other = OPPOSITE[side];
  const preferredRoom = roomOn(side, trigger, viewport, gutter, offset);
  const otherRoom = roomOn(other, trigger, viewport, gutter, offset);
  const needed = isVertical(side) ? box.height : box.width;
  const chosen = needed <= preferredRoom || needed > otherRoom ? side : other;

  const vertical = isVertical(chosen);
  const available = chosen === side ? preferredRoom : otherRoom;
  const crossAvailable = (vertical ? viewport.width : viewport.height) - gutter * 2;

  let left: number;
  let top: number;

  if (vertical) {
    top = chosen === 'below' ? trigger.bottom + offset : trigger.top - offset - box.height;
    left = alignedStart(align, trigger.left, trigger.width, box.width);
  } else {
    left = chosen === 'right' ? trigger.right + offset : trigger.left - offset - box.width;
    top = alignedStart(align, trigger.top, trigger.height, box.height);
  }

  return {
    available,
    crossAvailable,
    left: clamp(left, gutter, viewport.width - box.width - gutter),
    side: chosen,
    top: clamp(top, gutter, viewport.height - box.height - gutter),
  };
}
