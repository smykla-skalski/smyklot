// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

import SettingsSaveComposer from '../src/lib/components/SettingsSaveComposer.svelte';

const base = {
  count: 2,
  onSave: vi.fn(),
  onDiscard: vi.fn(),
  onResolveConflict: vi.fn(),
  onDismiss: vi.fn(),
};

describe('SettingsSaveComposer [Component]', () => {
  afterEach(() => vi.useRealTimers());
  it('describes one workspace-wide draft and exposes one Save and Discard pair', async () => {
    const onSave = vi.fn();
    const onDiscard = vi.fn();
    render(SettingsSaveComposer, { ...base, onSave, onDiscard });

    expect(screen.getByText('2 changed settings')).toBeTruthy();
    expect(
      screen.getByText('Review anywhere in this workspace, then save everything together'),
    ).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

    expect(onSave).toHaveBeenCalledOnce();
    expect(onDiscard).toHaveBeenCalledOnce();
  });

  it('keeps a conflicted draft and updates it before another save', async () => {
    const onResolveConflict = vi.fn();
    render(SettingsSaveComposer, {
      ...base,
      conflict: true,
      problem: 'Settings changed in another session',
      onResolveConflict,
    });

    expect(screen.getByText('Your draft is still safe')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Update draft' }));
    expect(onResolveConflict).toHaveBeenCalledOnce();
  });

  it('explains a conflict received from another open tab', () => {
    render(SettingsSaveComposer, { ...base, count: 1, conflict: true });

    expect(screen.getByText('Your draft is still safe')).toBeTruthy();
    expect(screen.getByText('Settings also changed in another open tab')).toBeTruthy();
  });

  it('links a validation problem to the control section without hijacking modified clicks', async () => {
    const onOpenProblem = vi.fn();
    render(SettingsSaveComposer, {
      ...base,
      problem: 'A label name is required',
      problemHref: '#/sync/labels',
      problemLabel: 'Labels',
      onOpenProblem,
    });
    const link = screen.getByRole('link', { name: 'Open Labels' });

    await fireEvent.click(link);
    expect(onOpenProblem).toHaveBeenCalledOnce();
    await fireEvent.click(link, { metaKey: true });
    expect(onOpenProblem).toHaveBeenCalledOnce();
  });

  it('disables saving for a read-only viewer without hiding their recovered draft', () => {
    render(SettingsSaveComposer, { ...base, count: 1, readOnly: true });

    expect(screen.getByText('1 changed setting')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: 'Discard' }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it('blocks saving while an editor holds an invalid value', () => {
    render(SettingsSaveComposer, {
      ...base,
      invalidProblem: 'Line width must be from 40 to 320',
    });

    expect(screen.getByText('Fix the invalid setting before saving')).toBeTruthy();
    expect(screen.getByText('Line width must be from 40 to 320')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: 'Discard' }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it('returns to an invalid file without enabling Save or intercepting modified clicks', async () => {
    const onOpenProblem = vi.fn();
    render(SettingsSaveComposer, {
      ...base,
      invalidProblem: '.config/quality.yaml: Invalid YAML',
      problemHref: '/workspace/example/sync/files/.config/quality.yaml',
      problemLabel: '.config/quality.yaml',
      onOpenProblem,
    });
    const link = screen.getByRole('link', { name: 'Open .config/quality.yaml' });
    expect(link.getAttribute('href')).toBe('/workspace/example/sync/files/.config/quality.yaml');
    await fireEvent.click(link);
    expect(onOpenProblem).toHaveBeenCalledOnce();
    await fireEvent.click(link, { ctrlKey: true });
    expect(onOpenProblem).toHaveBeenCalledOnce();
    expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('keeps validation-only feedback visible without an ineffective dismissal', async () => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();
    render(SettingsSaveComposer, {
      ...base,
      count: 0,
      invalidProblem: 'Enter a duration in whole seconds',
      onDismiss,
    });

    expect(screen.queryByRole('button', { name: 'Dismiss' })).toBeNull();
    await vi.advanceTimersByTimeAsync(6_000);
    expect(screen.getByText('Enter a duration in whole seconds')).toBeTruthy();
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it('shows a saved receipt until it is dismissed', async () => {
    const onDismiss = vi.fn();
    render(SettingsSaveComposer, {
      ...base,
      count: 0,
      notice: 'Reconciliation found no repository changes',
      onDismiss,
    });

    expect(screen.getByText('Settings saved')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });
  it('expires a successful receipt after five seconds', async () => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();
    render(SettingsSaveComposer, {
      ...base,
      count: 0,
      notice: 'Reconciliation complete',
      onDismiss,
    });
    await vi.advanceTimersByTimeAsync(4_999);
    expect(onDismiss).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it('gives a fresh receipt a new timer and pauses while it is being read', async () => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();
    const props = { ...base, count: 0, notice: 'First save', onDismiss };
    const view = render(SettingsSaveComposer, props);
    await vi.advanceTimersByTimeAsync(4_000);
    await view.rerender({ ...props, notice: 'Second save' });
    await vi.advanceTimersByTimeAsync(4_000);
    expect(onDismiss).not.toHaveBeenCalled();
    const receipt = screen.getByRole('complementary', { name: 'Settings receipt' });
    await fireEvent.pointerEnter(receipt);
    await vi.advanceTimersByTimeAsync(6_000);
    expect(onDismiss).not.toHaveBeenCalled();
    await fireEvent.pointerLeave(receipt);
    await vi.advanceTimersByTimeAsync(5_000);
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it.each([
    { count: 1 },
    { problem: 'GitHub refused the change' },
    { invalidProblem: 'Duration is invalid' },
    { conflict: true },
    { saving: true },
    { resolving: true },
  ])('never expires work that replaces a receipt: %j', async (change) => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();
    const props = { ...base, count: 0, notice: 'Reconciliation complete', onDismiss };
    const view = render(SettingsSaveComposer, props);
    await vi.advanceTimersByTimeAsync(4_500);
    await view.rerender({ ...props, ...change });
    await vi.advanceTimersByTimeAsync(10_000);
    expect(onDismiss).not.toHaveBeenCalled();
    expect(screen.queryByText('Settings saved')).toBeNull();
    expect(
      screen
        .getByRole('complementary', { name: 'Settings draft' })
        .classList.contains('action-required'),
    ).toBe(true);
  });

  it('pauses expiry while a keyboard user has focus inside the receipt', async () => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();
    render(SettingsSaveComposer, {
      ...base,
      count: 0,
      notice: 'Reconciliation complete',
      onDismiss,
    });
    const receipt = screen.getByRole('complementary', { name: 'Settings receipt' });
    await fireEvent.focusIn(receipt);
    await vi.advanceTimersByTimeAsync(6_000);
    expect(onDismiss).not.toHaveBeenCalled();
    await fireEvent.focusOut(receipt, { relatedTarget: null });
    await vi.advanceTimersByTimeAsync(5_000);
    expect(onDismiss).toHaveBeenCalledOnce();
  });
});
