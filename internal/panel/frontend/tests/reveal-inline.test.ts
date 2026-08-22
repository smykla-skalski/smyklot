// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest';

import { observeInlineSelection, revealInline } from '../src/lib/reveal-inline';

function rectangle(left: number, right: number): DOMRect {
  return { left, right } as DOMRect;
}

describe('revealInline [Unit]', () => {
  afterEach(() => vi.unstubAllGlobals());
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

  it('reveals the selected item after its container becomes narrower', () => {
    let resize!: () => void;
    const disconnectSpy = vi.fn();
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(callback: () => void) {
          resize = callback;
        }
        observe(): void {}
        disconnect = disconnectSpy;
      },
    );
    const container = document.createElement('nav');
    const item = document.createElement('a');
    item.setAttribute('aria-current', 'page');
    container.append(item);
    let frameRight = 640;
    container.getBoundingClientRect = () => rectangle(0, frameRight);
    item.getBoundingClientRect = () =>
      rectangle(520 - container.scrollLeft, 620 - container.scrollLeft);

    const stop = observeInlineSelection(container);
    expect(container.scrollLeft).toBe(0);

    frameRight = 320;
    resize();
    expect(container.scrollLeft).toBe(300);

    stop();
    expect(disconnectSpy).toHaveBeenCalledOnce();
  });
});
