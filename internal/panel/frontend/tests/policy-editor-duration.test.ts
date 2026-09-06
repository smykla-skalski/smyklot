// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

import PolicyEditorDialog from '../src/lib/components/PolicyEditorDialog.svelte';

describe('job policy duration constraints [Component]', () => {
  it('accepts and submits the displayed zero cadence when Run job is turned off', async () => {
    document.body.innerHTML = '<main class="app-shell"></main>';
    const onSubmit = vi.fn();
    render(PolicyEditorDialog, {
      policy: {
        kind: 'catalog_refresh',
        enabled: true,
        cadence: 30_000_000_000,
        profile_id: 'always-open',
        default_priority: 'normal',
        retry_delay: 30_000_000_000,
        revision: 1,
        updated_at: '2026-09-06T10:00:00Z',
      },
      profiles: [
        {
          id: 'always-open',
          name: 'Always open',
          timezone: 'UTC',
          system: true,
          revision: 1,
          windows: [],
          exceptions: [],
        },
      ],
      busy: false,
      error: '',
      onClose: () => {},
      onSubmit,
    });
    const cadence = screen.getByRole('textbox', { name: 'How often' });
    await fireEvent.input(cadence, { target: { value: '0' } });
    expect(cadence.getAttribute('aria-invalid')).toBe('true');
    expect((screen.getByRole('button', { name: 'Save job' }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Run this job' }));
    expect(cadence.getAttribute('aria-invalid')).toBe('false');
    const save = screen.getByRole('button', { name: 'Save job' }) as HTMLButtonElement;
    expect(save.disabled).toBe(false);
    await fireEvent.click(save);
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false, cadence_seconds: 0 }),
    );
  });
});
