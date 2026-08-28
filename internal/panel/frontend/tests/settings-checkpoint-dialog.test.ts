// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SettingsCheckpointDialog from '../src/lib/components/SettingsCheckpointDialog.svelte';
import { settingsCheckpointSummary } from '../src/lib/settings-checkpoint-summary';
import type {
  SettingsCheckpoint,
  SettingsCheckpointItem,
  SettingsCheckpointSide,
  SettingsCheckpointState,
  SettingsRestoreInput,
} from '../src/lib/types';

const actor = {
  id: 'account-1',
  provider: 'github',
  subject_id: '1',
  login: 'bart',
  display_name: 'Bart Smykla',
  avatar_url: null,
};

class TestResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

function state(document: Record<string, unknown>, revision: number): SettingsCheckpointState {
  return { document, revision, digest: `digest-${revision}` };
}

function side(
  value: SettingsCheckpointState | null,
  differs: boolean,
  restorable = true,
  incompatibility?: SettingsCheckpointSide['incompatibility'],
): SettingsCheckpointSide {
  return {
    available: true,
    state: value,
    differs,
    restorable,
    ...(incompatibility === undefined ? {} : { incompatibility }),
  };
}

function unavailableSide(): SettingsCheckpointSide {
  return { available: false, state: null, differs: false, restorable: false };
}

function item(
  value: Partial<SettingsCheckpointItem> &
    Pick<SettingsCheckpointItem, 'kind' | 'before' | 'after' | 'current'>,
): SettingsCheckpointItem {
  return {
    document_version: 1,
    changed: true,
    ...value,
  };
}

