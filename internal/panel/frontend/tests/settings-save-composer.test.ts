// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

import SettingsSaveComposer from '../src/lib/components/SettingsSaveComposer.svelte';

const base = {
  count: 2,
  onSave: vi.fn(),
  onDiscard: vi.fn(),
  onResolveConflict: vi.fn(),
  onDismiss: vi.fn(),
};

describe('SettingsSaveComposer [Component]', () => {
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
});
