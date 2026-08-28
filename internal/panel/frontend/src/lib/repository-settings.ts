import { CONFIG_KEYS } from './config';
import { FORMATTING_FIELDS, formattingPatchValue, parseFormattingPatch } from './formatting';
import type {
  ConfigKey,
  ConfigPatch,
  FormattingFieldKey,
  InstallationRepositorySettingsInput,
  InstallationRepositorySettingsState,
  PendingCIBranchPatterns,
  PendingCIMode,
  RepositoryDetail,
} from './types';
import type { SettingsCommittedResource, SettingsDraftRegistry } from './settings-drafts.svelte';
import type { SettingsJson, SettingsLocation, SettingsResource } from './settings-draft-storage';

const DOCUMENT_KEYS = [
  'enabled_override',
  'pending_ci_mode_override',
  'pending_ci_branch_patterns_override',
  'pending_ci_quiet_period_seconds_override',
  'path_index_interval_seconds_override',
  'config_patch',
  'ignore_repository_file',
] as const;

const PATTERN_KEYS = ['include', 'exclude'] as const;
const QUIET_PERIOD_MAX_SECONDS = 86_400;
const PATH_INDEX_MAX_SECONDS = 604_800;
const CONFIG_PATCH_KEYS = [...CONFIG_KEYS, 'formatting'] as const;

export type RepositorySettingsConfigPatch = Record<string, SettingsJson> & ConfigPatch;
export type RepositorySettingsBranchPatterns = Record<string, SettingsJson> &
  PendingCIBranchPatterns;

export type RepositorySettingsDocument = Record<string, SettingsJson> & {
  enabled_override: boolean | null;
  pending_ci_mode_override: PendingCIMode | null;
  pending_ci_branch_patterns_override: RepositorySettingsBranchPatterns | null;
  pending_ci_quiet_period_seconds_override: number | null;
  path_index_interval_seconds_override: number | null;
  config_patch: RepositorySettingsConfigPatch;
  ignore_repository_file: boolean;
};

export type RepositorySettingsControlId =
  | `repositories.${string}.enabled_override`
  | `repositories.${string}.pending_ci_mode_override`
  | `repositories.${string}.pending_ci_branch_patterns_override.include`
  | `repositories.${string}.pending_ci_branch_patterns_override.exclude`
  | `repositories.${string}.pending_ci_quiet_period_seconds_override`
  | `repositories.${string}.path_index_interval_seconds_override`
  | `repositories.${string}.ignore_repository_file`
  | `repositories.${string}.config_patch.${ConfigKey | FormattingFieldKey}`;

export interface RepositorySettingsControlDefinition {
  id: RepositorySettingsControlId;
  location: SettingsLocation;
}

/** Stable identities shared by staging, navigation attention, and reconciliation. */
export function repositorySettingsControls(
  repositoryId: string,
): readonly RepositorySettingsControlDefinition[] {
  const prefix = `repositories.${repositoryId}` as const;
  const at = (...path: string[]): SettingsLocation => ({
    section: 'repositories',
    path: [repositoryId, ...path],
  });

  return [
    { id: `${prefix}.enabled_override`, location: at('enablement', 'enabled_override') },
    {
      id: `${prefix}.pending_ci_mode_override`,
      location: at('merge', 'pending_ci_mode_override'),
    },
    {
      id: `${prefix}.pending_ci_branch_patterns_override.include`,
      location: at('merge', 'pending_ci_branch_patterns_override', 'include'),
    },
    {
      id: `${prefix}.pending_ci_branch_patterns_override.exclude`,
      location: at('merge', 'pending_ci_branch_patterns_override', 'exclude'),
    },
    {
      id: `${prefix}.pending_ci_quiet_period_seconds_override`,
      location: at('merge', 'pending_ci_quiet_period_seconds_override'),
    },
    {
      id: `${prefix}.path_index_interval_seconds_override`,
      location: at('merge', 'path_index_interval_seconds_override'),
    },
    {
      id: `${prefix}.ignore_repository_file`,
      location: at('file', 'ignore_repository_file'),
    },
    ...CONFIG_KEYS.map((key): RepositorySettingsControlDefinition => ({
      id: `${prefix}.config_patch.${key}`,
      location: at(configGroup(key), key),
    })),
    ...FORMATTING_FIELDS.map((field): RepositorySettingsControlDefinition => ({
      id: `${prefix}.config_patch.${field.key}`,
      location: at('formatting', ...field.path),
    })),
  ];
}

