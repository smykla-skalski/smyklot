// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import MutationReceipt from '../src/lib/components/MutationReceipt.svelte';
import { receipts } from '../src/lib/receipts.svelte';

describe('MutationReceipt [Component]', () => {
  beforeEach(() => {
    receipts.clear();
    vi.useFakeTimers();
  });
  afterEach(() => {
    receipts.clear();
    vi.useRealTimers();
  });

  it('expires ordinary success receipts after five seconds', async () => {
    receipts.say('Updated labels');
    render(MutationReceipt);
    await vi.advanceTimersByTimeAsync(4_999);
    expect(receipts.current?.say).toBe('Updated labels');
    await vi.advanceTimersByTimeAsync(1);
    expect(receipts.current).toBeNull();
  });

  it('keeps sticky decisions and their queued receipt until dismissed', async () => {
    receipts.say('Paused sync', { sticky: true });
    render(MutationReceipt);
    receipts.say('Updated labels');
    await vi.advanceTimersByTimeAsync(10_000);
    expect(receipts.current?.say).toBe('Paused sync');
    await fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(receipts.current?.say).toBe('Updated labels');
    await vi.advanceTimersByTimeAsync(5_000);
    expect(receipts.current).toBeNull();
  });

  it('retains keyboard focus hold after the pointer leaves', async () => {
    receipts.say('Updated labels');
    render(MutationReceipt);
    const toast = screen.getByRole('status');
    await fireEvent.pointerEnter(toast);
    await fireEvent.focusIn(toast);
    await fireEvent.pointerLeave(toast);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(receipts.current?.say).toBe('Updated labels');
    await fireEvent.focusOut(toast, { relatedTarget: null });
    await vi.advanceTimersByTimeAsync(5_000);
    expect(receipts.current).toBeNull();
  });
});
