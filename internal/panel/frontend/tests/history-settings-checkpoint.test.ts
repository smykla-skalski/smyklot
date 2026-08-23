// @vitest-environment jsdom
import { QueryClient } from '@tanstack/svelte-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import HistoryPanelHarness from './support/HistoryPanelHarness.svelte';
import type {
  AuditEntry,
  InstallationSettingsBatchResponse,
  RootRuntimeSettings,
  SettingsCheckpoint,
  SettingsRestoreInput,
} from '../src/lib/types';

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

const genericCheckpoint: SettingsCheckpoint = {
  id: 'settings-1',
  action: 'installation.settings.saved',
  actor,
  created_at: '2026-08-23T08:00:00Z',
  affected_kinds: ['target'],
  items: [
    {
      kind: 'target',
      document_version: 1,
      after: {
        available: true,
        state: {
          document: {
            repository_default_enabled: true,
            pending_ci_mode_default: 'checks',
            config_patch: {},
          },
          digest: 'after',
          revision: 1,
        },
        differs: true,
        restorable: true,
      },
      before: { available: true, state: null, differs: false, restorable: true },
      current: null,
      changed: true,
    },
  ],
};

const rootCheckpoint: SettingsCheckpoint = {
  id: 'runtime-1',
  action: 'runtime.settings.saved',
  actor,
  created_at: '2026-08-23T09:00:00Z',
  affected_kinds: ['runtime'],
  items: [
    {
      kind: 'runtime',
      document_version: 1,
      before: {
        available: true,
        state: {
          document: {
            bot_config: null,
            log_level: 'info',
            poll_interval: null,
            pending_ci_quiet_period: 30_000_000_000,
            path_index_interval: null,
            session_ttl: null,
          },
          digest: 'runtime-before',
          revision: 2,
        },
        differs: false,
        restorable: true,
      },
      after: {
        available: true,
        state: {
          document: {
            bot_config: null,
            log_level: 'debug',
            poll_interval: null,
            pending_ci_quiet_period: 30_000_000_000,
            path_index_interval: null,
            session_ttl: null,
          },
          digest: 'runtime-after',
          revision: 3,
        },
        differs: true,
        restorable: true,
      },
      current: {
        document: {
          bot_config: null,
          log_level: 'info',
          poll_interval: null,
          pending_ci_quiet_period: 30_000_000_000,
          path_index_interval: null,
          session_ttl: null,
        },
        digest: 'runtime-current',
        revision: 7,
      },
      changed: true,
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
      created_at: genericCheckpoint.created_at,
    },
    {
      id: 'audit-repository-migration',
      target_id: 'target-from-entry',
      actor,
      action: 'repository.config_migration.reset',
      summary: 'Reset migrated repository configuration',
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
      name: 'Saved installation settings, inspect settings snapshot',
    });
    await fireEvent.click(settingsInspect);
    await screen.findByRole('dialog', { name: 'Settings history' });
    expect(fetchSettingsCheckpoint).toHaveBeenCalledWith('target-from-entry', 'settings-1');
    expect(screen.getByText('You can inspect this snapshot, but cannot restore it')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Restore selected' })).toBeNull();
    expect(screen.getByText('Reset migrated repository configuration')).toBeTruthy();
  });

  it('routes targetless Root history through runtime checkpoint inspection and restore', async () => {
    const fetchRootSettingsCheckpoint = vi.fn(async (checkpointId: string) => {
      void checkpointId;
      return rootCheckpoint;
    });
    const restored = {} as RootRuntimeSettings;
    const restoreRootSettingsCheckpoint = vi.fn(
      async (checkpointId: string, input: SettingsRestoreInput) => {
        void checkpointId;
        void input;
        return restored;
      },
    );
    const onRootSettingsRestored = vi.fn();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    render(HistoryPanelHarness, {
      props: {
        queryClient,
        context: 'root',
        rootRole: 'Super Root',
        targetId: 'root',
        section: 'audit',
        readOnly: false,
        fetchAudit: async () => ({
          items: [
            {
              id: 'runtime-audit-1',
              category: 'runtime',
              actor,
              action: 'runtime.settings.saved',
              summary: 'Saved runtime settings',
              settings_checkpoint_id: rootCheckpoint.id,
              created_at: rootCheckpoint.created_at,
            },
          ],
          next_cursor: null,
          total: 1,
        }),
        fetchFailures: async () => ({ items: [], next_cursor: null, total: 0 }),
        fetchRootSettingsCheckpoint,
        restoreRootSettingsCheckpoint,
        onRootSettingsRestored,
      },
    });

    await fireEvent.click(
      await screen.findByRole('button', {
        name: 'Saved runtime settings, inspect settings snapshot',
      }),
    );
    await screen.findByRole('dialog', { name: 'Settings history' });
    expect(fetchRootSettingsCheckpoint).toHaveBeenCalledWith('runtime-1');
    expect(screen.getByRole('checkbox', { name: 'Restore Runtime settings' })).toHaveProperty(
      'checked',
      true,
    );

    await fireEvent.click(screen.getByRole('button', { name: 'Restore selected' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Confirm restore' }));
    await waitFor(() => {
      expect(restoreRootSettingsCheckpoint).toHaveBeenCalledWith('runtime-1', {
        state: 'after',
        selections: [{ kind: 'runtime', expected_revision: 7 }],
      });
      expect(onRootSettingsRestored).toHaveBeenCalledWith(restored);
    });
  });

  it('restores a target checkpoint from global Root history', async () => {
    const fetchSettingsCheckpoint = vi.fn(async () => genericCheckpoint);
    const restored = { checkpoint_id: 'restored-1' } satisfies InstallationSettingsBatchResponse;
    const restoreSettingsCheckpoint = vi.fn(
      async (targetId: string, checkpointId: string, input: SettingsRestoreInput) => {
        void targetId;
        void checkpointId;
        void input;
        return restored;
      },
    );
    const onSettingsRestored = vi.fn();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    render(HistoryPanelHarness, {
      props: {
        queryClient,
        context: 'root',
        targetId: 'root',
        section: 'audit',
        readOnly: false,
        fetchAudit: async () => ({ items: auditEntries(), next_cursor: null, total: 2 }),
        fetchFailures: async () => ({ items: [], next_cursor: null, total: 0 }),
        fetchSettingsCheckpoint,
        restoreSettingsCheckpoint,
        hasUnsavedSettingsDraftsForTarget: () => false,
        onSettingsRestored,
      },
    });

    await fireEvent.click(
      await screen.findByRole('button', {
        name: 'Saved installation settings, inspect settings snapshot',
      }),
    );
    await screen.findByRole('dialog', { name: 'Settings history' });
    await fireEvent.click(screen.getByRole('button', { name: 'Restore selected' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Confirm restore' }));

    await waitFor(() => {
      expect(restoreSettingsCheckpoint).toHaveBeenCalledWith('target-from-entry', 'settings-1', {
        state: 'after',
        selections: [{ kind: 'target', expected_revision: 0 }],
      });
      expect(onSettingsRestored).toHaveBeenCalledWith(restored, 'target-from-entry');
    });
  });

  it('blocks global Root restore for the target that owns an unsaved draft', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    render(HistoryPanelHarness, {
      props: {
        queryClient,
        context: 'root',
        targetId: 'root',
        section: 'audit',
        readOnly: false,
        fetchAudit: async () => ({ items: auditEntries(), next_cursor: null, total: 2 }),
        fetchFailures: async () => ({ items: [], next_cursor: null, total: 0 }),
        fetchSettingsCheckpoint: async () => genericCheckpoint,
        restoreSettingsCheckpoint: async () => ({}),
        hasUnsavedSettingsDraftsForTarget: (targetId: string) => targetId === 'target-from-entry',
      },
    });

    await fireEvent.click(
      await screen.findByRole('button', {
        name: 'Saved installation settings, inspect settings snapshot',
      }),
    );
    expect(
      await screen.findByText('Save or discard unsaved settings before restoring history'),
    ).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Restore selected' })).toHaveProperty(
      'disabled',
      true,
    );
  });
});