export function repositorySettingsResource(
  targetId: string,
  repositoryId: string,
): SettingsResource {
  return { type: 'repository-settings', targetId, repositoryId };
}

/** Adopt a server base without replacing a locally edited repository draft. */
export function adoptRepositorySettings(
  registry: SettingsDraftRegistry,
  targetId: string,
  detail: RepositoryDetail,
): boolean {
  return registry.adoptBase(
    repositorySettingsResource(targetId, detail.repository.id),
    detail.revision,
    buildRepositorySettingsDocument(detail),
  );
}

/** Return the staged document when dirty, otherwise a fresh canonical document. */
export function repositorySettingsDraftDocument(
  registry: SettingsDraftRegistry,
  targetId: string,
  detail: RepositoryDetail,
): RepositorySettingsDocument {
  const stored = registry.value(repositorySettingsResource(targetId, detail.repository.id));
  if (stored === null) return buildRepositorySettingsDocument(detail);
  const parsed = parseRepositorySettingsDocument(stored);
  if (parsed === null) throw new TypeError('stored repository settings are invalid');
  return parsed;
}

/** Stage one control while retaining the resource's complete document. */
export function stageRepositorySettingsControl(
  registry: SettingsDraftRegistry,
  targetId: string,
  detail: RepositoryDetail,
  nextValue: RepositorySettingsDocument,
  controlId: RepositorySettingsControlId,
): boolean {
  const definition = repositorySettingsControls(detail.repository.id).find(
    ({ id }) => id === controlId,
  );
  if (definition === undefined) return false;

  const next = parseRepositorySettingsDocument(nextValue);
  if (next === null) return false;
  const resource = repositorySettingsResource(targetId, detail.repository.id);
  const snapshot = registry.resource(resource);
  const base = parseRepositorySettingsDocument(
    snapshot?.base ?? buildRepositorySettingsDocument(detail),
  );
  if (base === null) return false;
  const saved = repositorySettingsSavedControls(detail.repository.id, base);
  const current = repositorySettingsSavedControls(detail.repository.id, next);

  return registry.stage(resource, next, {
    id: controlId,
    location: definition.location,
    saved: saved[controlId]!,
    value: current[controlId]!,
  });
}

/** Build only the complete editable state. Revision and inherited values stay metadata. */
export function buildRepositorySettingsDocument(
  detail: RepositoryDetail,
): RepositorySettingsDocument {
  const document = parseRepositorySettingsDocument({
    enabled_override: detail.repository.enabled_override,
    pending_ci_mode_override: detail.pending_ci_mode_override,
    pending_ci_branch_patterns_override: detail.pending_ci_branch_patterns_override,
    pending_ci_quiet_period_seconds_override: detail.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: detail.path_index_interval_seconds_override,
    config_patch: detail.config_patch,
    ignore_repository_file: detail.ignore_repository_file,
  });
  if (document === null) throw new TypeError('repository contains an invalid settings value');
  return document;
}

