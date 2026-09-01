// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';

import { collapse } from '../src/lib/live-list.svelte';

/**
 * What a row does on its way out.
 *
 * The overview cards draw the queue as it moves, and a row that vanishes outright takes
 * its height with it in one frame - the card shuts, and every card below it jumps. The
 * fade a table can afford is not enough here, because a list of blocks has nothing
 * holding the space while the row is still there.
 *
 * So this measures the one thing that could be wrong about it: whether every box
 * property that contributes to the row's height comes down with the opacity, and whether
 * the words inside are clipped rather than left to overflow a box that is shrinking
 * under them.
 */
describe('a row leaving a live list [Unit]', () => {
  function row(): HTMLElement {
    const element = document.createElement('div');
    /* jsdom computes no layout, so the height and padding are declared rather than
       measured - which is exactly what `collapse` reads. */
    element.style.blockSize = '64px';
    element.style.paddingBlockStart = '10px';
    element.style.paddingBlockEnd = '6px';
    document.body.append(element);

    return element;
  }

  it('gives the space back as it fades, and clips while it does', () => {
    const { css, duration } = collapse(row(), { duration: 140 });
    expect(duration).toBe(140);
    expect(css).toBeDefined();

    const whole = css!(1, 0);
    expect(whole).toContain('opacity: 1');
    expect(whole).toContain('block-size: 64px');
    expect(whole).toContain('padding-block: 10px 6px');
    /* Without this the text keeps its own height inside a box that is losing its own,
       and the row's words spill over the row below while it goes. */
    expect(whole).toContain('overflow: hidden');

    const half = css!(0.5, 0.5);
    expect(half).toContain('opacity: 0.5');
    expect(half).toContain('block-size: 32px');
    expect(half).toContain('padding-block: 5px 3px');

    const gone = css!(0, 1);
    expect(gone).toContain('block-size: 0px');
    expect(gone).toContain('padding-block: 0px 0px');
  });

  it('takes no space at all from a row it cannot measure', () => {
    const bare = document.createElement('div');
    document.body.append(bare);
    const { css } = collapse(bare);

    /* jsdom reports an empty string for an unset length, which parses as NaN. A NaN in
       the emitted CSS is a declaration the browser drops, so the row would keep its full
       height for the whole transition and then snap - the exact fault this exists to
       prevent, arriving silently. */
    expect(css!(0.5, 0.5)).not.toContain('NaN');
  });
});
