// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import SettingsCheckpointDialog from '../src/lib/components/SettingsCheckpointDialog.svelte';
import { settingsCheckpointSummary } from '../src/lib/settings-checkpoint-summary';
import type {
  InstallationSettingsCheckpoint,
  InstallationSettingsCheckpointItem,
  InstallationSettingsCheckpointState,
  InstallationSettingsRestoreInput,
} from '../src/lib/types';

const actor = {
  id: 'account-1',
  provider: 'github',
  subject_id: '1',
  login: 'bart',
  display_name: 'Bart Smykla',
  avatar_url: null,
};

function state(
  document: Record<string, unknown>,
  revision: number,
): InstallationSettingsCheckpointState {
  return { document, revision, digest: `digest-${revision}` };
}

function item(
  value: Partial<InstallationSettingsCheckpointItem> &
    Pick<InstallationSettingsCheckpointItem, 'kind' | 'before' | 'after' | 'current'>,
): InstallationSettingsCheckpointItem {
  return {
    document_version: 1,
    changed: true,
    differs: true,
    restorable: true,
    ...value,
  };
}

function checkpoint(): InstallationSettingsCheckpoint {
  return {
    id: 'checkpoint-1',
    action: 'installation.settings.saved',
    actor,
    created_at: '2026-08-23T08:00:00Z',
    affected_kinds: ['target', 'repository', 'sync_config', 'sync_override'],
    items: [
      item({
        kind: 'target',
        before: state(
          {
            repository_default_enabled: false,
            pending_ci_mode_default: 'checks',
            config_patch: {},
          },
          6,
        ),
        after: state(
          {
            repository_default_enabled: true,
            pending_ci_mode_default: 'labels',
            config_patch: { quiet_success: true },
          },
          7,
        ),
        current: state(
          {
            repository_default_enabled: false,
            pending_ci_mode_default: 'checks',
            config_patch: {},
          },
          11,
        ),
      }),
      item({
        kind: 'repository',
        repository_id: 'repo-1',
        repository_full_name: 'smykla-skalski/smyklot',
        before: null,
        after: state(
          { enabled_override: true, ignore_repository_file: false, config_patch: {} },
          2,
        ),
        current: state(
          { enabled_override: true, ignore_repository_file: false, config_patch: {} },
          2,
        ),
        differs: false,
      }),
      item({
        kind: 'sync_config',
        sync_kind: 'labels',
        before: null,
        after: state(
          {
            enabled: true,
            document: JSON.stringify({
              labels: [{ name: 'ci/green' }],
              allow_removal: true,
              excludes: [],
            }),
          },
          3,
        ),
        current: state({ enabled: false, document: '{}' }, 4),
        restorable: false,
        incompatibility: {
          code: 'document_version',
          reason: 'This stored document is no longer compatible.',
        },
      }),
      item({
        kind: 'sync_override',
        repository_id: 'repo-2',
        repository_full_name: 'smykla-skalski/website',
        sync_kind: 'files',
        before: null,
        after: state({ enabled: null, document: JSON.stringify({ files: [] }) }, 1),
        current: null,
      }),
    ],
  };
}

function mount(
  over: {
    readOnly?: boolean;
    hasUnsavedDrafts?: boolean;
    restore?: (input: InstallationSettingsRestoreInput) => Promise<{ checkpoint_id: string }>;
  } = {},
) {
  const restore = vi.fn(
    async (_targetId: string, _checkpointId: string, input: InstallationSettingsRestoreInput) =>
      over.restore?.(input) ?? { checkpoint_id: 'checkpoint-2' },
  );
  const onRestored = vi.fn();
  const onClose = vi.fn();
  render(SettingsCheckpointDialog, {
    target: document.querySelector('.app-shell') as HTMLElement,
    props: {
      open: true,
      targetId: 'target-1',
      checkpointId: 'checkpoint-1',
      readOnly: over.readOnly ?? false,
      hasUnsavedDrafts: over.hasUnsavedDrafts ?? false,
      returnFocus: null,
      fetchCheckpoint: async () => checkpoint(),
      restoreCheckpoint: restore,
      onRestored,
      onClose,
    },
  });
  return { restore, onRestored, onClose };
}

describe('SettingsCheckpointDialog [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
  });

  it('summarises the four fixed resource kinds without a generic diff', () => {
    const [target, repository, syncConfig, syncOverride] = checkpoint().items;
    expect(settingsCheckpointSummary(target!, target!.after)).toBe(
      'Repositories on by default · Pending CI labels · 1 policy override',
    );
    expect(settingsCheckpointSummary(repository!, repository!.after)).toBe(
      'Enabled · Repository file read · 0 policy overrides',
    );
    expect(settingsCheckpointSummary(syncConfig!, syncConfig!.after)).toBe(
      'On · 1 label · removal allowed · 0 exclusions',
    );
    expect(settingsCheckpointSummary(syncOverride!, syncOverride!.after)).toBe(
      'Inherits enablement · 1 stored field',
    );
  });

  it('preselects only differing restorable resources and explains incompatibility', async () => {
    mount();

    await screen.findByText('Saved by Bart Smykla');
    expect(screen.getByRole('checkbox', { name: 'Restore Installation defaults' })).toHaveProperty(
      'checked',
      true,
    );
    expect(screen.getByRole('checkbox', { name: 'Restore smykla-skalski/smyklot' })).toHaveProperty(
      'checked',
      false,
    );
    expect(screen.getByRole('checkbox', { name: 'Restore Labels Sync' })).toHaveProperty(
      'disabled',
      true,
    );
    expect(screen.getByText('This stored document is no longer compatible')).toBeTruthy();

    await fireEvent.click(screen.getAllByText('View stored state')[0] as HTMLElement);
    expect(screen.getAllByText(/"before":/)[0]?.textContent).toContain('"current"');

    await fireEvent.click(screen.getByRole('checkbox', { name: 'Restore Installation defaults' }));
    await fireEvent.click(
      screen.getByRole('checkbox', {
        name: 'Restore smykla-skalski/website · Files override',
      }),
    );
    expect(screen.getByText('Select at least one differing resource to restore')).toBeTruthy();
  });

  it('restores selected resources once and only after confirmation', async () => {
    const { restore, onRestored, onClose } = mount();
    await screen.findByText('Saved by Bart Smykla');

    await fireEvent.click(screen.getByRole('button', { name: 'Restore selected' }));
    expect(restore).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toContain('creates new active revisions');

    await fireEvent.click(screen.getByRole('button', { name: 'Confirm restore' }));
    await waitFor(() => expect(restore).toHaveBeenCalledOnce());
    expect(restore.mock.calls[0]?.[2]).toEqual({
      selections: [
        { kind: 'target', expected_revision: 11 },
        {
          kind: 'sync_override',
          repository_id: 'repo-2',
          sync_kind: 'files',
          expected_revision: 0,
        },
      ],
    });
    expect(onRestored).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('keeps inspection available while drafts block restore', async () => {
    mount({ hasUnsavedDrafts: true });
    await screen.findByText('Saved by Bart Smykla');

    expect(
      screen.getByText(
        'Save or discard this installation’s unsaved settings before restoring history',
      ),
    ).toBeTruthy();
    expect(
      screen.getByText('Repositories on by default · Pending CI labels · 1 policy override'),
    ).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Restore selected' })).toHaveProperty(
      'disabled',
      true,
    );
  });

  it('is inspection-only for read-only viewers', async () => {
    mount({ readOnly: true });
    await screen.findByText('Saved by Bart Smykla');

    expect(screen.getByText('You can inspect this snapshot, but cannot restore it')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Restore selected' })).toBeNull();
  });
});
