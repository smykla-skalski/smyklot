// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';

import PolicyEditorDialog from '../src/lib/components/PolicyEditorDialog.svelte';

afterEach(cleanup);

describe('job policy duration constraints [Component]', () => {
  it.each(['catalog_refresh', 'config_file_sync'] as const)(
    'requires a positive %s cadence until Run job is turned off',
    async (kind) => {
      document.body.innerHTML = '<main class="app-shell"></main>';
      const onSubmit = vi.fn();
      render(PolicyEditorDialog, {
        policy: {
          kind,
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
    },
  );

  it('retains configuration file sync defaults and submits changed minutes as seconds', async () => {
    document.body.innerHTML = '<main class="app-shell"></main>';
    const onSubmit = vi.fn();
    render(PolicyEditorDialog, {
      policy: {
        kind: 'config_file_sync',
        enabled: true,
        cadence: 900_000_000_000,
        profile_id: 'always-open',
        default_priority: 'normal',
        retry_delay: 30_000_000_000,
        retention: 172_800_000_000_000,
        revision: 7,
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
    expect(screen.getByText('Configuration file sync')).toBeTruthy();
    expect(screen.getByRole('group', { name: 'How often' }).getAttribute('aria-describedby')).toBe(
      'policy-cadence-help',
    );
    expect(screen.getByText(/Checks connected configuration files for changes/)).toBeTruthy();
    const cadence = screen.getByRole('textbox', { name: 'How often' }) as HTMLInputElement;
    expect(cadence.value).toBe('15');
    expect(screen.getByRole('combobox', { name: 'How often unit' }).textContent).toContain(
      'minutes',
    );
    await fireEvent.input(cadence, { target: { value: '20' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save job' }));
    expect(onSubmit).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({
        enabled: true,
        cadence_seconds: 1200,
        retry_delay_seconds: 30,
        retention_seconds: 172800,
        default_priority: 'normal',
        expected_revision: 7,
      }),
    );
    expect(onSubmit.mock.calls[0]?.[0].approval_lifetime_seconds).toBeUndefined();
  });
  it('keeps an empty or fractional webhook attempt limit unsavable', async () => {
    document.body.innerHTML = '<main class="app-shell"></main>';
    const onSubmit = vi.fn();
    render(PolicyEditorDialog, {
      policy: {
        kind: 'webhook_delivery',
        enabled: true,
        cadence: 0,
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
    const input = screen.getByRole('spinbutton', { name: 'Attempts before giving up' });
    const save = screen.getByRole('button', { name: 'Save job' }) as HTMLButtonElement;
    for (const value of ['', '1.5']) {
      await fireEvent.input(input, { target: { value } });
      expect(save.disabled).toBe(true);
    }
    await fireEvent.input(input, { target: { value: '8' } });
    expect(save.disabled).toBe(false);
    await fireEvent.click(save);
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ configuration: expect.objectContaining({ max_attempts: 8 }) }),
    );
  });
});
