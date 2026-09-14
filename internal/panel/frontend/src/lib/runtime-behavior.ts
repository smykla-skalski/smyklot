import { CONFIG_KEYS } from './config';
import {
  applyFormattingPatch,
  completeFormattingPatch,
  defaultFormattingPolicy,
  formattingOverrideCount,
  formattingValidationField,
  parseFormattingPatch,
  parseFormattingPolicy,
} from './formatting';
import type { ConfigKey, ConfigPatch, ConfigValues } from './types';

export class RuntimeBehaviorValidationError extends TypeError {
  constructor(
    readonly field: string,
    message: string,
  ) {
    super(message);
  }
}

/** Presence owns a field, even when its value equals the deployment default. */
export interface RuntimeBehaviorIntent {
  version: 1;
  overrides: ConfigPatch;
}

/** Decode persisted editor/API intent, including complete documents from older panels. */
export function parseRuntimeBehavior(value: unknown): RuntimeBehaviorIntent | null {
  if (value === null) return null;
  if (!isRecord(value)) throw new TypeError('bot_config must be an object or null');
  if (!Object.hasOwn(value, 'version')) return legacyRuntimeBehavior(value);
  if (value.version !== 1 || Object.keys(value).length !== 2 || !isRecord(value.overrides)) {
    throw new TypeError('bot_config requires version 1 and an overrides object');
  }
  return runtimeBehaviorFromPatch(value.overrides);
}

/** Validate and copy a sparse patch. Empty formatting groups do not own fields. */
export function runtimeBehaviorFromPatch(value: unknown): RuntimeBehaviorIntent | null {
  if (!isRecord(value)) throw new TypeError('bot_config.overrides must be an object');
  const overrides: ConfigPatch = {};
  for (const [key, field] of Object.entries(value)) {
    if (key === 'formatting') {
      const formatting = parseFormattingPatch(field);
      if (formatting === null)
        throw new RuntimeBehaviorValidationError(
          `bot_config.${formattingValidationField(field) ?? 'formatting'}`,
          'bot_config.overrides.formatting is invalid',
        );
      if (formattingOverrideCount(formatting) > 0) overrides.formatting = formatting;
      continue;
    }
    if (!CONFIG_KEYS.includes(key as ConfigKey)) {
      throw new RuntimeBehaviorValidationError(
        `bot_config.${key}`,
        `bot_config.overrides.${key} is not a runtime setting`,
      );
    }
    if (!validConfigValue(key as ConfigKey, field)) {
      throw new RuntimeBehaviorValidationError(
        `bot_config.${key}`,
        `bot_config.overrides.${key} must have a valid value or be omitted to inherit`,
      );
    }
    Object.assign(overrides, { [key]: cloneValue(field) });
  }
  return Object.keys(overrides).length === 0 ? null : { version: 1, overrides };
}

export function resolveRuntimeBehavior(
  deployment: ConfigValues,
  intent: RuntimeBehaviorIntent | null,
): ConfigValues {
  const resolved = cloneValue(deployment);
  if (intent === null) return resolved;
  for (const key of CONFIG_KEYS) {
    const value = intent.overrides[key];
    if (value !== undefined) Object.assign(resolved, { [key]: cloneValue(value) });
  }
  if (intent.overrides.formatting !== undefined) {
    resolved.formatting = applyFormattingPatch(resolved.formatting, intent.overrides.formatting);
  }
  return resolved;
}

function legacyRuntimeBehavior(value: Record<string, unknown>): RuntimeBehaviorIntent {
  const normalized = legacyBehaviorFields(value);
  // Historical concrete Go configs pin zero values for omitted behavior fields.
  // This is migration behavior only; versioned overrides remain strict and sparse.
  for (const key of CONFIG_KEYS) {
    if (normalized[key] === undefined || normalized[key] === null) {
      normalized[key] =
        key === 'allowed_commands'
          ? []
          : key === 'command_aliases'
            ? {}
            : key === 'command_prefix'
              ? ''
              : false;
    }
  }
  if (
    normalized.runner !== undefined &&
    normalized.runner !== null &&
    normalized.runner !== '' &&
    normalized.runner !== 'service' &&
    normalized.runner !== 'action'
  ) {
    throw new TypeError('bot_config.runner is invalid');
  }
  normalized.formatting = legacyFormatting(value);
  const patch: Record<string, unknown> = {};
  for (const key of CONFIG_KEYS) {
    if (!validConfigValue(key, normalized[key]))
      throw new RuntimeBehaviorValidationError(`bot_config.${key}`, `bot_config.${key} is invalid`);
    patch[key] = normalized[key];
  }
  const formatting = parseFormattingPolicy(normalized.formatting);
  if (formatting === null)
    throw new RuntimeBehaviorValidationError(
      `bot_config.${formattingValidationField(normalized.formatting, true) ?? 'formatting'}`,
      'bot_config.formatting is invalid',
    );
  patch.formatting = completeFormattingPatch(formatting);
  // Every legacy config owns its complete field set, including false and empty values.
  return runtimeBehaviorFromPatch(patch)!;
}

