import { describe, expect, it } from 'vitest';

import { placeTooltip } from '../src/lib/tooltip';

const VIEWPORT = { height: 800, width: 1200 };
const BOX = { height: 100, width: 272 };

function trigger(
  left: number,
  top: number,
): {
  bottom: number;
  height: number;
  left: number;
  right: number;
  top: number;
  width: number;
} {
  return { bottom: top + 18, height: 18, left, right: left + 18, top, width: 18 };
}

describe('placeTooltip', () => {
  it('hangs below the trigger, aligned on the requested edge', () => {
    expect(placeTooltip(trigger(600, 200), BOX, VIEWPORT, 'end')).toEqual({ left: 346, top: 224 });
    expect(placeTooltip(trigger(600, 200), BOX, VIEWPORT, 'start')).toEqual({
      left: 600,
      top: 224,
    });
    expect(placeTooltip(trigger(600, 200), BOX, VIEWPORT, 'center')).toEqual({
      left: 473,
      top: 224,
    });
  });

  it('flips above the trigger when there is no room below', () => {
    expect(placeTooltip(trigger(600, 720), BOX, VIEWPORT, 'end').top).toBe(614);
  });

  it('keeps the gutter when neither side fits', () => {
    const tall = { height: 900, width: 272 };
    expect(placeTooltip(trigger(600, 400), tall, VIEWPORT, 'end').top).toBe(16);
  });

  it('never crosses a viewport edge', () => {
    expect(placeTooltip(trigger(8, 200), BOX, VIEWPORT, 'end').left).toBe(16);
    expect(placeTooltip(trigger(1180, 200), BOX, VIEWPORT, 'start').left).toBe(912);
  });
});
