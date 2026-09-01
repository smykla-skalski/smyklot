import { describe, expect, it, vi } from 'vitest';

import { PanelApiError } from '../src/lib/api';
import { rebaseWorkspaceConflicts, saveWorkspaceDrafts } from '../src/lib/workspace-settings-save';
import {
  adoptRepositorySettings,
  repositorySettingsDraftDocument,
  stageRepositorySettingsControl,
} from '../src/lib/repository-settings';
import {
  adoptSyncOverrideSettings,
  buildSyncOverrideEditorEnvelope,
  stageSyncOverrideControl,
} from '../src/lib/repository-sync-override-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import {
  adoptSyncConfigSettings,
  buildSyncConfigEditorEnvelope,
  stageSyncConfigControl,
  syncConfigDraftEnvelope,
} from '../src/lib/sync-config-settings';
import {
  adoptTargetDefaults,
  buildTargetDefaultsDocument,
  stageTargetDefaultsControl,
  targetDefaultsDraftDocument,
  targetDefaultsResource,
} from '../src/lib/target-defaults-settings';
import type {
  WorkspaceSettingsBatchInput,
  WorkspaceSettingsBatchResponse,
  SyncOverride,
} from '../src/lib/types';
import { emptySyncConfig, REPOSITORY_DETAIL, TARGET } from '../stories/support/fixtures';

function registry(): SettingsDraftRegistry {
  const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
  drafts.hydrate('viewer-1');
  return drafts;
}

function emptyOverride(): SyncOverride {
  return { kind: 'files', enabled: null, document: {}, revision: 0, unreadable: false };
}