/** Historical Config decoding merges concrete structs and ignores null scalars. */
function legacyFormatting(value: Record<string, unknown>): Record<string, unknown> {
  const defaults = defaultFormattingPolicy();
  const zero = (node: unknown): unknown =>
    isRecord(node)
      ? Object.fromEntries(Object.entries(node).map(([key, child]) => [key, zero(child)]))
      : typeof node === 'number'
        ? 0
        : '';
  // Production initializes defaults only for records without the exact old tag.
  const policy = (Object.hasOwn(value, 'formatting') ? zero(defaults) : defaults) as Record<
    string,
    unknown
  >;
  for (const [key, field] of Object.entries(value)) {
    if (key.toLowerCase() === 'formatting')
      mergeLegacyFormatting(policy, field, 'bot_config.formatting');
  }
  return policy;
}

function mergeLegacyFormatting(
  target: Record<string, unknown>,
  value: unknown,
  path: string,
): void {
  if (value === null) return;
  if (!isRecord(value)) throw new RuntimeBehaviorValidationError(path, `${path} is invalid`);
  for (const [name, field] of Object.entries(value)) {
    const key = name.toLowerCase();
    if (!Object.hasOwn(target, key) || field === null) continue;
    const current = target[key];
    const fieldPath = `${path}.${key}`;
    if (isRecord(current)) {
      mergeLegacyFormatting(current, field, fieldPath);
    } else if (
      typeof field !== typeof current ||
      (typeof field === 'number' && !Number.isInteger(field))
    ) {
      throw new RuntimeBehaviorValidationError(fieldPath, `${fieldPath} is invalid`);
    } else {
      target[key] = field;
    }
  }
}

/** Match encoding/json's historical field folding and repeated-field updates. */
function legacyBehaviorFields(value: Record<string, unknown>): Record<string, unknown> {
  const normalized: Record<string, unknown> = Object.create(null);
  for (const [name, raw] of Object.entries(value)) {
    const key = name.toLowerCase();
    if (!CONFIG_KEYS.includes(key as ConfigKey) && key !== 'runner') {
      normalized[name] = raw;
      continue;
    }
    // Null leaves concrete Go scalars unchanged, but clears slices and maps.
    if (raw === null && key !== 'allowed_commands' && key !== 'command_aliases') continue;
    let field = raw;
    if (key === 'allowed_commands' && Array.isArray(field)) {
      field = field.map((entry) => (entry === null ? '' : entry));
    }
    if (key === 'command_aliases' && isRecord(field)) {
      field = Object.fromEntries(
        Object.entries(field).map(([alias, entry]) => [alias, entry === null ? '' : entry]),
      );
    }
    // Go retains a type error even if a later differently-cased field is valid.
    if (
      field !== null &&
      (key === 'runner' ? typeof field !== 'string' : !validConfigValue(key as ConfigKey, field))
    ) {
      throw new RuntimeBehaviorValidationError(`bot_config.${key}`, `bot_config.${key} is invalid`);
    }
    if (key === 'command_aliases' && isRecord(field) && isRecord(normalized[key])) {
      normalized[key] = { ...normalized[key], ...field };
    } else {
      normalized[key] = field;
    }
  }
  return normalized;
}

function validConfigValue(key: ConfigKey, value: unknown): boolean {
  if (key === 'allowed_commands') return Array.isArray(value) && value.every(isString);
  if (key === 'command_aliases') return isRecord(value) && Object.values(value).every(isString);
  if (key === 'command_prefix') return isString(value);
  return typeof value === 'boolean';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === 'string';
}

// Svelte may pass a reactive proxy; copying values avoids structuredClone's proxy rejection.
function cloneValue<T>(value: T): T {
  if (Array.isArray(value)) return value.map(cloneValue) as T;
  if (!isRecord(value)) return value;
  return Object.fromEntries(
    Object.entries(value).map(([key, field]) => [key, cloneValue(field)]),
  ) as T;
}
