import { CONFIG_KEYS } from './config';
import type {
  ConfigKey,
  ConfigPatch,
  InstallationTargetSettingsState,
  PanelTarget,
  PendingCIBranchPatterns,
  PendingCIMode,
} from './types';
import type { SettingsCommittedResource, SettingsDraftRegistry } from './settings-drafts.svelte';
import type { SettingsJson, SettingsLocation, SettingsResource } from './settings-draft-storage';

const TARGET_DEFAULTS_KEYS = [
  'repository_default_enabled',
  'pending_ci_mode_default',
  'pending_ci_branch_patterns_default',
  'pending_ci_quiet_period_seconds_override',
  'path_index_interval_seconds_override',
  'config_patch',
] as const;

const PENDING_CI_BRANCH_PATTERN_KEYS = ['include', 'exclude'] as const;
const QUIET_PERIOD_MAX_SECONDS = 86_400;
const PATH_INDEX_MAX_SECONDS = 604_800;

export type TargetDefaultsBranchPatterns = Record<string, SettingsJson> & {
  include: string[];
  exclude: string[];
};

export type TargetDefaultsConfigPatch = Record<string, SettingsJson> & ConfigPatch;

export type TargetDefaultsDocument = Record<string, SettingsJson> & {
  repository_default_enabled: boolean;
  pending_ci_mode_default: PendingCIMode;
  pending_ci_branch_patterns_default: TargetDefaultsBranchPatterns;
  pending_ci_quiet_period_seconds_override: number | null;
  path_index_interval_seconds_override: number | null;
  config_patch: TargetDefaultsConfigPatch;
};

export type TargetDefaultsControlId =
  | 'defaults.repository_default_enabled'
  | 'defaults.path_index_interval_seconds_override'
  | 'defaults.pending_ci_mode_default'
  | 'defaults.pending_ci_branch_patterns_default.include'
  | 'defaults.pending_ci_branch_patterns_default.exclude'
  | 'defaults.pending_ci_quiet_period_seconds_override'
  | `defaults.config_patch.${ConfigKey}`;

export interface TargetDefaultsControlDefinition {
  id: TargetDefaultsControlId;
  location: SettingsLocation;
}

const fixedControls: readonly TargetDefaultsControlDefinition[] = [
  {
    id: 'defaults.repository_default_enabled',
    location: { section: 'defaults', path: ['repositories', 'repository_default_enabled'] },
  },
  {
    id: 'defaults.path_index_interval_seconds_override',
    location: {
      section: 'defaults',
      path: ['repositories', 'path_index_interval_seconds_override'],
    },
  },
  {
    id: 'defaults.pending_ci_mode_default',
    location: { section: 'defaults', path: ['merge', 'pending_ci_mode_default'] },
  },
  {
    id: 'defaults.pending_ci_branch_patterns_default.include',
    location: {
      section: 'defaults',
      path: ['merge', 'pending_ci_branch_patterns_default', 'include'],
    },
  },
  {
    id: 'defaults.pending_ci_branch_patterns_default.exclude',
    location: {
      section: 'defaults',
      path: ['merge', 'pending_ci_branch_patterns_default', 'exclude'],
    },
  },
  {
    id: 'defaults.pending_ci_quiet_period_seconds_override',
    location: {
      section: 'defaults',
      path: ['merge', 'pending_ci_quiet_period_seconds_override'],
    },
  },
];

/** Stable control identities are shared by staging, navigation badges, and save reconciliation. */
export const TARGET_DEFAULTS_CONTROLS: readonly TargetDefaultsControlDefinition[] = [
  ...fixedControls,
  ...CONFIG_KEYS.map(configControlDefinition),
];

export function targetDefaultsResource(targetId: string): SettingsResource {
  return { type: 'target-defaults', targetId };
}

/** Adopt the latest server document without replacing a locally edited draft. */
export function adoptTargetDefaults(registry: SettingsDraftRegistry, target: PanelTarget): boolean {
  return registry.adoptBase(
    targetDefaultsResource(target.id),
    target.revision,
    buildTargetDefaultsDocument(target),
  );
}

