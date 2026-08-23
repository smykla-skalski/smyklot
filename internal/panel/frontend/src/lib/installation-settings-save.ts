import { PanelApiError } from './api';
import { CONFIG_KEYS } from './config';
import {
  parseRepositorySettingsDocument,
  repositorySettingsBatchInput,
  repositorySettingsCommittedResource,
  repositorySettingsResource,
  repositorySettingsSavedControls,
} from './repository-settings';
import {
  parseSyncOverrideEditorEnvelope,
  syncOverrideBatchInput,
  syncOverrideCommittedResource,
  syncOverrideSavedControls,
} from './repository-sync-override-settings';
import type {
  SettingsCommittedResource,
  SettingsDraftRegistry,
  SettingsSaveAttempt,
  SettingsSaveConflict,
} from './settings-drafts.svelte';
import type { SettingsJson, SettingsResource } from './settings-draft-storage';
import {
  parseSyncConfigEditorEnvelope,
  syncConfigBatchInput,
  syncConfigCommittedResource,
  syncConfigResource,
  syncConfigSavedControls,
  type SyncConfigEditorEnvelope,
} from './sync-config-settings';
import {
  parseTargetDefaultsDocument,
  targetDefaultsCommittedState,
  targetDefaultsResource,
  targetDefaultsSavedControls,
} from './target-defaults-settings';
import type {
  InstallationSettingsBatchInput,
  InstallationSettingsBatchResponse,
  InstallationSettingsConflict,
  SyncKind,
} from './types';

export type SaveInstallationSettings = (
  targetId: string,
  input: InstallationSettingsBatchInput,
) => Promise<InstallationSettingsBatchResponse>;

export interface InstallationSettingsSaveResult {
  saved: boolean;
  checkpointId?: string;
}

type SerializedBatch =
  { ok: true; input: InstallationSettingsBatchInput } | { ok: false; problem: string };

const savedNotice = 'Saved. Reconciliation creates a plan only when repositories need changes';
const noOpNotice = 'Your draft already matches the saved settings';

/** Send every dirty resource for one installation through exactly one atomic request. */
export async function saveInstallationDrafts(
  registry: SettingsDraftRegistry,
  targetId: string,
  save: SaveInstallationSettings,
): Promise<InstallationSettingsSaveResult> {
  const scope = { type: 'installation', targetId } as const;
  const attempt = registry.beginSave(scope);
  if (attempt === null) return { saved: false };

  const serialized = serializeInstallationAttempt(attempt, targetId);
  if (!serialized.ok) {
    registry.failSave(attempt, serialized.problem);
    return { saved: false };
  }

  try {
    const response = await save(targetId, serialized.input);
    const committed = committedResources(targetId, response);
    const notice = response.checkpoint_id === undefined ? noOpNotice : savedNotice;
    const accepted = registry.commitSave(attempt, committed, notice);
    return {
      saved: accepted,
      ...(accepted && response.checkpoint_id !== undefined
        ? { checkpointId: response.checkpoint_id }
        : {}),
    };
  } catch (cause) {
    const problem = cause instanceof Error ? cause.message : String(cause);
    const conflicts =
      cause instanceof PanelApiError
        ? cause.conflicts.flatMap((conflict) => settingsConflict(conflict))
        : [];
    registry.failSave(attempt, problem, conflicts);
    return { saved: false };
  }
}

/** Rebase preserved drafts on the latest complete documents returned with a 409. */
export function rebaseInstallationConflicts(
  registry: SettingsDraftRegistry,
  targetId: string,
): number {
  const scope = { type: 'installation', targetId } as const;
  let rebased = 0;
  for (const snapshot of registry.dirtyResources(scope)) {
    const conflict = snapshot.conflict;
    if (conflict?.type !== 'revision' || conflict.latestBase === undefined) continue;
    const savedControls = savedProjection(snapshot.resource, conflict.latestBase);
    const rebasedDraft = mergeDirtyControls(
      snapshot.resource,
      conflict.latestBase,
      snapshot.value,
      snapshot.controls.map(({ id }) => id),
    );
    if (savedControls === null || rebasedDraft === null) continue;
    if (
      registry.rebase(
        snapshot.resource,
        conflict.actualRevision,
        conflict.latestBase,
        savedControls,
        rebasedDraft,
      )
    ) {
      rebased += 1;
    }
  }
  return rebased;
}

