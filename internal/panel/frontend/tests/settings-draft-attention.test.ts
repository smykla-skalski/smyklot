import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  SETTINGS_DRAFT_INACTIVITY_MS,
  SettingsDraftAttentionController,
  type SettingsDraftVisibilitySource,
} from '../src/lib/settings-draft-attention';

class VisibilitySource implements SettingsDraftVisibilitySource {
  visibilityState: DocumentVisibilityState = 'visible';
  private readonly listeners = new Set<() => void>();

  addEventListener(_type: 'visibilitychange', listener: () => void): void {
    this.listeners.add(listener);
  }

  removeEventListener(_type: 'visibilitychange', listener: () => void): void {
    this.listeners.delete(listener);
  }

  setVisibility(state: DocumentVisibilityState): void {
    this.visibilityState = state;
    for (const listener of this.listeners) listener();
  }

  get listenerCount(): number {
    return this.listeners.size;
  }
}

describe('SettingsDraftAttentionController [Unit]', () => {
  afterEach(() => vi.useRealTimers());

  it('marks a dirty tab only after it has been hidden for thirty minutes', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    const source = new VisibilitySource();
    const onAttention = vi.fn();
    const controller = new SettingsDraftAttentionController(source, onAttention);

    controller.update({ dirty: true, lastChangedAt: Date.now(), attentionAt: null });
    source.setVisibility('hidden');
    await vi.advanceTimersByTimeAsync(SETTINGS_DRAFT_INACTIVITY_MS - 1);

    expect(onAttention).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);

    expect(onAttention).toHaveBeenCalledOnce();
    controller.dispose();
  });

  it('rechecks elapsed time when a throttled background tab becomes visible', () => {
    let now = 10_000;
    const source = new VisibilitySource();
    const onAttention = vi.fn();
    const cancel = vi.fn();
    const controller = new SettingsDraftAttentionController(source, onAttention, {
      now: () => now,
      schedule: () => 1 as unknown as ReturnType<typeof setTimeout>,
      cancel,
    });

    controller.update({ dirty: true, lastChangedAt: now, attentionAt: null });
    source.setVisibility('hidden');
    now += SETTINGS_DRAFT_INACTIVITY_MS;
    source.setVisibility('visible');

    expect(onAttention).toHaveBeenCalledOnce();
    expect(cancel).toHaveBeenCalledOnce();
    controller.dispose();
  });

  it('gives a draft received in a hidden tab its own interval and cleans up', () => {
    let now = 500;
    const source = new VisibilitySource();
    source.visibilityState = 'hidden';
    const schedule = vi.fn(() => 1 as unknown as ReturnType<typeof setTimeout>);
    const cancel = vi.fn();
    const controller = new SettingsDraftAttentionController(source, vi.fn(), {
      now: () => now,
      schedule,
      cancel,
    });

    now += 5_000;
    controller.update({ dirty: true, lastChangedAt: now, attentionAt: null });

    expect(schedule).toHaveBeenCalledWith(expect.any(Function), SETTINGS_DRAFT_INACTIVITY_MS);

    controller.dispose();

    expect(cancel).toHaveBeenCalledOnce();
    expect(source.listenerCount).toBe(0);
  });

  it('does not schedule clean drafts or drafts already marked for attention', () => {
    const source = new VisibilitySource();
    source.visibilityState = 'hidden';
    const schedule = vi.fn(() => 1 as unknown as ReturnType<typeof setTimeout>);
    const controller = new SettingsDraftAttentionController(source, vi.fn(), { schedule });

    controller.update({ dirty: false, lastChangedAt: null, attentionAt: null });
    controller.update({ dirty: true, lastChangedAt: 1, attentionAt: 2 });

    expect(schedule).not.toHaveBeenCalled();
    controller.dispose();
  });
});