/** Read the document the editor should show: its draft when dirty, otherwise the server base. */
export function targetDefaultsDraftDocument(
  registry: SettingsDraftRegistry,
  target: PanelTarget,
): TargetDefaultsDocument {
  const stored = registry.value(targetDefaultsResource(target.id));
  if (stored === null) return buildTargetDefaultsDocument(target);
  const document = parseTargetDefaultsDocument(stored);
  if (document === null) throw new TypeError('stored target defaults are invalid');
  return document;
}

/** Stage one identified control while keeping the resource document complete. */
export function stageTargetDefaultsControl(
  registry: SettingsDraftRegistry,
  target: PanelTarget,
  nextValue: TargetDefaultsDocument,
  controlId: TargetDefaultsControlId,
): boolean {
  const definition = TARGET_DEFAULTS_CONTROLS.find(({ id }) => id === controlId);
  if (definition === undefined) return false;

  const next = parseTargetDefaultsDocument(nextValue);
  if (next === null) return false;
  const resource = targetDefaultsResource(target.id);
  const snapshot = registry.resource(resource);
  const base = parseTargetDefaultsDocument(snapshot?.base ?? buildTargetDefaultsDocument(target));
  if (base === null) return false;
  const savedControls = targetDefaultsSavedControls(base);
  const nextControls = targetDefaultsSavedControls(next);

  return registry.stage(resource, next, {
    id: controlId,
    location: definition.location,
    saved: savedControls[controlId]!,
    value: nextControls[controlId]!,
  });
}

/** Build the complete stored document. Revision remains resource metadata, not editable state. */
export function buildTargetDefaultsDocument(target: PanelTarget): TargetDefaultsDocument {
  const document = parseTargetDefaultsDocument({
    repository_default_enabled: target.repository_default_enabled,
    pending_ci_mode_default: target.pending_ci_mode_default,
    pending_ci_branch_patterns_default: target.pending_ci_branch_patterns_default,
    pending_ci_quiet_period_seconds_override: target.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: target.path_index_interval_seconds_override,
    config_patch: target.config_patch,
  });
  if (document === null) throw new TypeError('target defaults contain an invalid settings value');
  return document;
}

/** Parse an untrusted persisted value and reject partial, extended, or non-finite documents. */
export function parseTargetDefaultsDocument(value: unknown): TargetDefaultsDocument | null {
  if (!isObject(value) || !hasExactKeys(value, TARGET_DEFAULTS_KEYS)) return null;
  if (typeof value.repository_default_enabled !== 'boolean') return null;
  if (!isPendingCIMode(value.pending_ci_mode_default)) return null;

  const patterns = parseBranchPatterns(value.pending_ci_branch_patterns_default);
  if (patterns === null) return null;
  const quietPeriod = parseOptionalSeconds(
    value.pending_ci_quiet_period_seconds_override,
    QUIET_PERIOD_MAX_SECONDS,
  );
  if (quietPeriod === undefined) return null;
  const pathIndex = parseOptionalSeconds(
    value.path_index_interval_seconds_override,
    PATH_INDEX_MAX_SECONDS,
  );
  if (pathIndex === undefined) return null;
  const configPatch = parseConfigPatch(value.config_patch);
  if (configPatch === null) return null;

  return {
    repository_default_enabled: value.repository_default_enabled,
    pending_ci_mode_default: value.pending_ci_mode_default,
    pending_ci_branch_patterns_default: patterns,
    pending_ci_quiet_period_seconds_override: quietPeriod,
    path_index_interval_seconds_override: pathIndex,
    config_patch: configPatch,
  };
}

/** Render a draft while preserving identity, permissions, inherited values, and revision metadata. */
export function overlayTargetDefaultsDocument(
  target: PanelTarget,
  document: TargetDefaultsDocument,
): PanelTarget {
  return {
    ...target,
    repository_default_enabled: document.repository_default_enabled,
    pending_ci_mode_default: document.pending_ci_mode_default,
    pending_ci_branch_patterns_default: cloneBranchPatterns(
      document.pending_ci_branch_patterns_default,
    ),
    pending_ci_quiet_period_seconds_override: document.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: document.path_index_interval_seconds_override,
    config_patch: cloneConfigPatch(document.config_patch),
  };
}

