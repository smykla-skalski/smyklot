import { CONFIG_KEYS } from './config';
import { durationParts, durationSeconds, type DurationUnit } from './duration';
import type { SettingsCommittedResource, SettingsDraftRegistry } from './settings-drafts.svelte';
import type { SettingsJson, SettingsLocation, SettingsResource } from './settings-draft-storage';
import type {
  ConfigKey,
  ConfigPatch,
  ConfigValues,
  RootRuntimeSettings,
  RootRuntimeSettingsInput,
} from './types';

const DOCUMENT_KEYS = [
  'bot_config',
  'log_level',
  'reaction_poll_interval_seconds',
  'merge_after_ci_quiet_period_seconds',
  'path_index_interval_seconds',
  'session_ttl_seconds',
  'path_index_max_seconds',
] as const;
const DURATION_KEYS = [
  'reaction_poll_interval_seconds',
  'merge_after_ci_quiet_period_seconds',
  'path_index_interval_seconds',
  'session_ttl_seconds',
] as const;
const DURATION_VALUE_KEYS = ['override_seconds', 'editor'] as const;
const DURATION_EDITOR_KEYS = ['amount', 'unit'] as const;
const LOG_LEVELS = new Set(['debug', 'info', 'warn', 'error']);
const MAX_DURATION_AMOUNT_LENGTH = 32;

export type RuntimeDurationKey = (typeof DURATION_KEYS)[number];

export type RuntimeConfigDocument = Record<string, SettingsJson> & ConfigValues;

export interface RuntimeDurationEditor extends Record<string, SettingsJson> {
  amount: string;
  unit: DurationUnit;
}

export interface RuntimeDurationDraft extends Record<string, SettingsJson> {
  override_seconds: number | null;
  editor: RuntimeDurationEditor | null;
}

export interface RuntimeSettingsDraftDocument extends Record<string, SettingsJson> {
  bot_config: RuntimeConfigDocument | null;
  log_level: string | null;
  reaction_poll_interval_seconds: RuntimeDurationDraft;
  merge_after_ci_quiet_period_seconds: RuntimeDurationDraft;
  path_index_interval_seconds: RuntimeDurationDraft;
  session_ttl_seconds: RuntimeDurationDraft;
  path_index_max_seconds: number;
}

export type RuntimeSettingsControlId =
  `runtime.bot_config.${ConfigKey}` | 'runtime.log_level' | `runtime.${RuntimeDurationKey}`;

export interface RuntimeSettingsControlDefinition {
  id: RuntimeSettingsControlId;
  location: SettingsLocation;
}

export interface RuntimeDurationSpec {
  key: RuntimeDurationKey;
  units: readonly DurationUnit[];
  minimumSeconds: number;
  maximumSeconds: number;
  allowZero: boolean;
  problem: string;
}

export type RuntimeSettingsSerializationResult =
  | { ok: true; input: RootRuntimeSettingsInput }
  | { ok: false; controlId: RuntimeSettingsControlId; problem: string };

export const RUNTIME_RESOURCE = { type: 'runtime' } as const satisfies SettingsResource;
export const ROOT_SETTINGS_SCOPE = { type: 'root' } as const;

export const RUNTIME_DURATION_SPECS: Readonly<Record<RuntimeDurationKey, RuntimeDurationSpec>> = {
  reaction_poll_interval_seconds: {
    key: 'reaction_poll_interval_seconds',
    units: ['seconds', 'minutes', 'hours'],
    minimumSeconds: 1,
    maximumSeconds: 24 * 60 * 60,
    allowZero: true,
    problem: 'Reaction sweep interval must be off or between 1 second and 24 hours',
  },
  merge_after_ci_quiet_period_seconds: {
    key: 'merge_after_ci_quiet_period_seconds',
    units: ['seconds', 'minutes', 'hours'],
    minimumSeconds: 1,
    maximumSeconds: 24 * 60 * 60,
    allowZero: false,
    problem: 'Merge-after-CI quiet period must be between 1 second and 24 hours',
  },
  path_index_interval_seconds: {
    key: 'path_index_interval_seconds',
    units: ['minutes', 'hours', 'days'],
    minimumSeconds: 60,
    maximumSeconds: 7 * 24 * 60 * 60,
    allowZero: false,
    problem: 'Path index interval must be between 1 minute and the service ceiling',
  },
  session_ttl_seconds: {
    key: 'session_ttl_seconds',
    units: ['minutes', 'hours', 'days'],
    minimumSeconds: 60,
    maximumSeconds: 30 * 24 * 60 * 60,
    allowZero: false,
    problem: 'Session lifetime must be between 1 minute and 30 days',
  },
};

