// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

import SettingsDraftAttention from '../src/lib/components/SettingsDraftAttention.svelte';

describe('SettingsDraftAttention [Component]', () => {
  it('announces drafts a tab was left holding and can be dismissed', async () => {
    const onDismiss = vi.fn();
    render(SettingsDraftAttention, {
      kind: 'inactive',
      count: 3,
      reviewHref: '/workspace/smykla-skalski/repositories',
      onDismiss,
    });

    const notice = screen.getByRole('status');
    expect(notice.textContent).toContain('Unsaved settings need attention');
    expect(notice.textContent).toContain('3 unsaved settings are still here and not saved');
    const review = screen.getByRole('link', { name: 'Review' });
    expect(review.getAttribute('href')).toBe('/workspace/smykla-skalski/repositories');
    review.addEventListener('click', (event) => event.preventDefault());
    await fireEvent.click(review);
    expect(onDismiss).toHaveBeenCalledOnce();

    await fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));

    expect(onDismiss).toHaveBeenCalledTimes(2);
  });

  it('explains the thirty-minute hidden-tab threshold', () => {
    render(SettingsDraftAttention, {
      kind: 'inactive',
      count: 1,
      reviewHref: '/workspace/smykla-skalski/sync/settings',
      onDismiss: vi.fn(),
    });

    const notice = screen.getByRole('status');
    expect(notice.textContent).toContain('Unsaved settings need attention');
    expect(notice.textContent).toContain('out of view for at least 30 minutes');
    expect(notice.textContent).toContain('1 unsaved setting is still here and not saved');
    expect(screen.getByRole('link', { name: 'Review' }).getAttribute('href')).toBe(
      '/workspace/smykla-skalski/sync/settings',
    );
  });

  it('uses alert semantics and hides Review for a storage failure', () => {
    render(SettingsDraftAttention, {
      kind: 'storage-problem',
      problem: 'Browser storage refused this update',
      reviewHref: '/workspace/smykla-skalski/repositories',
      onDismiss: vi.fn(),
    });

    const notice = screen.getByRole('alert');
    expect(notice.textContent).toContain('Unsaved settings may not survive');
    expect(notice.textContent).toContain('Browser storage refused this update');
    expect(notice.classList.contains('callout-warning')).toBe(true);
    expect(screen.queryByRole('link', { name: 'Review' })).toBeNull();
  });
});
