import TargetSettingsHarness from './support/TargetSettingsHarness.svelte';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import { TARGET } from '../stories/support/fixtures';
// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import ConfigurationFileSync from '../src/lib/components/ConfigurationFileSync.svelte';
import type { ConfigFileSyncStatus } from '../src/lib/config-file-sync';

const off: ConfigFileSyncStatus = { enabled: false, available: true, status: 'off' };
const ready: ConfigFileSyncStatus = {
  enabled: true,
  available: true,
  status: 'ready',
  last_check: {
    checked_at: '2026-09-07T12:00:00Z',
    settings_current: true,
    status: 'ready',
    path: '.smyklot.toml',
  },
};
const props = (data = off) => ({
  scope: 'repository' as const,
  repository: 'acme/widget',
  enabled: false,
  savedEnabled: false,
  now: Date.parse('2026-09-07T12:05:00Z'),
  connection: { data, isPending: false, isFetching: false, error: null, refetch: vi.fn() },
  onChange: vi.fn(),
});

describe('configuration-file sync card [Component]', () => {
  it('stages only its own opt-in and keeps the saved connection off until saved', async () => {
    const input = props();
    const result = render(ConfigurationFileSync, input);
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Sync settings with file' }));
    expect(input.onChange).toHaveBeenCalledWith(true);
    expect(input.connection.refetch).not.toHaveBeenCalled();
    await result.rerender({ ...input, enabled: true, dirty: true });
    expect(screen.getByText('Starts after you save')).toBeTruthy();
    expect(within(screen.getByLabelText('Saved connection status')).getByText('Off')).toBeTruthy();
    expect(document.querySelectorAll('[data-unsaved]')).toHaveLength(1);
    await result.rerender({ ...input, enabled: false, dirty: false });
    expect(document.querySelectorAll('[data-unsaved]')).toHaveLength(0);
    expect(screen.queryByText('Starts after you save')).toBeNull();
  });
  it('keeps a queued off draft separate from a currently working connection', () => {
    render(ConfigurationFileSync, { ...props(ready), savedEnabled: true, dirty: true });
    expect(screen.getByText('Stops after you save')).toBeTruthy();
    expect(screen.getByText('In sync')).toBeTruthy();
  });
  it('does not change file-use when the connection opt-in changes', async () => {
    const input = { ...props(), fileIgnored: true, savedFileIgnored: true, enabled: true };
    render(ConfigurationFileSync, input);
    expect(
      screen.getByText('Use file settings must also be on to allow imports and pull requests'),
    ).toBeTruthy();
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Sync settings with file' }));
    expect(input.onChange).toHaveBeenCalledExactlyOnceWith(false);
  });
  it('shows the workspace file as a distinct scope', () => {
    render(ConfigurationFileSync, { ...props(), scope: 'workspace', repository: 'acme/.github' });
    expect(screen.getByText('.smyklot/workspace.toml')).toBeTruthy();
    expect(screen.getByText(/separate from that repository/)).toBeTruthy();
  });
  it('allows read-only status recovery without enabling settings changes', async () => {
    const input = props();
    render(ConfigurationFileSync, {
      ...input,
      readOnly: true,
      connection: { ...input.connection, error: new Error('Access unavailable') },
    });
    expect((screen.getByRole('checkbox') as HTMLInputElement).disabled).toBe(true);
    expect(screen.getByText('Read only')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(input.connection.refetch).toHaveBeenCalledOnce();
    expect(input.onChange).not.toHaveBeenCalled();
  });
  it('has no fake conflict-resolution action and retains the safe outstanding PR link', () => {
    render(ConfigurationFileSync, {
      ...props({
        ...ready,
        status: 'blocked',
        last_check: {
          ...ready.last_check!,
          status: 'blocked',
          problem: 'proposal_outstanding',
          proposal: { number: 42, url: 'https://github.com/acme/widget/pull/42' },
        },
      }),
      enabled: true,
      savedEnabled: true,
    });
    expect(screen.getByRole('link', { name: '#42' }).getAttribute('href')).toBe(
      'https://github.com/acme/widget/pull/42',
    );
    expect(screen.queryByRole('button', { name: /resolve|review conflicts/i })).toBeNull();
  });
});

it('ages the workspace observation while its settings page remains mounted', async () => {
  vi.useFakeTimers();
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      disconnect() {}
    },
  );
  vi.setSystemTime(new Date('2026-09-07T12:05:00Z'));
  document.body.innerHTML = '<main class="app-shell"></main>';
  const drafts = new SettingsDraftRegistry({ storage: null });
  drafts.hydrate('viewer');
  let result: ReturnType<typeof render> | undefined;
  try {
    result = render(TargetSettingsHarness, {
      props: {
        drafts,
        target: { ...TARGET, config_file_sync_enabled: true },
        configFileConnection: props(ready).connection,
      },
    });
    const region = within(screen.getByRole('region', { name: 'Configuration file sync' }));
    expect(region.getByText('5 minutes ago')).toBeTruthy();
    await vi.advanceTimersByTimeAsync(60 * 60_000);
    expect(region.getByText('1 hour ago')).toBeTruthy();
  } finally {
    result?.unmount();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  }
});