function mergeDirtyControls(
  resource: SettingsResource,
  latestBase: SettingsJson,
  draftValue: SettingsJson,
  dirtyControlIds: readonly string[],
): SettingsJson | null {
  if (resource.type === 'target-defaults') {
    return mergeTargetDefaults(latestBase, draftValue, dirtyControlIds);
  }
  if (resource.type === 'repository-settings') {
    return mergeRepositorySettings(resource.repositoryId, latestBase, draftValue, dirtyControlIds);
  }
  if (resource.type === 'sync-override' && resource.kind === 'files') {
    return mergeSyncOverride(resource.repositoryId, latestBase, draftValue, dirtyControlIds);
  }
  if (resource.type === 'sync-config') {
    return mergeSyncConfig(resource.kind, latestBase, draftValue, dirtyControlIds);
  }
  return null;
}

function mergeSyncConfig(
  kind: SyncKind,
  latestBase: SettingsJson,
  draftValue: SettingsJson,
  ids: readonly string[],
): SettingsJson | null {
  const latest = parseSyncConfigEditorEnvelope(latestBase, kind);
  const draft = parseSyncConfigEditorEnvelope(draftValue, kind);
  if (latest === null || draft === null || latest.kind !== draft.kind) return null;

  for (const id of ids) {
    if (id === `sync.${kind}.enabled`) latest.enabled = draft.enabled;
    else if (latest.kind === 'labels' && draft.kind === 'labels') {
      if (id === 'sync.labels.labels') latest.labels = draft.labels.map((label) => ({ ...label }));
      else if (id === 'sync.labels.allow_removal') latest.allow_removal = draft.allow_removal;
      else if (id === 'sync.labels.excludes') latest.excludes = [...draft.excludes];
      else return null;
    } else if (
      latest.kind !== 'labels' &&
      draft.kind !== 'labels' &&
      id === `sync.${kind}.document`
    ) {
      latest.document_text = draft.document_text;
    } else return null;
  }
  return latest as SyncConfigEditorEnvelope;
}

function mergeTargetDefaults(
  latestBase: SettingsJson,
  draftValue: SettingsJson,
  ids: readonly string[],
): SettingsJson | null {
  const latest = parseTargetDefaultsDocument(latestBase);
  const draft = parseTargetDefaultsDocument(draftValue);
  if (latest === null || draft === null) return null;

  for (const id of ids) {
    if (id === 'defaults.repository_default_enabled') {
      latest.repository_default_enabled = draft.repository_default_enabled;
    } else if (id === 'defaults.path_index_interval_seconds_override') {
      latest.path_index_interval_seconds_override = draft.path_index_interval_seconds_override;
    } else if (id === 'defaults.pending_ci_mode_default') {
      latest.pending_ci_mode_default = draft.pending_ci_mode_default;
    } else if (id === 'defaults.pending_ci_quiet_period_seconds_override') {
      latest.pending_ci_quiet_period_seconds_override =
        draft.pending_ci_quiet_period_seconds_override;
    } else if (id === 'defaults.pending_ci_branch_patterns_default.include') {
      latest.pending_ci_branch_patterns_default.include = [
        ...draft.pending_ci_branch_patterns_default.include,
      ];
    } else if (id === 'defaults.pending_ci_branch_patterns_default.exclude') {
      latest.pending_ci_branch_patterns_default.exclude = [
        ...draft.pending_ci_branch_patterns_default.exclude,
      ];
    } else if (id.startsWith('defaults.config_patch.')) {
      const key = id.slice('defaults.config_patch.'.length);
      if (!isConfigKey(key)) return null;
      copyConfigKey(latest.config_patch, draft.config_patch, key);
    } else {
      return null;
    }
  }
  return latest;
}

