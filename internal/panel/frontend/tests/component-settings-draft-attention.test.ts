// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

import SettingsDraftAttention from '../src/lib/components/SettingsDraftAttention.svelte';

describe('SettingsDraftAttention [Component]', () => {
  it('announces drafts restored from an earlier session and can be dismissed', async () => {
    const onDismiss = vi.fn();
    render(SettingsDraftAttention, { kind: 'restored', count: 3, onDismiss });

    const notice = screen.getByRole('status');
    expect(notice.textContent).toContain('Unsaved settings restored');
    expect(notice.textContent).toContain('3 unsaved settings from an earlier session');

    await fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));

    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it('explains the thirty-minute hidden-tab threshold', () => {
    render(SettingsDraftAttention, {
      kind: 'inactive',
      count: 1,
      onDismiss: vi.fn(),
    });

    const notice = screen.getByRole('status');
    expect(notice.textContent).toContain('Unsaved settings need attention');
    expect(notice.textContent).toContain('out of view for at least 30 minutes');
    expect(notice.textContent).toContain('1 unsaved setting is still here and not saved');
  });

  it('uses alert semantics for a storage failure', () => {
    render(SettingsDraftAttention, {
      kind: 'storage-problem',
      problem: 'Browser storage refused this update.',
      onDismiss: vi.fn(),
    });

    const notice = screen.getByRole('alert');
    expect(notice.textContent).toContain('Unsaved settings may not survive');
    expect(notice.textContent).toContain('Browser storage refused this update.');
    expect(notice.classList.contains('callout-warning')).toBe(true);
  });
});
