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
  const normalized = { ...value };
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
  if (!Object.hasOwn(normalized, 'formatting')) normalized.formatting = defaultFormattingPolicy();
  const policy = normalized.formatting;
  if (
    isRecord(policy) &&
    isRecord(policy.common) &&
    !Object.hasOwn(policy.common, 'inline_max_chars')
  ) {
    normalized.formatting = { ...policy, common: { ...policy.common, inline_max_chars: 0 } };
  }
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
