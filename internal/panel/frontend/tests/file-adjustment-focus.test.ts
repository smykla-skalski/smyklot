// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { revealFileAdjustment } from '../src/lib/file-adjustment-link';

const { navigating } = vi.hoisted(() => ({
  navigating: { complete: null as Promise<void> | null },
}));
vi.mock('$app/state', () => ({ navigating }));

afterEach(() => {
  navigating.complete = null;
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

it.each(['complete', 'detach', 'abort'] as const)(
  'waits for navigation before the adjustment receives focus: %s',
  async (outcome) => {
    let complete!: () => void;
    let abort!: (reason: Error) => void;
    navigating.complete = new Promise<void>((resolve, reject) => {
      complete = resolve;
      abort = reject;
    });
    let frame: FrameRequestCallback | undefined;
    const schedule = vi.fn((callback: FrameRequestCallback) => {
      frame = callback;
      return 1;
    });
    vi.stubGlobal('requestAnimationFrame', schedule);
    vi.stubGlobal('cancelAnimationFrame', vi.fn());
    vi.stubGlobal('scrollBy', vi.fn());
    const node = document.createElement('div');
    node.tabIndex = -1;
    document.body.append(node);
    const cancel = revealFileAdjustment(node);
    expect(schedule).not.toHaveBeenCalled();
    if (outcome === 'detach') cancel();
    if (outcome === 'abort') abort(new Error('navigation aborted'));
    else complete();
    await Promise.resolve();
    if (outcome === 'complete') {
      expect(schedule).toHaveBeenCalledOnce();
      frame!(0);
      expect(document.activeElement).toBe(node);
    } else {
      expect(schedule).not.toHaveBeenCalled();
      expect(document.activeElement).not.toBe(node);
    }
  },
);
