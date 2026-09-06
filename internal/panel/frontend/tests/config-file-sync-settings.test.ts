import { describe, expect, it } from 'vitest';

import {
  adoptRepositorySettings,
  buildRepositorySettingsDocument,
  overlayRepositorySettingsDocument,
  parseRepositorySettingsDocument,
  repositorySettingsBatchInput,
  repositorySettingsCommittedResource,
  repositorySettingsDraftDocument,
  repositorySettingsResource,
  stageRepositorySettingsControl,
} from '../src/lib/repository-settings';
import { SettingsDraftRegistry } from '../src/lib/settings-drafts.svelte';
import { settingsCheckpointSummary } from '../src/lib/settings-checkpoint-summary';
import {
  adoptTargetDefaults,
  buildTargetDefaultsDocument,
  overlayTargetDefaultsDocument,
  parseTargetDefaultsDocument,
  stageTargetDefaultsControl,
  targetDefaultsCommittedState,
  targetDefaultsDraftDocument,
  targetDefaultsResource,
} from '../src/lib/target-defaults-settings';
import { rebaseWorkspaceConflicts, saveWorkspaceDrafts } from '../src/lib/workspace-settings-save';
import { TARGET, REPOSITORY_DETAIL } from '../stories/support/fixtures';

const target = { ...TARGET, config_file_sync_enabled: true };
const repository = { ...REPOSITORY_DETAIL, config_file_sync_enabled: true };
const targetControl = 'defaults.config_file_sync_enabled';
const repositoryControl =
  `repositories.${repository.repository.id}.config_file_sync_enabled` as const;
const scope = { type: 'workspace', targetId: target.id } as const;

function registry() {
  const drafts = new SettingsDraftRegistry({ storage: null, now: () => 1, writerId: 'file-sync' });
  drafts.hydrate('viewer');
  return drafts;
}