/** Canonical saved values for every target-defaults control, including inherited config keys. */
export function targetDefaultsSavedControls(
  document: TargetDefaultsDocument,
): Readonly<Record<string, SettingsJson>> {
  const controls: Record<string, SettingsJson> = {
    'defaults.repository_default_enabled': document.repository_default_enabled,
    'defaults.path_index_interval_seconds_override': document.path_index_interval_seconds_override,
    'defaults.pending_ci_mode_default': document.pending_ci_mode_default,
    'defaults.pending_ci_branch_patterns_default.include': [
      ...document.pending_ci_branch_patterns_default.include,
    ],
    'defaults.pending_ci_branch_patterns_default.exclude': [
      ...document.pending_ci_branch_patterns_default.exclude,
    ],
    'defaults.pending_ci_quiet_period_seconds_override':
      document.pending_ci_quiet_period_seconds_override,
  };

  for (const key of CONFIG_KEYS) {
    const overridden = Object.hasOwn(document.config_patch, key);
    const value = overridden ? configValue(document.config_patch, key) : null;
    controls[`defaults.config_patch.${key}`] = { overridden, value };
  }
  return controls;
}

/** Convert the server's canonical response into the registry's atomic-save result. */
export function targetDefaultsCommittedResource(target: PanelTarget): SettingsCommittedResource {
  const value = buildTargetDefaultsDocument(target);
  return {
    resource: targetDefaultsResource(target.id),
    revision: target.revision,
    value,
    savedControls: targetDefaultsSavedControls(value),
  };
}

/** Convert the compact atomic-save state without needing inherited presentation metadata. */
export function targetDefaultsCommittedState(
  state: InstallationTargetSettingsState,
): SettingsCommittedResource {
  const value = parseTargetDefaultsDocument({
    repository_default_enabled: state.repository_default_enabled,
    pending_ci_mode_default: state.pending_ci_mode_default,
    pending_ci_branch_patterns_default: state.pending_ci_branch_patterns_default,
    pending_ci_quiet_period_seconds_override: state.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: state.path_index_interval_seconds_override,
    config_patch: state.config_patch,
  });
  if (value === null) throw new TypeError('saved target defaults are invalid');
  return {
    resource: targetDefaultsResource(state.target_id),
    revision: state.revision,
    value,
    savedControls: targetDefaultsSavedControls(value),
  };
}

function configControlDefinition(key: ConfigKey): TargetDefaultsControlDefinition {
  return {
    id: `defaults.config_patch.${key}`,
    location: { section: 'defaults', path: [configGroup(key), key] },
  };
}

function configGroup(key: ConfigKey): 'behavior' | 'commands' {
  switch (key) {
    case 'command_prefix':
    case 'allowed_commands':
    case 'command_aliases':
      return 'commands';
    case 'quiet_success':
    case 'quiet_reactions':
    case 'quiet_pending':
    case 'disable_mentions':
    case 'disable_bare_commands':
    case 'disable_unapprove':
    case 'disable_reactions':
    case 'disable_deleted_comments':
    case 'allow_self_approval':
    case 'allow_draft_merges':
      return 'behavior';
  }
}

function parseBranchPatterns(value: unknown): TargetDefaultsBranchPatterns | null {
  if (!isObject(value) || !hasExactKeys(value, PENDING_CI_BRANCH_PATTERN_KEYS)) return null;
  if (!isStringArray(value.include) || !isStringArray(value.exclude)) return null;
  return { include: [...value.include], exclude: [...value.exclude] };
}

function parseOptionalSeconds(value: unknown, maximum: number): number | null | undefined {
  if (value === null) return null;
  if (!Number.isSafeInteger(value) || typeof value !== 'number' || value < 0 || value > maximum) {
    return undefined;
  }
  return value;
}

