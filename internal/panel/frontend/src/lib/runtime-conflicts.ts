import { BOOLEAN_FIELDS } from './config';
import { formattingField } from './formatting';
import {
  RUNTIME_RESOURCE,
  buildRuntimeSettingsDraftDocument,
  runtimeSettingsSavedControls,
} from './runtime-settings';
import { sameSettingsJson, type SettingsJson } from './settings-draft-storage';
import type { SettingsDraftRegistry } from './settings-drafts.svelte';
import type { RootRuntimeSettings } from './types';

export type RuntimeConflictChoice = 'draft' | 'saved';
export interface RuntimeFieldConflict {
  id: string;
  label: string;
  draft: SettingsJson;
  saved: SettingsJson;
}

export function runtimeFieldConflicts(
  registry: SettingsDraftRegistry,
  latest: RootRuntimeSettings,
): RuntimeFieldConflict[] {
  const snapshot = registry.resource(RUNTIME_RESOURCE);
  if (snapshot === null || snapshot.conflict?.type !== 'revision') return [];
  const saved = runtimeSettingsSavedControls(
    buildRuntimeSettingsDraftDocument(latest),
    latest.behavior_defaults.deployment,
  );
  return snapshot.controls.flatMap((control) => {
    const remote = saved[control.id as keyof typeof saved];
    if (
      remote === undefined ||
      sameSettingsJson(control.saved, remote) ||
      sameSettingsJson(control.value, remote)
    )
      return [];
    return [
      {
        id: control.id,
        label: runtimeControlLabel(control.id),
        draft: control.value,
        saved: remote,
      },
    ];
  });
}

function runtimeControlLabel(id: string): string {
  const key = id.replace('runtime.bot_config.', '');
  const behavior = BOOLEAN_FIELDS.find((field) => field.key === key);
  if (behavior !== undefined) return behavior.label;
  const formatting = formattingField(key);
  if (formatting !== undefined)
    return `Formatting: ${formatting.path.join(' ').replaceAll('_', ' ')}`;
  const labels: Record<string, string> = {
    command_prefix: 'Command prefix',
    allowed_commands: 'Allowed commands',
    command_aliases: 'Command aliases',
    'runtime.log_level': 'Log level',
    'runtime.reaction_poll_interval_seconds': 'Reaction sweep interval',
    'runtime.merge_after_ci_quiet_period_seconds': 'Merge-after-CI quiet period',
    'runtime.path_index_interval_seconds': 'File list refresh interval',
    'runtime.session_ttl_seconds': 'Session lifetime',
  };
  return labels[key] ?? id;
}

export function runtimeConflictValue(id: string, value: SettingsJson): string {
  if (value === null) return 'Follow deployment';
  const behavior = BOOLEAN_FIELDS.find((field) => `runtime.bot_config.${field.key}` === id);
  if (behavior !== undefined && typeof value === 'boolean')
    return (behavior.positive ? value : !value) ? 'On' : 'Off';
  if (typeof value === 'string') return value === '' ? 'Empty' : value;
  if (Array.isArray(value))
    return value.length === 0 ? 'All commands' : value.map(String).join(', ');
  if (typeof value === 'object' && 'override_seconds' in value)
    return value.override_seconds === null
      ? 'Follow deployment'
      : `${value.override_seconds} seconds`;
  return JSON.stringify(value);
}