describe('workspace settings save coordinator [Unit]', () => {
  it('sends target, repository, and override drafts in one request and commits together', async () => {
    const drafts = registry();
    const targetId = TARGET.id;
    const repositoryId = REPOSITORY_DETAIL.repository.id;
    const override = emptyOverride();
    const syncConfig = emptySyncConfig('settings');

    adoptTargetDefaults(drafts, TARGET);
    const targetDocument = {
      ...targetDefaultsDraftDocument(drafts, TARGET),
      repository_default_enabled: !TARGET.repository_default_enabled,
    };
    stageTargetDefaultsControl(
      drafts,
      TARGET,
      targetDocument,
      'defaults.repository_default_enabled',
    );

    adoptRepositorySettings(drafts, targetId, REPOSITORY_DETAIL);
    const repositoryDocument = {
      ...repositorySettingsDraftDocument(drafts, targetId, REPOSITORY_DETAIL),
      enabled_override: false,
    };
    stageRepositorySettingsControl(
      drafts,
      targetId,
      REPOSITORY_DETAIL,
      repositoryDocument,
      `repositories.${repositoryId}.enabled_override`,
    );

    adoptSyncOverrideSettings(drafts, targetId, repositoryId, override);
    stageSyncOverrideControl(
      drafts,
      targetId,
      repositoryId,
      override,
      { ...buildSyncOverrideEditorEnvelope(override), enabled: false },
      `repositories.${repositoryId}.sync.files.enabled`,
    );

    adoptSyncConfigSettings(drafts, targetId, syncConfig);
    stageSyncConfigControl(
      drafts,
      targetId,
      syncConfig,
      { ...buildSyncConfigEditorEnvelope(syncConfig), enabled: true },
      'sync.settings.enabled',
    );

    const save = vi.fn(
      async (
        sentTargetId: string,
        input: WorkspaceSettingsBatchInput,
      ): Promise<WorkspaceSettingsBatchResponse> => {
        expect(sentTargetId).toBe(targetId);
        expect(input.target?.expected_revision).toBe(TARGET.revision);
        expect(input.repositories).toHaveLength(1);
        expect(input.sync_configs).toHaveLength(1);
        expect(input.sync_overrides).toHaveLength(1);
        const { expected_revision: _targetRevision, ...savedTarget } = input.target!;
        const { expected_revision: _repositoryRevision, ...savedRepository } =
          input.repositories![0]!;
        const { expected_revision: _overrideRevision, ...savedOverride } =
          input.sync_overrides![0]!;
        const { expected_revision: _syncRevision, ...savedSyncConfig } = input.sync_configs![0]!;
        void _targetRevision;
        void _repositoryRevision;
        void _overrideRevision;
        void _syncRevision;
        return {
          checkpoint_id: '91',
          target: { target_id: targetId, ...savedTarget, revision: TARGET.revision + 1 },
          repositories: [{ ...savedRepository, revision: REPOSITORY_DETAIL.revision + 1 }],
          sync_configs: [
            {
              target_id: targetId,
              kind: savedSyncConfig.kind,
              enabled: savedSyncConfig.enabled,
              document:
                savedSyncConfig.kind === 'labels'
                  ? {
                      labels: savedSyncConfig.labels,
                      allow_removal: savedSyncConfig.allow_removal,
                      excludes: savedSyncConfig.excludes,
                    }
                  : savedSyncConfig.document,
              revision: syncConfig.revision + 1,
            },
          ],
          sync_overrides: [
            { target_id: targetId, ...savedOverride, revision: override.revision + 1 },
          ],
        };
      },
    );

    await expect(saveWorkspaceDrafts(drafts, targetId, save)).resolves.toEqual({
      saved: true,
      checkpointId: '91',
    });
    expect(save).toHaveBeenCalledOnce();
    expect(drafts.hasDirty({ type: 'workspace', targetId })).toBe(false);
    expect(drafts.operation({ type: 'workspace', targetId }).notice).toContain(
      'Reconciliation creates a plan only when repositories need changes',
    );
  });

  it('blocks malformed persisted editor text before any request', async () => {
    const drafts = registry();
    const targetId = TARGET.id;
    const repositoryId = REPOSITORY_DETAIL.repository.id;
    adoptTargetDefaults(drafts, TARGET);
    stageTargetDefaultsControl(
      drafts,
      TARGET,
      {
        ...targetDefaultsDraftDocument(drafts, TARGET),
        repository_default_enabled: !TARGET.repository_default_enabled,
      },
      'defaults.repository_default_enabled',
    );
    const override: SyncOverride = {
      ...emptyOverride(),
      document: { merges: [{ path: 'renovate.json', overrides: { timezone: 'UTC' } }] },
    };
    adoptSyncOverrideSettings(drafts, targetId, repositoryId, override);
    stageSyncOverrideControl(
      drafts,
      targetId,
      repositoryId,
      override,
      { ...buildSyncOverrideEditorEnvelope(override), override_texts: ['{"timezone": '] },
      `repositories.${repositoryId}.sync.files.document`,
    );
    const save = vi.fn();

    const result = await saveWorkspaceDrafts(drafts, targetId, save);
    expect(result).toMatchObject({
      saved: false,
      problemControl: {
        id: `repositories.${repositoryId}.sync.files.document`,
        location: {
          section: 'repositories',
          path: [repositoryId, 'sync', 'files', 'document'],
        },
      },
    });
    expect(save).not.toHaveBeenCalled();
    expect(drafts.hasDirty({ type: 'workspace', targetId })).toBe(true);
    expect(drafts.operation({ type: 'workspace', targetId }).problem).toContain(
      'not a JSON object',
    );
  });

  it('preserves a stale draft and rebases it on the complete 409 state', async () => {
    const drafts = registry();
    const targetId = TARGET.id;
    adoptTargetDefaults(drafts, TARGET);
    const wanted = {
      ...targetDefaultsDraftDocument(drafts, TARGET),
      repository_default_enabled: !TARGET.repository_default_enabled,
    };
    stageTargetDefaultsControl(drafts, TARGET, wanted, 'defaults.repository_default_enabled');
    const wantedFormatting = {
      ...targetDefaultsDraftDocument(drafts, TARGET),
      config_patch: {
        ...targetDefaultsDraftDocument(drafts, TARGET).config_patch,
        formatting: { json: { arrays: 'expanded' as const } },
      },
    };
    stageTargetDefaultsControl(
      drafts,
      TARGET,
      wantedFormatting,
      'defaults.config_patch.formatting.json.arrays',
    );
    const latest = {
      target_id: targetId,
      ...buildTargetDefaultsDocument(TARGET),
      repository_default_enabled: TARGET.repository_default_enabled,
      pending_ci_quiet_period_seconds_override: 75,
      config_patch: {
        ...TARGET.config_patch,
        formatting: { common: { line_width: 120 } },
      },
      revision: TARGET.revision + 1,
    };
    const save = vi.fn(async () => {
      throw new PanelApiError(409, 'settings_conflict', 'Settings changed elsewhere.', undefined, [
        {
          resource: 'target',
          target_id: targetId,
          expected_revision: TARGET.revision,
          actual_revision: latest.revision,
          latest,
        },
      ]);
    });

    await expect(saveWorkspaceDrafts(drafts, targetId, save)).resolves.toEqual({
      saved: false,
    });
    expect(drafts.resource(targetDefaultsResource(targetId))?.conflict).toMatchObject({
      type: 'revision',
      actualRevision: latest.revision,
    });
    expect(rebaseWorkspaceConflicts(drafts, targetId)).toBe(1);
    const rebased = drafts.resource(targetDefaultsResource(targetId));
    expect(rebased?.expectedRevision).toBe(latest.revision);
    expect(rebased?.conflict).toBeNull();
    expect(targetDefaultsDraftDocument(drafts, TARGET).repository_default_enabled).toBe(
      wanted.repository_default_enabled,
    );
    expect(
      targetDefaultsDraftDocument(drafts, TARGET).pending_ci_quiet_period_seconds_override,
    ).toBe(75);
    expect(targetDefaultsDraftDocument(drafts, TARGET).config_patch.formatting).toEqual({
      common: { line_width: 120 },
      json: { arrays: 'expanded' },
    });
  });

  it('rebases one dirty Sync control without replacing concurrent document changes', async () => {
    const drafts = registry();
    const targetId = TARGET.id;
    const config = {
      ...emptySyncConfig('settings'),
      enabled: false,
      revision: 3,
      document: { merge_method: 'squash' },
    };
    adoptSyncConfigSettings(drafts, targetId, config);
    stageSyncConfigControl(
      drafts,
      targetId,
      config,
      { ...buildSyncConfigEditorEnvelope(config), enabled: true },
      'sync.settings.enabled',
    );
    const latest = {
      target_id: targetId,
      kind: 'settings' as const,
      enabled: false,
      document: { merge_method: 'merge', future_setting: true },
      revision: 4,
    };
    const save = vi.fn(async () => {
      throw new PanelApiError(409, 'settings_conflict', 'Settings changed elsewhere.', undefined, [
        {
          resource: 'sync_config',
          target_id: targetId,
          kind: 'settings',
          expected_revision: config.revision,
          actual_revision: latest.revision,
          latest,
        },
      ]);
    });

    await expect(saveWorkspaceDrafts(drafts, targetId, save)).resolves.toEqual({
      saved: false,
    });
    expect(rebaseWorkspaceConflicts(drafts, targetId)).toBe(1);
    const rebased = syncConfigDraftEnvelope(drafts, targetId, config);
    expect(rebased).toMatchObject({ kind: 'settings', enabled: true });
    if (rebased.kind === 'labels') return;
    expect(rebased.document_text).toContain('"merge_method": "merge"');
    expect(rebased.document_text).toContain('"future_setting": true');
  });
});