describe('configuration file sync persistence [Unit]', () => {
  it.each([true, false])(
    'retains explicit %s through documents, overlays and canonical responses',
    (enabled) => {
      const targetDocument = buildTargetDefaultsDocument({
        ...target,
        config_file_sync_enabled: enabled,
      });
      const repositoryDocument = buildRepositorySettingsDocument({
        ...repository,
        config_file_sync_enabled: enabled,
      });
      expect(parseTargetDefaultsDocument(targetDocument)?.config_file_sync_enabled).toBe(enabled);
      expect(parseRepositorySettingsDocument(repositoryDocument)?.config_file_sync_enabled).toBe(
        enabled,
      );
      expect(overlayTargetDefaultsDocument(target, targetDocument).config_file_sync_enabled).toBe(
        enabled,
      );
      expect(
        overlayRepositorySettingsDocument(repository, repositoryDocument).config_file_sync_enabled,
      ).toBe(enabled);
      expect(
        targetDefaultsCommittedState({ ...targetDocument, target_id: target.id, revision: 12 })
          .savedControls[targetControl],
      ).toBe(enabled);
      expect(
        repositorySettingsCommittedResource(target.id, {
          ...repositoryDocument,
          repository_id: repository.repository.id,
          revision: 12,
        }).savedControls[repositoryControl],
      ).toBe(enabled);
    },
  );

  it.each([null, 'true', 1, {}, []])('rejects non-boolean persisted flags %s', (value) => {
    expect(
      parseTargetDefaultsDocument({
        ...buildTargetDefaultsDocument(target),
        config_file_sync_enabled: value,
      }),
    ).toBeNull();
    expect(
      parseRepositorySettingsDocument({
        ...buildRepositorySettingsDocument(repository),
        config_file_sync_enabled: value,
      }),
    ).toBeNull();
  });

  it('preserves omission in old drafts and falls back to the current server value in overlays', () => {
    const oldTarget = buildTargetDefaultsDocument(target);
    const oldRepository = buildRepositorySettingsDocument(repository);
    delete oldTarget.config_file_sync_enabled;
    delete oldRepository.config_file_sync_enabled;
    const parsedTarget = parseTargetDefaultsDocument(oldTarget)!;
    const parsedRepository = parseRepositorySettingsDocument(oldRepository)!;
    expect(Object.hasOwn(parsedTarget, 'config_file_sync_enabled')).toBe(false);
    expect(Object.hasOwn(parsedRepository, 'config_file_sync_enabled')).toBe(false);
    expect(overlayTargetDefaultsDocument(target, parsedTarget).config_file_sync_enabled).toBe(true);
    expect(
      overlayRepositorySettingsDocument(repository, parsedRepository).config_file_sync_enabled,
    ).toBe(true);
    expect(
      Object.hasOwn(
        repositorySettingsBatchInput(repository.repository.id, 1, parsedRepository),
        'config_file_sync_enabled',
      ),
    ).toBe(false);
  });

  it('clears both dirty controls when their saved value is restored', () => {
    const drafts = registry();
    adoptTargetDefaults(drafts, target);
    adoptRepositorySettings(drafts, target.id, repository);
    for (const enabled of [false, true]) {
      stageTargetDefaultsControl(
        drafts,
        target,
        { ...buildTargetDefaultsDocument(target), config_file_sync_enabled: enabled },
        targetControl,
      );
      stageRepositorySettingsControl(
        drafts,
        target.id,
        repository,
        { ...buildRepositorySettingsDocument(repository), config_file_sync_enabled: enabled },
        repositoryControl,
      );
      expect(drafts.hasDirty(scope)).toBe(!enabled);
    }
  });

  it('clears a matching restored backend value and preserves a local flag while rebasing other changes', () => {
    const drafts = registry();
    const off = { ...target, config_file_sync_enabled: false };
    adoptTargetDefaults(drafts, off);
    stageTargetDefaultsControl(drafts, off, buildTargetDefaultsDocument(target), targetControl);
    adoptTargetDefaults(drafts, { ...target, revision: off.revision + 1 });
    expect(drafts.hasDirty(scope)).toBe(false);

    adoptRepositorySettings(drafts, target.id, repository);
    stageRepositorySettingsControl(
      drafts,
      target.id,
      repository,
      { ...buildRepositorySettingsDocument(repository), config_file_sync_enabled: false },
      repositoryControl,
    );
    adoptRepositorySettings(drafts, target.id, {
      ...repository,
      config_patch: { quiet_pending: true },
      revision: repository.revision + 1,
    });
    expect(rebaseWorkspaceConflicts(drafts, target.id)).toBe(1);
    expect(repositorySettingsDraftDocument(drafts, target.id, repository)).toMatchObject({
      config_file_sync_enabled: false,
      config_patch: { quiet_pending: true },
    });
  });

  it('preserves a newly enabled server flag when rebasing unrelated edits in both scopes', () => {
    const drafts = registry();
    const offTarget = { ...target, config_file_sync_enabled: false };
    const offRepository = { ...repository, config_file_sync_enabled: false };
    adoptTargetDefaults(drafts, offTarget);
    adoptRepositorySettings(drafts, target.id, offRepository);
    stageTargetDefaultsControl(
      drafts,
      offTarget,
      {
        ...buildTargetDefaultsDocument(offTarget),
        repository_default_enabled: !offTarget.repository_default_enabled,
      },
      'defaults.repository_default_enabled',
    );
    stageRepositorySettingsControl(
      drafts,
      target.id,
      offRepository,
      {
        ...buildRepositorySettingsDocument(offRepository),
        ignore_repository_file: !offRepository.ignore_repository_file,
      },
      `repositories.${repository.repository.id}.ignore_repository_file`,
    );
    adoptTargetDefaults(drafts, { ...target, revision: target.revision + 1 });
    adoptRepositorySettings(drafts, target.id, {
      ...repository,
      revision: repository.revision + 1,
    });
    expect(rebaseWorkspaceConflicts(drafts, target.id)).toBe(2);
    expect(targetDefaultsDraftDocument(drafts, target).config_file_sync_enabled).toBe(true);
    expect(
      repositorySettingsDraftDocument(drafts, target.id, repository).config_file_sync_enabled,
    ).toBe(true);
  });

  it.each([false, true])(
    'saves unrelated edits without disabling flags, legacy draft=%s',
    async (legacy) => {
      const drafts = registry();
      const targetDocument = buildTargetDefaultsDocument(target);
      const repositoryDocument = buildRepositorySettingsDocument(repository);
      if (legacy) {
        delete targetDocument.config_file_sync_enabled;
        delete repositoryDocument.config_file_sync_enabled;
      }
      drafts.adoptBase(targetDefaultsResource(target.id), target.revision, targetDocument);
      drafts.adoptBase(
        repositorySettingsResource(target.id, repository.repository.id),
        repository.revision,
        repositoryDocument,
      );
      stageTargetDefaultsControl(
        drafts,
        target,
        {
          ...targetDocument,
          repository_default_enabled: !targetDocument.repository_default_enabled,
        },
        'defaults.repository_default_enabled',
      );
      stageRepositorySettingsControl(
        drafts,
        target.id,
        repository,
        {
          ...repositoryDocument,
          ignore_repository_file: !repositoryDocument.ignore_repository_file,
        },
        `repositories.${repository.repository.id}.ignore_repository_file`,
      );
      const result = await saveWorkspaceDrafts(drafts, target.id, async (_id, input) => {
        expect(input.target?.config_file_sync_enabled).toBe(legacy ? undefined : true);
        expect(input.repositories?.[0]?.config_file_sync_enabled).toBe(legacy ? undefined : true);
        return {
          target: {
            ...input.target!,
            config_file_sync_enabled: true,
            target_id: target.id,
            revision: target.revision + 1,
          },
          repositories: [
            {
              ...input.repositories![0]!,
              config_file_sync_enabled: true,
              revision: repository.revision + 1,
            },
          ],
        };
      });
      expect(result.saved).toBe(true);
      expect(drafts.hasDirty(scope)).toBe(false);
      expect(targetDefaultsDraftDocument(drafts, target).config_file_sync_enabled).toBe(true);
      expect(
        repositorySettingsDraftDocument(drafts, target.id, repository).config_file_sync_enabled,
      ).toBe(true);
    },
  );

  it.each(['target', 'repository'] as const)('names the checkpoint flag for %s', (kind) => {
    const item = {
      kind,
      document_version: 1,
      before: { available: false, state: null, differs: false, restorable: false },
      after: { available: false, state: null, differs: false, restorable: false },
      current: null,
      changed: true,
    };
    expect(
      settingsCheckpointSummary(item, {
        document: { config_file_sync_enabled: true },
        revision: 1,
        digest: 'a',
      }),
    ).toContain('Configuration file sync on');
    expect(
      settingsCheckpointSummary(item, {
        document: { config_file_sync_enabled: false },
        revision: 1,
        digest: 'b',
      }),
    ).toContain('Configuration file sync off');
    expect(
      settingsCheckpointSummary(item, { document: {}, revision: 1, digest: 'c' }),
    ).not.toContain('Configuration file sync');
  });
});
