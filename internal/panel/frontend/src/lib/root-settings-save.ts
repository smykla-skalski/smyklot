import { runtimeFieldConflicts, type RuntimeConflictChoice } from './runtime-conflicts';
import { PanelApiError } from './api';
import { CONFIG_KEYS } from './config';
import { formattingField, formattingPatchValue, setFormattingPatchValue } from './formatting';
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

/** Validate persisted runtime edits even while their page is not mounted. */
export function rootSettingsDraftValidation(registry: SettingsDraftRegistry) {
  const resource = registry.resource(RUNTIME_RESOURCE);
  if (resource === null || !registry.hasDirty(ROOT_SETTINGS_SCOPE)) return null;
  const document = parseRuntimeSettingsDraftDocument(resource.value);
  if (document === null) return null;
  const result = serializeRuntimeSettingsDraft(resource.expectedRevision, document);
  return result.ok ? null : result;
}

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
    const controlId =
      cause instanceof PanelApiError && cause.status === 400 && cause.field !== undefined
        ? `runtime.${cause.field}`
        : null;
    registry.failSave(
      attempt,
      messageOf(cause),
      [],
      controlId !== null && entry.controls.some((control) => control.id === controlId)
        ? [{ resource: RUNTIME_RESOURCE, controlId }]
        : [],
    );
    return { saved: false };
  }
}

export function rebaseRootSettingsConflict(
  registry: SettingsDraftRegistry,
  latest: RootRuntimeSettings,
  choices: Readonly<Record<string, RuntimeConflictChoice>> = {},
): boolean {
  const snapshot = registry.resource(RUNTIME_RESOURCE);
  if (snapshot === null || snapshot.conflict?.type !== 'revision') return false;
  const conflicts = runtimeFieldConflicts(registry, latest);
  if (conflicts.some((conflict) => choices[conflict.id] === undefined)) return false;
  const useSaved = new Set(
    conflicts.filter((conflict) => choices[conflict.id] === 'saved').map((conflict) => conflict.id),
  );
  const draft = parseRuntimeSettingsDraftDocument(snapshot.value);
  if (draft === null) return false;

  const latestBase = buildRuntimeSettingsDraftDocument(latest);
  const merged = buildRuntimeSettingsDraftDocument(latest);
  const configPatch = runtimeConfigPatch(latestBase.bot_config);
  for (const control of snapshot.controls) {
    if (useSaved.has(control.id)) continue;
    if (control.id.startsWith('runtime.bot_config.')) {
      const key = control.id.slice('runtime.bot_config.'.length);
      const field = formattingField(key);
      if (field !== undefined) {
        const desired =
          control.value === null
            ? undefined
            : formattingPatchValue(draft.bot_config?.overrides.formatting ?? {}, field);
        if (control.value !== null && desired === undefined) return false;
        configPatch.formatting = setFormattingPatchValue(
          configPatch.formatting ?? {},
          field,
          desired,
        );
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
  merged.bot_config = applyRuntimeConfigPatch(configPatch as ConfigPatch);

  return registry.rebase(
    RUNTIME_RESOURCE,
    latest.revision,
    latestBase,
    runtimeSettingsSavedControls(latestBase),
    merged,
    runtimeSettingsSavedControls(merged),
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