export function runtimeSettingsControls(): readonly RuntimeSettingsControlDefinition[] {
  return [
    ...CONFIG_KEYS.map((key): RuntimeSettingsControlDefinition => ({
      id: `runtime.bot_config.${key}`,
      location: { section: 'runtime', path: ['settings', 'behavior', key] },
    })),
    {
      id: 'runtime.log_level',
      location: { section: 'runtime', path: ['settings', 'runtime', 'log_level'] },
    },
    ...DURATION_KEYS.map((key): RuntimeSettingsControlDefinition => ({
      id: `runtime.${key}`,
      location: { section: 'runtime', path: ['settings', 'runtime', key] },
    })),
  ];
}

export function buildRuntimeSettingsDraftDocument(
  settings: RootRuntimeSettings,
): RuntimeSettingsDraftDocument {
  const document = parseRuntimeSettingsDraftDocument({
    bot_config: settings.behavior_defaults.override,
    log_level: settings.log_level.override,
    reaction_poll_interval_seconds: durationDraft(settings.reaction_poll_interval.override_seconds),
    merge_after_ci_quiet_period_seconds: durationDraft(
      settings.merge_after_ci_quiet_period.override_seconds,
    ),
    path_index_interval_seconds: durationDraft(settings.path_index_interval.override_seconds),
    session_ttl_seconds: durationDraft(settings.session_lifetime.override_seconds),
    path_index_max_seconds: settings.path_index_interval.max_seconds,
  });
  if (document === null) throw new TypeError('runtime settings contain invalid editable values');
  return document;
}

export function parseRuntimeSettingsDraftDocument(
  value: unknown,
): RuntimeSettingsDraftDocument | null {
  if (!isRecord(value) || !hasExactKeys(value, DOCUMENT_KEYS)) return null;
  const botConfig = parseConfig(value.bot_config);
  if (botConfig === undefined) return null;
  if (value.log_level !== null && !isLogLevel(value.log_level)) return null;
  if (
    typeof value.path_index_max_seconds !== 'number' ||
    !Number.isSafeInteger(value.path_index_max_seconds) ||
    value.path_index_max_seconds <= 0
  ) {
    return null;
  }

  const durations = Object.fromEntries(
    DURATION_KEYS.map((key) => [key, parseDurationDraft(value[key])]),
  ) as Record<RuntimeDurationKey, RuntimeDurationDraft | null>;
  if (DURATION_KEYS.some((key) => durations[key] === null)) return null;

  return {
    bot_config: botConfig,
    log_level: value.log_level,
    reaction_poll_interval_seconds: durations.reaction_poll_interval_seconds!,
    merge_after_ci_quiet_period_seconds: durations.merge_after_ci_quiet_period_seconds!,
    path_index_interval_seconds: durations.path_index_interval_seconds!,
    session_ttl_seconds: durations.session_ttl_seconds!,
    path_index_max_seconds: value.path_index_max_seconds,
  };
}

export function adoptRuntimeSettings(
  registry: SettingsDraftRegistry,
  settings: RootRuntimeSettings,
): boolean {
  return registry.adoptBase(
    RUNTIME_RESOURCE,
    settings.revision,
    buildRuntimeSettingsDraftDocument(settings),
  );
}

export function runtimeSettingsDraftDocument(
  registry: SettingsDraftRegistry,
  settings: RootRuntimeSettings,
): RuntimeSettingsDraftDocument {
  const stored = registry.value(RUNTIME_RESOURCE);
  if (stored === null) return buildRuntimeSettingsDraftDocument(settings);
  const parsed = parseRuntimeSettingsDraftDocument(stored);
  if (parsed === null) throw new TypeError('stored runtime settings draft is invalid');
  return parsed;
}