function parseConfigPatch(value: unknown): TargetDefaultsConfigPatch | null {
  if (!isObject(value) || !hasOnlyKeys(value, CONFIG_KEYS)) return null;
  const patch: TargetDefaultsConfigPatch = {};

  if (!copyBoolean(value, patch, 'quiet_success')) return null;
  if (!copyBoolean(value, patch, 'quiet_reactions')) return null;
  if (!copyBoolean(value, patch, 'quiet_pending')) return null;
  if (!copyStringArray(value, patch, 'allowed_commands')) return null;
  if (!copyAliases(value, patch)) return null;
  if (!copyString(value, patch, 'command_prefix')) return null;
  if (!copyBoolean(value, patch, 'disable_mentions')) return null;
  if (!copyBoolean(value, patch, 'disable_bare_commands')) return null;
  if (!copyBoolean(value, patch, 'disable_unapprove')) return null;
  if (!copyBoolean(value, patch, 'disable_reactions')) return null;
  if (!copyBoolean(value, patch, 'disable_deleted_comments')) return null;
  if (!copyBoolean(value, patch, 'allow_self_approval')) return null;
  if (!copyBoolean(value, patch, 'allow_draft_merges')) return null;
  return patch;
}

function copyBoolean(
  source: Record<string, unknown>,
  destination: ConfigPatch,
  key:
    | 'quiet_success'
    | 'quiet_reactions'
    | 'quiet_pending'
    | 'disable_mentions'
    | 'disable_bare_commands'
    | 'disable_unapprove'
    | 'disable_reactions'
    | 'disable_deleted_comments'
    | 'allow_self_approval'
    | 'allow_draft_merges',
): boolean {
  if (!Object.hasOwn(source, key)) return true;
  const value = source[key];
  if (typeof value !== 'boolean') return false;
  destination[key] = value;
  return true;
}

function copyString(
  source: Record<string, unknown>,
  destination: ConfigPatch,
  key: 'command_prefix',
): boolean {
  if (!Object.hasOwn(source, key)) return true;
  const value = source[key];
  if (typeof value !== 'string') return false;
  destination[key] = value;
  return true;
}

function copyStringArray(
  source: Record<string, unknown>,
  destination: ConfigPatch,
  key: 'allowed_commands',
): boolean {
  if (!Object.hasOwn(source, key)) return true;
  const value = source[key];
  if (!isStringArray(value)) return false;
  destination[key] = [...value];
  return true;
}

function copyAliases(source: Record<string, unknown>, destination: ConfigPatch): boolean {
  if (!Object.hasOwn(source, 'command_aliases')) return true;
  const value = source.command_aliases;
  if (!isObject(value)) return false;
  const aliases: Array<[string, string]> = [];
  for (const [alias, command] of Object.entries(value)) {
    if (typeof command !== 'string') return false;
    aliases.push([alias, command]);
  }
  destination.command_aliases = Object.fromEntries(aliases);
  return true;
}

function configValue(patch: ConfigPatch, key: ConfigKey): SettingsJson {
  switch (key) {
    case 'quiet_success':
    case 'quiet_reactions':
    case 'quiet_pending':
    case 'disable_mentions':
    case 'disable_bare_commands':
    case 'disable_unapprove':
    case 'disable_reactions':
    case 'disable_deleted_comments':
    case 'allow_self_approval':
    case 'allow_draft_merges':
      return patch[key] ?? null;
    case 'command_prefix':
      return patch.command_prefix ?? null;
    case 'allowed_commands':
      return patch.allowed_commands === undefined ? null : [...patch.allowed_commands];
    case 'command_aliases':
      return patch.command_aliases === undefined ? null : { ...patch.command_aliases };
  }
}

function cloneConfigPatch(patch: ConfigPatch): ConfigPatch {
  const parsed = parseConfigPatch(patch);
  if (parsed === null)
    throw new TypeError('target config patch contains an invalid settings value');
  return parsed;
}

function cloneBranchPatterns(patterns: PendingCIBranchPatterns): PendingCIBranchPatterns {
  return { include: [...patterns.include], exclude: [...patterns.exclude] };
}

function isPendingCIMode(value: unknown): value is PendingCIMode {
  return value === 'labels' || value === 'checks';
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === 'string');
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function hasExactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function hasOnlyKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  return Object.keys(value).every((key) => keys.includes(key));
}
