// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';

import { collapse, LiveList } from '../src/lib/live-list.svelte';

interface Row {
  id: string;
  says: string;
}

/**
 * What the two live overview cards do, and what they refuse to do.
 *
 * A row's contents follow the service; the set of rows follows the reader. Everything
 * below is one of those two halves - the half that has to stay current, and the half
 * that must not move until somebody asks.
 */
describe('a live list [Unit]', () => {
  /* The live list is a getter the card owns, so a plain closure over a `let` is exactly
     what the component gives it. Both answers are read through that getter every time,
     which is what makes them measurable without a component around them. */
  function list(initial: Row[]) {
    let live = initial;

    return {
      set: (next: Row[]) => (live = next),
      shown: new LiveList(
        () => live,
        (row) => row.id,
      ),
    };
  }

  it("reads every shown row fresh, so its words are the service's own", () => {
    const { set, shown } = list([{ id: 'a', says: 'waiting' }]);
    expect(shown.rows.map((row) => row.says)).toEqual(['waiting']);

    set([{ id: 'a', says: 'running' }]);

    /* The same row, the same place, the new word. This is the half that is live. */
    expect(shown.rows.map((row) => row.says)).toEqual(['running']);
    expect(shown.changed).toBe(0);
  });

  it('keeps the set still when the service adds and removes, and counts both', () => {
    const { set, shown } = list([
      { id: 'a', says: 'one' },
      { id: 'b', says: 'two' },
    ]);
    expect(shown.rows).toHaveLength(2);

    set([
      { id: 'b', says: 'two' },
      { id: 'c', says: 'three' },
    ]);

    /* `a` has gone and `c` has arrived, and the card reports neither of those by moving:
       it shows the pair it was given and says two things have changed. */
    expect(shown.rows.map((row) => row.id)).toEqual(['a', 'b']);
    expect(shown.changed).toBe(2);

    shown.refresh();

    expect(shown.rows.map((row) => row.id)).toEqual(['b', 'c']);
    expect(shown.changed).toBe(0);
  });

  it('draws a row that has left from the last copy it saw', () => {
    const { set, shown } = list([{ id: 'a', says: 'running' }]);
    expect(shown.rows).toHaveLength(1);

    set([]);

    /* Still on screen, still readable, because the reader has not asked for it to go -
       and a card cannot render a row whose data it threw away. */
    expect(shown.rows.map((row) => row.says)).toEqual(['running']);
    expect(shown.changed).toBe(1);
  });

  it('takes what arrives while it has nothing to protect', () => {
    /* The first load: the query answers empty before it answers with rows. A card that
       froze on that would open by announcing that everything on it had changed. */
    const { set, shown } = list([]);
    expect(shown.rows).toHaveLength(0);

    set([{ id: 'a', says: 'one' }]);

    expect(shown.rows.map((row) => row.id)).toEqual(['a']);
    expect(shown.changed).toBe(0);
  });
});

/**
 * What a row does on its way out.
 *
 * A row that vanishes outright takes its height with it in one frame - the card shuts,
 * and every card below it jumps. So this measures the one thing that could be wrong
 * about the way out: whether every box property contributing to the row's height comes
 * down with the opacity, and whether the words inside are clipped rather than left to
 * overflow a box that is shrinking under them.
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
    /* A grid item's floor is its own content, so the track will not follow the box down
       without it - the row shrinks and the card does not. */
    expect(whole).toContain('min-block-size: 0');

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