export function stageRuntimeSettingsControl(
  registry: SettingsDraftRegistry,
  settings: RootRuntimeSettings,
  nextValue: RuntimeSettingsDraftDocument,
  controlId: RuntimeSettingsControlId,
): boolean {
  const definition = runtimeSettingsControls().find(({ id }) => id === controlId);
  const next = parseRuntimeSettingsDraftDocument(nextValue);
  if (definition === undefined || next === null) return false;
  const snapshot = registry.resource(RUNTIME_RESOURCE);
  const base = parseRuntimeSettingsDraftDocument(
    snapshot?.base ?? buildRuntimeSettingsDraftDocument(settings),
  );
  if (base === null) return false;
  const saved = runtimeSettingsSavedControls(base, settings.behavior_defaults.deployment);
  const current = runtimeSettingsSavedControls(next, settings.behavior_defaults.deployment);

  return registry.stage(RUNTIME_RESOURCE, next, {
    id: controlId,
    location: definition.location,
    saved: saved[controlId]!,
    value: current[controlId]!,
  });
}

export function runtimeSettingsSavedControls(
  document: RuntimeSettingsDraftDocument,
  deployment: ConfigValues,
): Readonly<Record<RuntimeSettingsControlId, SettingsJson>> {
  const patch = runtimeConfigPatch(deployment, document.bot_config);
  const controls: Record<string, SettingsJson> = {
    'runtime.log_level': document.log_level,
  };
  for (const key of CONFIG_KEYS) {
    controls[`runtime.bot_config.${key}`] = patch[key] === undefined ? null : cloneJson(patch[key]);
  }
  for (const key of DURATION_KEYS) controls[`runtime.${key}`] = cloneJson(document[key]);
  return controls as Record<RuntimeSettingsControlId, SettingsJson>;
}

export function overlayRuntimeSettings(
  settings: RootRuntimeSettings,
  document: RuntimeSettingsDraftDocument,
): RootRuntimeSettings {
  const parsed = parseRuntimeSettingsDraftDocument(document);
  if (parsed === null) throw new TypeError('runtime settings draft is invalid');
  return {
    ...settings,
    behavior_defaults: { ...settings.behavior_defaults, override: parsed.bot_config },
    log_level: { ...settings.log_level, override: parsed.log_level },
    reaction_poll_interval: {
      ...settings.reaction_poll_interval,
      override_seconds: parsed.reaction_poll_interval_seconds.override_seconds,
    },
    merge_after_ci_quiet_period: {
      ...settings.merge_after_ci_quiet_period,
      override_seconds: parsed.merge_after_ci_quiet_period_seconds.override_seconds,
    },
    path_index_interval: {
      ...settings.path_index_interval,
      override_seconds: parsed.path_index_interval_seconds.override_seconds,
    },
    session_lifetime: {
      ...settings.session_lifetime,
      override_seconds: parsed.session_ttl_seconds.override_seconds,
    },
  };
}

export function runtimeDurationEditor(
  value: RuntimeDurationDraft,
  spec: RuntimeDurationSpec,
  fallbackSeconds: number,
): RuntimeDurationEditor {
  if (value.editor !== null) return { ...value.editor };
  const parts = durationParts(value.override_seconds ?? fallbackSeconds, spec.units);
  return { amount: String(parts.amount), unit: parts.unit };
}

export function runtimeDurationSeconds(
  value: RuntimeDurationDraft,
  spec: RuntimeDurationSpec,
  maximumSeconds = spec.maximumSeconds,
): number | null | undefined {
  if (value.editor === null) return value.override_seconds;
  if (value.editor.amount.trim() === '') return undefined;
  const amount = Number(value.editor.amount);
  const seconds = durationSeconds({ amount, unit: value.editor.unit });
  if (seconds === null) return undefined;
  if (seconds === 0 && spec.allowZero) return 0;
  if (seconds < spec.minimumSeconds || seconds > maximumSeconds) return undefined;
  return seconds;
}

export function serializeRuntimeSettingsDraft(
  expectedRevision: number,
  document: RuntimeSettingsDraftDocument,
): RuntimeSettingsSerializationResult {
  if (!Number.isSafeInteger(expectedRevision) || expectedRevision < 0) {
    return {
      ok: false,
      controlId: 'runtime.log_level',
      problem: 'Runtime settings revision is invalid',
    };
  }
  const parsed = parseRuntimeSettingsDraftDocument(document);
  if (parsed === null) {
    return {
      ok: false,
      controlId: 'runtime.log_level',
      problem: 'Runtime settings draft is invalid',
    };
  }
  const durations: Partial<Record<RuntimeDurationKey, number | null>> = {};
  for (const key of DURATION_KEYS) {
    const spec = RUNTIME_DURATION_SPECS[key];
    const maximum =
      key === 'path_index_interval_seconds' ? parsed.path_index_max_seconds : spec.maximumSeconds;
    const seconds = runtimeDurationSeconds(parsed[key], spec, maximum);
    if (seconds === undefined) {
      return { ok: false, controlId: `runtime.${key}`, problem: spec.problem };
    }
    durations[key] = seconds;
  }

  return {
    ok: true,
    input: {
      bot_config: parsed.bot_config,
      log_level: parsed.log_level,
      reaction_poll_interval_seconds: durations.reaction_poll_interval_seconds!,
      merge_after_ci_quiet_period_seconds: durations.merge_after_ci_quiet_period_seconds!,
      path_index_interval_seconds: durations.path_index_interval_seconds!,
      session_ttl_seconds: durations.session_ttl_seconds!,
      expected_revision: expectedRevision,
    },
  };
}