function checkpoint(): SettingsCheckpoint {
  return {
    id: 'checkpoint-1',
    action: 'installation.settings.saved',
    actor,
    created_at: '2026-08-23T08:00:00Z',
    affected_kinds: ['target', 'repository', 'sync_config', 'sync_override'],
    items: [
      item({
        kind: 'target',
        before: side(
          state(
            {
              repository_default_enabled: false,
              pending_ci_mode_default: 'checks',
              config_patch: {},
            },
            6,
          ),
          false,
        ),
        after: side(
          state(
            {
              repository_default_enabled: true,
              pending_ci_mode_default: 'labels',
              config_patch: { quiet_success: true },
            },
            7,
          ),
          true,
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
        before: side(null, true),
        after: side(
          state({ enabled_override: true, ignore_repository_file: false, config_patch: {} }, 2),
          false,
        ),
        current: state(
          { enabled_override: true, ignore_repository_file: false, config_patch: {} },
          2,
        ),
      }),
      item({
        kind: 'sync_config',
        sync_kind: 'labels',
        before: side(null, true),
        after: side(
          state(
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
          true,
          false,
          {
            code: 'document_version',
            reason: 'This stored document is no longer compatible',
          },
        ),
        current: state({ enabled: false, document: '{}' }, 4),
      }),
      item({
        kind: 'sync_override',
        repository_id: 'repo-2',
        repository_full_name: 'smykla-skalski/website',
        sync_kind: 'files',
        before: side(null, false),
        after: side(state({ enabled: null, document: JSON.stringify({ files: [] }) }, 1), true),
        current: null,
      }),
    ],
  };
}

function mount(
  over: {
    readOnly?: boolean;
    hasUnsavedDrafts?: boolean;
    checkpoint?: SettingsCheckpoint;
    restore?: (input: SettingsRestoreInput) => Promise<{ checkpoint_id: string }>;
  } = {},
) {
  const restore = vi.fn(async (_checkpointId: string, input: SettingsRestoreInput) => {
    await over.restore?.(input);
  });
  const onClose = vi.fn();
  render(SettingsCheckpointDialog, {
    target: document.querySelector('.app-shell') as HTMLElement,
    props: {
      open: true,
      identity: 'target-1',
      checkpointId: 'checkpoint-1',
      readOnly: over.readOnly ?? false,
      hasUnsavedDrafts: over.hasUnsavedDrafts ?? false,
      returnFocus: null,
      fetchCheckpoint: async () => over.checkpoint ?? checkpoint(),
      restoreCheckpoint: restore,
      onClose,
    },
  });
  return { restore, onClose };
}

describe('SettingsCheckpointDialog [Component]', () => {
  beforeEach(() => {
    document.body.innerHTML = '<main class="app-shell"></main>';
    vi.stubGlobal('ResizeObserver', TestResizeObserver);
  });

  afterEach(() => vi.unstubAllGlobals());

  it('summarises the five supported resource kinds without a generic diff', () => {
    const [target, repository, syncConfig, syncOverride] = checkpoint().items;
    const runtime = item({
      kind: 'runtime',
      before: side(null, false),
      after: side(
        state(
          {
            bot_config: null,
            log_level: 'debug',
            poll_interval: null,
            pending_ci_quiet_period: 30_000_000_000,
            path_index_interval: null,
            session_ttl: null,
          },
          4,
        ),
        true,
      ),
      current: null,
    });
    expect(settingsCheckpointSummary(target!, target!.after.state)).toBe(
      'Repositories on by default · Pending CI labels · 1 policy override',
    );
    expect(settingsCheckpointSummary(repository!, repository!.after.state)).toBe(
      'Enabled · Repository file read · 0 policy overrides',
    );
    expect(settingsCheckpointSummary(syncConfig!, syncConfig!.after.state)).toBe(
      'On · 1 label · removal allowed · 0 exclusions',
    );
    expect(settingsCheckpointSummary(syncOverride!, syncOverride!.after.state)).toBe(
      'Inherits enablement · 1 stored field',
    );
    expect(settingsCheckpointSummary(runtime, runtime.after.state)).toBe(
      '2 overrides · Current deployment fills the rest',
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

    const rawToggle = screen.getByRole('button', {
      name: 'View stored state for Installation defaults',
    });
    await fireEvent.click(rawToggle);
    expect(screen.getAllByText(/"before":/)[0]?.textContent).toContain('"current"');
    expect(rawToggle.getAttribute('aria-controls')).toBe('settings-checkpoint-raw-target::');

    await fireEvent.click(screen.getByRole('checkbox', { name: 'Restore Installation defaults' }));
    await fireEvent.click(
      screen.getByRole('checkbox', {
        name: 'Restore smykla-skalski/website · Files override',
      }),
    );
    expect(screen.getByText('Select at least one differing resource to restore')).toBeTruthy();
  });

  it('restores selected resources once and only after confirmation', async () => {
    const { restore, onClose } = mount();
    await screen.findByText('Saved by Bart Smykla');

    await fireEvent.click(screen.getByRole('button', { name: 'Restore selected' }));
    expect(restore).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toContain('creates new active revisions');

    await fireEvent.click(screen.getByRole('button', { name: 'Confirm restore' }));
    await waitFor(() => expect(restore).toHaveBeenCalledOnce());
    expect(restore.mock.calls[0]?.[1]).toEqual({
      state: 'after',
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
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('recalculates selections and serializes a Before restore', async () => {
    const { restore } = mount();
    await screen.findByText('Saved by Bart Smykla');

    expect(screen.getByRole('checkbox', { name: 'Restore Installation defaults' })).toHaveProperty(
      'checked',
      true,
    );
    expect(screen.getByRole('checkbox', { name: 'Restore smykla-skalski/smyklot' })).toHaveProperty(
      'checked',
      false,
    );

    await fireEvent.click(screen.getByRole('radio', { name: 'Before change' }));
    expect(screen.getByRole('checkbox', { name: 'Restore Installation defaults' })).toHaveProperty(
      'checked',
      false,
    );
    expect(screen.getByRole('checkbox', { name: 'Restore smykla-skalski/smyklot' })).toHaveProperty(
      'checked',
      true,
    );
    expect(screen.getByRole('checkbox', { name: 'Restore Labels Sync' })).toHaveProperty(
      'checked',
      true,
    );
    expect(screen.queryByText('This stored document is no longer compatible')).toBeNull();

    await fireEvent.click(screen.getByRole('button', { name: 'Restore selected' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Confirm restore' }));
    await waitFor(() => expect(restore).toHaveBeenCalledOnce());
    expect(restore.mock.calls[0]?.[1]).toEqual({
      state: 'before',
      selections: [
        { kind: 'repository', repository_id: 'repo-1', expected_revision: 2 },
        { kind: 'sync_config', sync_kind: 'labels', expected_revision: 4 },
      ],
    });
  });

  it('does not offer an unavailable Before side for a baseline', async () => {
    const baseline: SettingsCheckpoint = {
      id: 'baseline-1',
      action: 'installation.settings.baseline',
      actor,
      created_at: '2026-08-23T07:00:00Z',
      affected_kinds: ['target'],
      items: [
        item({
          kind: 'target',
          before: unavailableSide(),
          after: side(
            state(
              {
                repository_default_enabled: false,
                pending_ci_mode_default: 'checks',
                config_patch: {},
              },
              1,
            ),
            true,
          ),
          current: state(
            {
              repository_default_enabled: true,
              pending_ci_mode_default: 'checks',
              config_patch: {},
            },
            3,
          ),
        }),
      ],
    };
    const { restore } = mount({ checkpoint: baseline });
    await screen.findByText('Initial snapshot by Bart Smykla');

    expect(screen.queryByRole('radio', { name: 'Before change' })).toBeNull();
    expect(screen.getByRole('radio', { name: 'Initial state' })).toHaveProperty('checked', true);
    expect(screen.getByText('Restore the settings captured when history began')).toBeTruthy();
    expect(screen.getByText('Not captured')).toBeTruthy();

    await fireEvent.click(screen.getByRole('button', { name: 'Restore selected' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Confirm restore' }));
    await waitFor(() => expect(restore).toHaveBeenCalledOnce());
    expect(restore.mock.calls[0]?.[1]).toEqual({
      state: 'after',
      selections: [{ kind: 'target', expected_revision: 3 }],
    });
  });

  it('keeps inspection available while drafts block restore', async () => {
    mount({ hasUnsavedDrafts: true });
    await screen.findByText('Saved by Bart Smykla');

    expect(
      screen.getByText('Save or discard unsaved settings before restoring history'),
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
