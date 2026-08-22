// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';

import { revealInline } from '../src/lib/reveal-inline';

function rectangle(left: number, right: number): DOMRect {
  return { left, right } as DOMRect;
}

describe('revealInline [Unit]', () => {
  it('scrolls a clipped item in from the trailing edge', () => {
    const container = document.createElement('div');
    const item = document.createElement('a');
    container.scrollLeft = 0;
    container.getBoundingClientRect = () => rectangle(20, 320);
    item.getBoundingClientRect = () => rectangle(290, 375);

    revealInline(container, item);

    expect(container.scrollLeft).toBe(55);
  });

  it('scrolls a clipped item in from the leading edge', () => {
    const container = document.createElement('div');
    const item = document.createElement('a');
    container.scrollLeft = 80;
    container.getBoundingClientRect = () => rectangle(20, 320);
    item.getBoundingClientRect = () => rectangle(-10, 75);

    revealInline(container, item);

    expect(container.scrollLeft).toBe(50);
  });

  it('leaves a visible item where the user put it', () => {
    const container = document.createElement('div');
    const item = document.createElement('a');
    container.scrollLeft = 40;
    container.getBoundingClientRect = () => rectangle(20, 320);
    item.getBoundingClientRect = () => rectangle(90, 175);

    revealInline(container, item);

    expect(container.scrollLeft).toBe(40);
  });
});