function mergeRepositorySettings(
  repositoryId: string,
  latestBase: SettingsJson,
  draftValue: SettingsJson,
  ids: readonly string[],
): SettingsJson | null {
  const latest = parseRepositorySettingsDocument(latestBase);
  const draft = parseRepositorySettingsDocument(draftValue);
  if (latest === null || draft === null) return null;
  const prefix = `repositories.${repositoryId}.`;

  for (const id of ids) {
    if (!id.startsWith(prefix)) return null;
    const key = id.slice(prefix.length);
    if (key === 'enabled_override') latest.enabled_override = draft.enabled_override;
    else if (key === 'pending_ci_mode_override') {
      latest.pending_ci_mode_override = draft.pending_ci_mode_override;
    } else if (key === 'pending_ci_quiet_period_seconds_override') {
      latest.pending_ci_quiet_period_seconds_override =
        draft.pending_ci_quiet_period_seconds_override;
    } else if (key === 'path_index_interval_seconds_override') {
      latest.path_index_interval_seconds_override = draft.path_index_interval_seconds_override;
    } else if (key === 'ignore_repository_file') {
      latest.ignore_repository_file = draft.ignore_repository_file;
    } else if (key === 'pending_ci_branch_patterns_override.include') {
      latest.pending_ci_branch_patterns_override = mergeRepositoryPatterns(latest, draft);
      latest.pending_ci_branch_patterns_override.include = [
        ...(draft.pending_ci_branch_patterns_override?.include ?? []),
      ];
    } else if (key === 'pending_ci_branch_patterns_override.exclude') {
      latest.pending_ci_branch_patterns_override = mergeRepositoryPatterns(latest, draft);
      latest.pending_ci_branch_patterns_override.exclude = [
        ...(draft.pending_ci_branch_patterns_override?.exclude ?? []),
      ];
    } else if (key.startsWith('config_patch.')) {
      const configKey = key.slice('config_patch.'.length);
      if (!isConfigKey(configKey)) return null;
      copyConfigKey(latest.config_patch, draft.config_patch, configKey);
    } else {
      return null;
    }
  }
  return latest;
}

function mergeRepositoryPatterns(
  latest: NonNullable<ReturnType<typeof parseRepositorySettingsDocument>>,
  draft: NonNullable<ReturnType<typeof parseRepositorySettingsDocument>>,
): NonNullable<typeof latest.pending_ci_branch_patterns_override> {
  const source = latest.pending_ci_branch_patterns_override ??
    draft.pending_ci_branch_patterns_override ?? { include: [], exclude: [] };
  return { include: [...source.include], exclude: [...source.exclude] };
}

function mergeSyncOverride(
  repositoryId: string,
  latestBase: SettingsJson,
  draftValue: SettingsJson,
  ids: readonly string[],
): SettingsJson | null {
  const latest = parseSyncOverrideEditorEnvelope(latestBase);
  const draft = parseSyncOverrideEditorEnvelope(draftValue);
  if (latest === null || draft === null) return null;
  const prefix = `repositories.${repositoryId}.sync.files.`;
  for (const id of ids) {
    if (id === `${prefix}enabled`) latest.enabled = draft.enabled;
    else if (id === `${prefix}document`) {
      latest.document = draft.document;
      latest.override_texts = [...draft.override_texts];
    } else return null;
  }
  return latest;
}

function isConfigKey(value: string): value is (typeof CONFIG_KEYS)[number] {
  return CONFIG_KEYS.some((key) => key === value);
}

function copyConfigKey(
  destination: Record<string, SettingsJson>,
  source: Record<string, SettingsJson>,
  key: (typeof CONFIG_KEYS)[number],
): void {
  if (Object.hasOwn(source, key)) destination[key] = source[key]!;
  else delete destination[key];
}

function serializeInstallationAttempt(
  attempt: SettingsSaveAttempt,
  targetId: string,
): SerializedBatch {
  const input: InstallationSettingsBatchInput = {};
  const repositories: NonNullable<InstallationSettingsBatchInput['repositories']> = [];
  const syncConfigs: NonNullable<InstallationSettingsBatchInput['sync_configs']> = [];
  const syncOverrides: NonNullable<InstallationSettingsBatchInput['sync_overrides']> = [];

  for (const entry of [...attempt.entries].sort((left, right) =>
    left.resourceKey.localeCompare(right.resourceKey),
  )) {
    const resource = entry.resource;
    if (resource.type !== 'runtime' && resource.targetId !== targetId) {
      return { ok: false, problem: 'A draft belongs to a different installation' };
    }
    if (resource.type === 'target-defaults') {
      const document = parseTargetDefaultsDocument(entry.value);
      if (document === null) return { ok: false, problem: 'Workspace defaults are not valid' };
      input.target = { ...document, expected_revision: entry.expectedRevision };
      continue;
    }
    if (resource.type === 'repository-settings') {
      const document = parseRepositorySettingsDocument(entry.value);
      if (document === null) return { ok: false, problem: 'Repository settings are not valid' };
      repositories.push(
        repositorySettingsBatchInput(resource.repositoryId, entry.expectedRevision, document),
      );
      continue;
    }
    if (resource.type === 'sync-override') {
      if (resource.kind !== 'files') {
        return { ok: false, problem: 'This repository Sync draft cannot be saved safely' };
      }
      const envelope = parseSyncOverrideEditorEnvelope(entry.value);
      if (envelope === null) {
        return { ok: false, problem: 'Repository file Sync settings are not valid' };
      }
      const serialized = syncOverrideBatchInput(
        resource.repositoryId,
        entry.expectedRevision,
        envelope,
      );
      if (!serialized.ok) return serialized;
      syncOverrides.push(serialized.input);
      continue;
    }
    if (resource.type === 'sync-config') {
      const envelope = parseSyncConfigEditorEnvelope(entry.value, resource.kind);
      if (envelope === null) return { ok: false, problem: 'Sync configuration is not valid' };
      const serialized = syncConfigBatchInput(entry.expectedRevision, envelope);
      if (!serialized.ok) return serialized;
      syncConfigs.push(serialized.input);
      continue;
    }
    return { ok: false, problem: 'Runtime settings need the Root settings save' };
  }

  if (repositories.length > 0) input.repositories = repositories;
  if (syncConfigs.length > 0) input.sync_configs = syncConfigs;
  if (syncOverrides.length > 0) input.sync_overrides = syncOverrides;
  return { ok: true, input };
}

