// @vitest-environment jsdom
import { QueryClient } from '@tanstack/svelte-query';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import HistoryPanelHarness from './support/HistoryPanelHarness.svelte';
import type { AuditEntry, InstallationSettingsCheckpoint } from '../src/lib/types';

class TestResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

const actor = {
  id: 'account-1',
  provider: 'github',
  subject_id: '1',
  login: 'bart',
  display_name: 'Bart Smykla',
  avatar_url: null,
};

const genericCheckpoint: InstallationSettingsCheckpoint = {
  id: 'settings-1',
  action: 'installation.settings.saved',
  actor,
  created_at: '2026-08-23T08:00:00Z',
  affected_kinds: ['target'],
  items: [
    {
      kind: 'target',
      document_version: 1,
      before: null,
      after: {
        document: {
          repository_default_enabled: true,
          pending_ci_mode_default: 'checks',
          config_patch: {},
        },
        digest: 'after',
        revision: 1,
      },
      current: null,
      changed: true,
      differs: true,
      restorable: true,
    },
  ],
};

function auditEntries(): AuditEntry[] {
  return [
    {
      id: 'audit-generic',
      target_id: 'target-from-entry',
      actor,
      action: 'installation.settings.saved',
      summary: 'Saved installation settings',
      settings_checkpoint_id: genericCheckpoint.id,
      sync_config_checkpoint_id: 'superseded-legacy-link',
      created_at: genericCheckpoint.created_at,
    },
    {
      id: 'audit-legacy',
      target_id: 'legacy-target',
      actor,
      action: 'sync.config.saved',
      summary: 'Older Sync configuration event',
      sync_config_checkpoint_id: 'sync-1',
      created_at: '2026-08-22T08:00:00Z',
    },
  ];
}

function mount() {
  const fetchSettingsCheckpoint = vi.fn(async () => genericCheckpoint);
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(HistoryPanelHarness, {
    props: {
      queryClient,
      targetId: 'fallback-target',
      fetchAudit: async () => ({ items: auditEntries(), next_cursor: null, total: 2 }),
      fetchFailures: async () => ({ items: [], next_cursor: null, total: 0 }),
      fetchSettingsCheckpoint,
    },
  });
  return { fetchSettingsCheckpoint };
}

describe('HistoryPanel settings checkpoints [Component]', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  it('uses generic settings checkpoints as the only inspection path', async () => {
    const { fetchSettingsCheckpoint } = mount();
    const settingsInspect = await screen.findByRole('button', {
      name: 'Saved installation settings. Inspect settings snapshot',
    });
    expect(
      screen.queryByRole('button', {
        name: 'Saved installation settings. Inspect Sync configuration snapshot',
      }),
    ).toBeNull();

    await fireEvent.click(settingsInspect);
    await screen.findByRole('dialog', { name: 'Settings history' });
    expect(fetchSettingsCheckpoint).toHaveBeenCalledWith('target-from-entry', 'settings-1');
    expect(screen.getByText('You can inspect this snapshot, but cannot restore it')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Restore selected' })).toBeNull();
    expect(screen.getByText('Older Sync configuration event')).toBeTruthy();
    expect(
      screen.queryByRole('button', {
        name: 'Older Sync configuration event. Inspect Sync configuration snapshot',
      }),
    ).toBeNull();
  });
});