export function runtimeSettingsCommittedResource(
  settings: RootRuntimeSettings,
): SettingsCommittedResource {
  const value = buildRuntimeSettingsDraftDocument(settings);
  return {
    resource: RUNTIME_RESOURCE,
    revision: settings.revision,
    value,
    savedControls: runtimeSettingsSavedControls(value, settings.behavior_defaults.deployment),
  };
}

export function runtimeConfigPatch(
  deployment: ConfigValues,
  override: RuntimeConfigDocument | ConfigValues | null,
): ConfigPatch {
  if (override === null) return {};
  return Object.fromEntries(
    CONFIG_KEYS.flatMap((key) =>
      sameJson(override[key], deployment[key]) ? [] : [[key, cloneJson(override[key])]],
    ),
  ) as ConfigPatch;
}

export function applyRuntimeConfigPatch(
  deployment: ConfigValues,
  patch: ConfigPatch,
): RuntimeConfigDocument | null {
  if (Object.keys(patch).length === 0) return null;
  return cloneJson({ ...deployment, ...patch }) as RuntimeConfigDocument;
}

function durationDraft(seconds: number | null): RuntimeDurationDraft {
  return { override_seconds: seconds, editor: null };
}

function parseDurationDraft(value: unknown): RuntimeDurationDraft | null {
  if (!isRecord(value) || !hasExactKeys(value, DURATION_VALUE_KEYS)) return null;
  if (
    value.override_seconds !== null &&
    (typeof value.override_seconds !== 'number' ||
      !Number.isSafeInteger(value.override_seconds) ||
      value.override_seconds < 0)
  ) {
    return null;
  }
  const editor = parseDurationEditor(value.editor);
  if (editor === undefined) return null;
  return { override_seconds: value.override_seconds, editor };
}

function parseDurationEditor(value: unknown): RuntimeDurationEditor | null | undefined {
  if (value === null) return null;
  if (!isRecord(value) || !hasExactKeys(value, DURATION_EDITOR_KEYS)) return undefined;
  if (
    typeof value.amount !== 'string' ||
    value.amount.length > MAX_DURATION_AMOUNT_LENGTH ||
    !isDurationUnit(value.unit)
  ) {
    return undefined;
  }
  return { amount: value.amount, unit: value.unit };
}

function parseConfig(value: unknown): RuntimeConfigDocument | null | undefined {
  if (value === null) return null;
  if (!isRecord(value) || CONFIG_KEYS.some((key) => !validConfigValue(key, value[key]))) {
    return undefined;
  }
  if (!Object.values(value).every(isSettingsJson)) return undefined;
  return cloneJson(value) as RuntimeConfigDocument;
}

function validConfigValue(key: ConfigKey, value: unknown): boolean {
  if (key === 'allowed_commands') return Array.isArray(value) && value.every(isString);
  if (key === 'command_aliases') {
    return isRecord(value) && Object.values(value).every(isString);
  }
  if (key === 'command_prefix') return typeof value === 'string';
  return typeof value === 'boolean';
}

function hasExactKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === 'string';
}

function isLogLevel(value: unknown): value is string {
  return typeof value === 'string' && LOG_LEVELS.has(value);
}

function isDurationUnit(value: unknown): value is DurationUnit {
  return value === 'seconds' || value === 'minutes' || value === 'hours' || value === 'days';
}

function isSettingsJson(value: unknown): value is SettingsJson {
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return true;
  if (typeof value === 'number') return Number.isFinite(value);
  if (Array.isArray(value)) return value.every(isSettingsJson);
  return isRecord(value) && Object.values(value).every(isSettingsJson);
}

function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function sameJson(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}