function committedResources(
  targetId: string,
  response: InstallationSettingsBatchResponse,
): SettingsCommittedResource[] {
  const committed: SettingsCommittedResource[] = [];
  if (response.target !== undefined) committed.push(targetDefaultsCommittedState(response.target));
  for (const state of response.repositories ?? []) {
    committed.push(repositorySettingsCommittedResource(targetId, state));
  }
  for (const state of response.sync_configs ?? []) {
    committed.push(syncConfigCommittedResource(state));
  }
  for (const state of response.sync_overrides ?? []) {
    committed.push(syncOverrideCommittedResource(state));
  }
  return committed;
}

function settingsConflict(conflict: InstallationSettingsConflict): SettingsSaveConflict[] {
  switch (conflict.resource) {
    case 'target':
      return [
        {
          resource: targetDefaultsResource(conflict.target_id),
          actualRevision: conflict.actual_revision,
          ...(conflict.latest === undefined
            ? {}
            : { latestBase: targetDefaultsCommittedState(conflict.latest).value }),
        },
      ];
    case 'repository':
      return [
        {
          resource: repositorySettingsResource(conflict.target_id, conflict.repository_id),
          actualRevision: conflict.actual_revision,
          ...(conflict.latest === undefined
            ? {}
            : {
                latestBase: repositorySettingsCommittedResource(conflict.target_id, conflict.latest)
                  .value,
              }),
        },
      ];
    case 'sync_override':
      return [
        {
          resource: {
            type: 'sync-override',
            targetId: conflict.target_id,
            repositoryId: conflict.repository_id,
            kind: conflict.kind,
          },
          actualRevision: conflict.actual_revision,
          ...(conflict.latest === undefined
            ? {}
            : { latestBase: syncOverrideCommittedResource(conflict.latest).value }),
        },
      ];
    case 'sync_config':
      return [
        {
          resource: syncConfigResource(conflict.target_id, conflict.kind),
          actualRevision: conflict.actual_revision,
          ...(conflict.latest === undefined
            ? {}
            : { latestBase: syncConfigCommittedResource(conflict.latest).value }),
        },
      ];
  }
}

function savedProjection(
  resource: SettingsResource,
  latestBase: SettingsJson,
): Readonly<Record<string, SettingsJson>> | null {
  if (resource.type === 'target-defaults') {
    const document = parseTargetDefaultsDocument(latestBase);
    return document === null ? null : targetDefaultsSavedControls(document);
  }
  if (resource.type === 'repository-settings') {
    const document = parseRepositorySettingsDocument(latestBase);
    return document === null
      ? null
      : repositorySettingsSavedControls(resource.repositoryId, document);
  }
  if (resource.type === 'sync-override' && resource.kind === 'files') {
    const envelope = parseSyncOverrideEditorEnvelope(latestBase);
    return envelope === null ? null : syncOverrideSavedControls(resource.repositoryId, envelope);
  }
  if (resource.type === 'sync-config') {
    const envelope = parseSyncConfigEditorEnvelope(latestBase, resource.kind);
    return envelope === null ? null : syncConfigSavedControls(envelope);
  }
  return null;
}
