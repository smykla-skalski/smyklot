import { describe, expect, it, vi } from 'vitest';

import { PanelApiError } from '../src/lib/api';
import {
  rebaseInstallationConflicts,
  saveInstallationDrafts,
} from '../src/lib/installation-settings-save';
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
  adoptTargetDefaults,
  stageTargetDefaultsControl,
  targetDefaultsDraftDocument,
  targetDefaultsResource,
} from '../src/lib/target-defaults-settings';
import type {
  InstallationSettingsBatchInput,
  InstallationSettingsBatchResponse,
  SyncOverride,
} from '../src/lib/types';
import { REPOSITORY_DETAIL, TARGET } from '../stories/support/fixtures';

function registry(): SettingsDraftRegistry {
  const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'test' });
  drafts.hydrate('viewer-1');
  return drafts;
}

function emptyOverride(): SyncOverride {
  return { kind: 'files', enabled: null, document: {}, revision: 0, unreadable: false };
}

describe('installation settings save coordinator [Unit]', () => {
  it('sends target, repository, and override drafts in one request and commits together', async () => {
    const drafts = registry();
    const targetId = TARGET.id;
    const repositoryId = REPOSITORY_DETAIL.repository.id;
    const override = emptyOverride();

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

    const save = vi.fn(
      async (
        sentTargetId: string,
        input: InstallationSettingsBatchInput,
      ): Promise<InstallationSettingsBatchResponse> => {
        expect(sentTargetId).toBe(targetId);
        expect(input.target?.expected_revision).toBe(TARGET.revision);
        expect(input.repositories).toHaveLength(1);
        expect(input.sync_overrides).toHaveLength(1);
        const { expected_revision: _targetRevision, ...savedTarget } = input.target!;
        const { expected_revision: _repositoryRevision, ...savedRepository } =
          input.repositories![0]!;
        const { expected_revision: _overrideRevision, ...savedOverride } =
          input.sync_overrides![0]!;
        void _targetRevision;
        void _repositoryRevision;
        void _overrideRevision;
        return {
          checkpoint_id: '91',
          target: { target_id: targetId, ...savedTarget, revision: TARGET.revision + 1 },
          repositories: [{ ...savedRepository, revision: REPOSITORY_DETAIL.revision + 1 }],
          sync_overrides: [
            { target_id: targetId, ...savedOverride, revision: override.revision + 1 },
          ],
        };
      },
    );

    await expect(saveInstallationDrafts(drafts, targetId, save)).resolves.toEqual({
      saved: true,
      checkpointId: '91',
    });
    expect(save).toHaveBeenCalledOnce();
    expect(drafts.hasDirty({ type: 'installation', targetId })).toBe(false);
    expect(drafts.operation({ type: 'installation', targetId }).notice).toContain(
      'Reconciliation creates a plan only when repositories need changes',
    );
  });

  it('blocks malformed persisted editor text before any request', async () => {
    const drafts = registry();
    const targetId = TARGET.id;
    const repositoryId = REPOSITORY_DETAIL.repository.id;
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

    await expect(saveInstallationDrafts(drafts, targetId, save)).resolves.toEqual({
      saved: false,
    });
    expect(save).not.toHaveBeenCalled();
    expect(drafts.hasDirty({ type: 'installation', targetId })).toBe(true);
    expect(drafts.operation({ type: 'installation', targetId }).problem).toContain(
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
    const latest = {
      target_id: targetId,
      ...targetDefaultsDraftDocument(drafts, TARGET),
      repository_default_enabled: TARGET.repository_default_enabled,
      pending_ci_quiet_period_seconds_override: 75,
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

    await expect(saveInstallationDrafts(drafts, targetId, save)).resolves.toEqual({
      saved: false,
    });
    expect(drafts.resource(targetDefaultsResource(targetId))?.conflict).toMatchObject({
      type: 'revision',
      actualRevision: latest.revision,
    });
    expect(rebaseInstallationConflicts(drafts, targetId)).toBe(1);
    const rebased = drafts.resource(targetDefaultsResource(targetId));
    expect(rebased?.expectedRevision).toBe(latest.revision);
    expect(rebased?.conflict).toBeNull();
    expect(targetDefaultsDraftDocument(drafts, TARGET).repository_default_enabled).toBe(
      wanted.repository_default_enabled,
    );
    expect(
      targetDefaultsDraftDocument(drafts, TARGET).pending_ci_quiet_period_seconds_override,
    ).toBe(75);
  });
});