/** Reject partial, extended, out-of-range, or non-finite persisted documents. */
export function parseRepositorySettingsDocument(value: unknown): RepositorySettingsDocument | null {
  if (!isObject(value) || !hasExactKeys(value, DOCUMENT_KEYS)) return null;
  if (!isOptionalBoolean(value.enabled_override)) return null;
  if (!isOptionalPendingCIMode(value.pending_ci_mode_override)) return null;
  const patterns = parseOptionalBranchPatterns(value.pending_ci_branch_patterns_override);
  if (patterns === undefined) return null;
  const quiet = parseOptionalSeconds(
    value.pending_ci_quiet_period_seconds_override,
    QUIET_PERIOD_MAX_SECONDS,
  );
  if (quiet === undefined) return null;
  const pathIndex = parseOptionalSeconds(
    value.path_index_interval_seconds_override,
    PATH_INDEX_MAX_SECONDS,
  );
  if (pathIndex === undefined) return null;
  const patch = parseConfigPatch(value.config_patch);
  if (patch === null || typeof value.ignore_repository_file !== 'boolean') return null;

  return {
    enabled_override: value.enabled_override,
    pending_ci_mode_override: value.pending_ci_mode_override,
    pending_ci_branch_patterns_override: patterns,
    pending_ci_quiet_period_seconds_override: quiet,
    path_index_interval_seconds_override: pathIndex,
    config_patch: patch,
    ignore_repository_file: value.ignore_repository_file,
  };
}

/** Render staged values without changing server identity, inherited, effective, or source data. */
export function overlayRepositorySettingsDocument(
  detail: RepositoryDetail,
  document: RepositorySettingsDocument,
): RepositoryDetail {
  const parsed = parseRepositorySettingsDocument(document);
  if (parsed === null) throw new TypeError('repository settings draft is invalid');

  return {
    ...detail,
    repository: { ...detail.repository, enabled_override: parsed.enabled_override },
    pending_ci_mode_override: parsed.pending_ci_mode_override,
    pending_ci_branch_patterns_override: parsed.pending_ci_branch_patterns_override,
    pending_ci_quiet_period_seconds_override: parsed.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: parsed.path_index_interval_seconds_override,
    config_patch: parsed.config_patch,
    ignore_repository_file: parsed.ignore_repository_file,
  };
}

/** Canonical values for every control, retaining inheritance as distinct from false or empty. */
export function repositorySettingsSavedControls(
  repositoryId: string,
  document: RepositorySettingsDocument,
): Readonly<Record<RepositorySettingsControlId, SettingsJson>> {
  const prefix = `repositories.${repositoryId}`;
  const patterns = document.pending_ci_branch_patterns_override;
  const controls: Record<string, SettingsJson> = {
    [`${prefix}.enabled_override`]: document.enabled_override,
    [`${prefix}.pending_ci_mode_override`]: document.pending_ci_mode_override,
    [`${prefix}.pending_ci_branch_patterns_override.include`]:
      patterns === null ? null : [...patterns.include],
    [`${prefix}.pending_ci_branch_patterns_override.exclude`]:
      patterns === null ? null : [...patterns.exclude],
    [`${prefix}.pending_ci_quiet_period_seconds_override`]:
      document.pending_ci_quiet_period_seconds_override,
    [`${prefix}.path_index_interval_seconds_override`]:
      document.path_index_interval_seconds_override,
    [`${prefix}.ignore_repository_file`]: document.ignore_repository_file,
  };

  for (const key of CONFIG_KEYS) {
    const overridden = Object.hasOwn(document.config_patch, key);
    controls[`${prefix}.config_patch.${key}`] = {
      overridden,
      value: overridden ? configValue(document.config_patch, key) : null,
    };
  }
  for (const field of FORMATTING_FIELDS) {
    const value =
      document.config_patch.formatting === undefined
        ? undefined
        : formattingPatchValue(document.config_patch.formatting, field);
    controls[`${prefix}.config_patch.${field.key}`] = {
      overridden: value !== undefined,
      value: value ?? null,
    };
  }
  return controls as Record<RepositorySettingsControlId, SettingsJson>;
}

/** Serialize the complete draft for the atomic installation settings request. */
export function repositorySettingsBatchInput(
  repositoryId: string,
  expectedRevision: number,
  document: RepositorySettingsDocument,
): InstallationRepositorySettingsInput {
  if (!isRevision(expectedRevision))
    throw new TypeError('repository revision must be non-negative');
  const parsed = parseRepositorySettingsDocument(document);
  if (parsed === null) throw new TypeError('repository settings draft is invalid');

  return { repository_id: repositoryId, ...parsed, expected_revision: expectedRevision };
}

