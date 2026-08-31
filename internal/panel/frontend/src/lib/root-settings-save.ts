import { PanelApiError } from './api';
import { CONFIG_KEYS } from './config';
import {
  applyFormattingPatch,
  completeFormattingPatch,
  formattingField,
  formattingPoliciesEqual,
  formattingPolicyValue,
  isFormattingPreset,
  setFormattingPolicyValue,
} from './formatting';
import {
  applyRuntimeConfigPatch,
  buildRuntimeSettingsDraftDocument,
  parseRuntimeSettingsDraftDocument,
  RUNTIME_RESOURCE,
  ROOT_SETTINGS_SCOPE,
  runtimeConfigPatch,
  runtimeSettingsCommittedResource,
  runtimeSettingsSavedControls,
  serializeRuntimeSettingsDraft,
} from './runtime-settings';
import type { SettingsDraftRegistry, SettingsSaveAttempt } from './settings-drafts.svelte';
import type {
  ConfigKey,
  ConfigPatch,
  RootRuntimeSettings,
  RootRuntimeSettingsInput,
} from './types';

export type FetchRootRuntimeSettings = () => Promise<RootRuntimeSettings>;
export type SaveRootRuntimeSettings = (
  input: RootRuntimeSettingsInput,
) => Promise<RootRuntimeSettings>;

export interface RootSettingsSaveResult {
  saved: boolean;
  settings?: RootRuntimeSettings;
  checkpointId?: string;
}

const savedNotice = 'Saved runtime settings';
const noOpNotice = 'Your draft already matches the saved runtime settings';

export async function saveRootSettingsDraft(
  registry: SettingsDraftRegistry,
  fetchSettings: FetchRootRuntimeSettings,
  saveSettings: SaveRootRuntimeSettings,
): Promise<RootSettingsSaveResult> {
  const attempt = registry.beginSave(ROOT_SETTINGS_SCOPE);
  if (attempt === null) return { saved: false };
  const entry = runtimeEntry(attempt);
  if (entry === null) {
    registry.failSave(attempt, 'The Root settings draft is incomplete');
    return { saved: false };
  }

  const document = parseRuntimeSettingsDraftDocument(entry.value);
  if (document === null) {
    registry.failSave(attempt, 'The Root settings draft is invalid');
    return { saved: false };
  }
  const serialized = serializeRuntimeSettingsDraft(entry.expectedRevision, document);
  if (!serialized.ok) {
    registry.failSave(attempt, serialized.problem);
    return { saved: false };
  }

  try {
    const response = await saveSettings(serialized.input);
    const notice = response.checkpoint_id === undefined ? noOpNotice : savedNotice;
    const accepted = registry.commitSave(
      attempt,
      [runtimeSettingsCommittedResource(response)],
      notice,
    );
    return {
      saved: accepted,
      ...(accepted ? { settings: response } : {}),
      ...(accepted && response.checkpoint_id !== undefined
        ? { checkpointId: response.checkpoint_id }
        : {}),
    };
  } catch (cause) {
    if (cause instanceof PanelApiError && cause.status === 409) {
      try {
        const latest = await fetchSettings();
        failConflict(registry, attempt, latest);
        return { saved: false };
      } catch {
        // Keep the service's conflict message when the follow-up read also fails
      }
    }
    registry.failSave(attempt, messageOf(cause));
    return { saved: false };
  }
}

export function rebaseRootSettingsConflict(
  registry: SettingsDraftRegistry,
  latest: RootRuntimeSettings,
): boolean {
  const snapshot = registry.resource(RUNTIME_RESOURCE);
  if (snapshot === null || snapshot.conflict?.type !== 'revision') return false;
  const draft = parseRuntimeSettingsDraftDocument(snapshot.value);
  if (draft === null) return false;

  const latestBase = buildRuntimeSettingsDraftDocument(latest);
  const merged = buildRuntimeSettingsDraftDocument(latest);
  const configPatch = runtimeConfigPatch(
    latest.behavior_defaults.deployment,
    latestBase.bot_config,
  );
  for (const control of snapshot.controls) {
    if (control.id.startsWith('runtime.bot_config.')) {
      const key = control.id.slice('runtime.bot_config.'.length);
      const field = formattingField(key);
      if (field !== undefined) {
        const current = applyFormattingPatch(
          latest.behavior_defaults.deployment.formatting,
          configPatch.formatting ?? {},
        );
        const desired =
          control.value === null
            ? formattingPolicyValue(latest.behavior_defaults.deployment.formatting, field)
            : draft.bot_config === null
              ? null
              : formattingPolicyValue(draft.bot_config.formatting, field);
        if (desired === null) return false;
        let resolved;
        if (field.key === 'formatting.preset' && control.value !== null) {
          if (!isFormattingPreset(desired)) return false;
          resolved = applyFormattingPatch(current, { preset: desired });
        } else {
          resolved = setFormattingPolicyValue(current, field, desired);
        }
        if (formattingPoliciesEqual(resolved, latest.behavior_defaults.deployment.formatting)) {
          delete configPatch.formatting;
        } else {
          configPatch.formatting = completeFormattingPatch(resolved);
        }
        continue;
      }
      if (!CONFIG_KEYS.includes(key as ConfigKey)) return false;
      const configKey = key as ConfigKey;
      if (control.value === null) delete configPatch[configKey];
      else Object.assign(configPatch, { [configKey]: control.value });
      continue;
    }
    if (control.id === 'runtime.log_level') {
      merged.log_level = draft.log_level;
      continue;
    }
    if (control.id === 'runtime.reaction_poll_interval_seconds') {
      merged.reaction_poll_interval_seconds = draft.reaction_poll_interval_seconds;
      continue;
    }
    if (control.id === 'runtime.merge_after_ci_quiet_period_seconds') {
      merged.merge_after_ci_quiet_period_seconds = draft.merge_after_ci_quiet_period_seconds;
      continue;
    }
    if (control.id === 'runtime.path_index_interval_seconds') {
      merged.path_index_interval_seconds = draft.path_index_interval_seconds;
      continue;
    }
    if (control.id === 'runtime.session_ttl_seconds') {
      merged.session_ttl_seconds = draft.session_ttl_seconds;
      continue;
    }
    return false;
  }
  merged.bot_config = applyRuntimeConfigPatch(
    latest.behavior_defaults.deployment,
    configPatch as ConfigPatch,
  );

  return registry.rebase(
    RUNTIME_RESOURCE,
    latest.revision,
    latestBase,
    runtimeSettingsSavedControls(latestBase, latest.behavior_defaults.deployment),
    merged,
  );
}

function runtimeEntry(attempt: SettingsSaveAttempt): SettingsSaveAttempt['entries'][number] | null {
  if (attempt.entries.length !== 1) return null;
  const entry = attempt.entries[0];
  return entry?.resource.type === 'runtime' ? entry : null;
}

function failConflict(
  registry: SettingsDraftRegistry,
  attempt: SettingsSaveAttempt,
  latest: RootRuntimeSettings,
): void {
  registry.failSave(attempt, 'Service settings changed in another session', [
    {
      resource: RUNTIME_RESOURCE,
      actualRevision: latest.revision,
      latestBase: buildRuntimeSettingsDraftDocument(latest),
    },
  ]);
}

function messageOf(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause);
}