/** Convert a compact canonical batch response into the registry's commit result. */
export function repositorySettingsCommittedResource(
  targetId: string,
  state: InstallationRepositorySettingsState,
): SettingsCommittedResource {
  const value = parseRepositorySettingsDocument({
    enabled_override: state.enabled_override,
    pending_ci_mode_override: state.pending_ci_mode_override,
    pending_ci_branch_patterns_override: state.pending_ci_branch_patterns_override,
    pending_ci_quiet_period_seconds_override: state.pending_ci_quiet_period_seconds_override,
    path_index_interval_seconds_override: state.path_index_interval_seconds_override,
    config_patch: state.config_patch,
    ignore_repository_file: state.ignore_repository_file,
  });
  if (value === null) throw new TypeError('saved repository settings are invalid');
  return {
    resource: repositorySettingsResource(targetId, state.repository_id),
    revision: state.revision,
    value,
    savedControls: repositorySettingsSavedControls(state.repository_id, value),
  };
}

function configGroup(key: ConfigKey): 'behavior' | 'commands' {
  return key === 'command_prefix' || key === 'allowed_commands' || key === 'command_aliases'
    ? 'commands'
    : 'behavior';
}

function parseOptionalBranchPatterns(
  value: unknown,
): RepositorySettingsBranchPatterns | null | undefined {
  if (value === null) return null;
  if (!isObject(value) || !hasExactKeys(value, PATTERN_KEYS)) return undefined;
  if (!isStringArray(value.include) || !isStringArray(value.exclude)) return undefined;
  return { include: [...value.include], exclude: [...value.exclude] };
}

function parseOptionalSeconds(value: unknown, maximum: number): number | null | undefined {
  if (value === null) return null;
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0 || value > maximum) {
    return undefined;
  }
  return value;
}

function parseConfigPatch(value: unknown): RepositorySettingsConfigPatch | null {
  if (!isObject(value) || !hasOnlyKeys(value, CONFIG_PATCH_KEYS)) return null;
  const patch: ConfigPatch = {};
  for (const key of CONFIG_KEYS) {
    if (!Object.hasOwn(value, key)) continue;
    const candidate = value[key];
    if (isBooleanConfigKey(key)) {
      if (typeof candidate !== 'boolean') return null;
      patch[key] = candidate;
    } else if (key === 'command_prefix') {
      if (typeof candidate !== 'string') return null;
      patch.command_prefix = candidate;
    } else if (key === 'allowed_commands') {
      if (!isStringArray(candidate)) return null;
      patch.allowed_commands = [...candidate];
    } else {
      if (!isObject(candidate)) return null;
      const entries: Array<[string, string]> = [];
      for (const [alias, command] of Object.entries(candidate)) {
        if (typeof command !== 'string') return null;
        entries.push([alias, command]);
      }
      patch.command_aliases = Object.fromEntries(entries);
    }
  }
  if (Object.hasOwn(value, 'formatting')) {
    const formatting = parseFormattingPatch(value.formatting);
    if (formatting === null) return null;
    patch.formatting = formatting;
  }
  return patch;
}

function configValue(patch: ConfigPatch, key: ConfigKey): SettingsJson {
  const value = patch[key];
  if (value === undefined) return null;
  if (Array.isArray(value)) return [...value];
  if (typeof value === 'object' && value !== null) return Object.fromEntries(Object.entries(value));
  return value;
}

function isBooleanConfigKey(
  key: ConfigKey,
): key is Exclude<ConfigKey, 'command_prefix' | 'allowed_commands' | 'command_aliases'> {
  return key !== 'command_prefix' && key !== 'allowed_commands' && key !== 'command_aliases';
}

function isOptionalBoolean(value: unknown): value is boolean | null {
  return value === null || typeof value === 'boolean';
}

function isOptionalPendingCIMode(value: unknown): value is PendingCIMode | null {
  return value === null || value === 'checks' || value === 'labels';
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === 'string');
}

function isObject(value: unknown): value is Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function hasExactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

function hasOnlyKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  return Object.keys(value).every((key) => keys.includes(key));
}

function isRevision(value: number): boolean {
  return Number.isSafeInteger(value) && value >= 0;
}
