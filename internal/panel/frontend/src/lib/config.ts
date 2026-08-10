import type { ConfigKey, ConfigPatch, ConfigValues } from './types';

export interface BooleanField {
  key: Exclude<ConfigKey, 'allowed_commands' | 'command_aliases' | 'command_prefix'>;
  label: string;
  help: string;
  /**
   * Whether the raw config value already means "the feature is active". The
   * quiet_* and disable_* keys store suppression, so their display value is
   * the inverse of the stored boolean.
   */
  positive: boolean;
}

export const BOOLEAN_FIELDS: ReadonlyArray<BooleanField> = [
  {
    key: 'quiet_success',
    label: 'Success replies',
    help: 'When enabled, successful commands get a reply comment. When disabled, only the emoji reaction is posted. Errors always post comments',
    positive: false,
  },
  {
    key: 'quiet_reactions',
    label: 'Reaction-command replies',
    help: 'When enabled, commands invoked by emoji reactions get a confirmation comment',
    positive: false,
  },
  {
    key: 'quiet_pending',
    label: 'Pending-check notices',
    help: 'When enabled, Smyklot posts a notice when a merge is waiting on required checks',
    positive: false,
  },
  {
    key: 'disable_mentions',
    label: 'Mention commands',
    help: 'When enabled, @smyklot mentions invoke commands. When disabled, only the command prefix works',
    positive: false,
  },
  {
    key: 'disable_bare_commands',
    label: 'Bare commands',
    help: 'When enabled, a bare command word like "approve" works on its own, without prefix or mention',
    positive: false,
  },
  {
    key: 'disable_unapprove',
    label: 'Unapprove command',
    help: 'When disabled, unapprove (and its synonym disapprove) is ignored even when the command is otherwise allowed',
    positive: false,
  },
  {
    key: 'disable_reactions',
    label: 'Reaction triggers',
    help: 'When enabled, emoji reactions can invoke Smyklot actions. Comment commands always work',
    positive: false,
  },
  {
    key: 'disable_deleted_comments',
    label: 'Deletion notices',
    help: 'When enabled, Smyklot announces when a processed command comment is deleted',
    positive: false,
  },
  {
    key: 'allow_self_approval',
    label: 'Self-approval',
    help: 'When enabled, pull request authors may approve their own changes through Smyklot',
    positive: true,
  },
];

/** Whether the feature a field describes is active for a given raw config value. */
export function fieldEnabled(field: BooleanField, raw: boolean): boolean {
  return field.positive ? raw : !raw;
}

/** The raw config value that makes a field's feature enabled or disabled. */
export function fieldRawValue(field: BooleanField, enabled: boolean): boolean {
  return field.positive ? enabled : !enabled;
}

export function clonePatch(patch: ConfigPatch): ConfigPatch {
  return JSON.parse(JSON.stringify(patch)) as ConfigPatch;
}

export function countOverrides(patch: ConfigPatch): number {
  return Object.keys(patch).length;
}

export function patchesEqual(left: ConfigPatch, right: ConfigPatch): boolean {
  return JSON.stringify(sortPatch(left)) === JSON.stringify(sortPatch(right));
}

export function reconcilePatchDraft(
  draft: ConfigPatch,
  previousPatch: ConfigPatch,
  nextPatch: ConfigPatch,
): ConfigPatch {
  return patchesEqual(previousPatch, nextPatch) ? draft : clonePatch(nextPatch);
}

function sortPatch(patch: ConfigPatch): ConfigPatch {
  const sorted: Record<string, unknown> = {};
  for (const key of Object.keys(patch).sort()) {
    const value = patch[key as ConfigKey];
    if (Array.isArray(value)) {
      sorted[key] = [...value].sort();
    } else if (typeof value === 'object' && value !== null) {
      sorted[key] = Object.fromEntries(
        Object.entries(value).sort(([a], [b]) => a.localeCompare(b)),
      );
    } else {
      sorted[key] = value;
    }
  }
  return sorted as ConfigPatch;
}

export function effectiveValue<K extends ConfigKey>(
  patch: ConfigPatch,
  effective: ConfigValues,
  key: K,
): ConfigValues[K] {
  return (patch[key] ?? effective[key]) as ConfigValues[K];
}

/** An empty allowlist is Smyklot's compact representation for every command. */
export function commandIsAllowed(allowedCommands: readonly string[], command: string): boolean {
  return allowedCommands.length === 0 || allowedCommands.includes(command);
}

/**
 * Toggle a command in the semantic selection while preserving Smyklot's compact
 * empty-list representation for "all commands". At least one command remains
 * enabled because an empty selection cannot represent "none" in Config.
 */
export function toggleAllowedCommand<T extends string>(
  allowedCommands: readonly T[],
  command: T,
  availableCommands: readonly T[],
): T[] {
  const selected =
    allowedCommands.length === 0
      ? [...availableCommands]
      : availableCommands.filter((candidate) => allowedCommands.includes(candidate));

  if (selected.includes(command)) {
    return selected.length === 1 ? selected : selected.filter((candidate) => candidate !== command);
  }

  const next = availableCommands.filter(
    (candidate) => selected.includes(candidate) || candidate === command,
  );
  return next.length === availableCommands.length ? [] : next;
}

export function updatePatchValue<K extends ConfigKey>(
  patch: ConfigPatch,
  inherited: ConfigValues,
  key: K,
  value: ConfigValues[K],
): ConfigPatch {
  const next = clonePatch(patch);
  if (configValueEqual(value, inherited[key])) {
    delete next[key];
  } else {
    Object.assign(next, { [key]: cloneValue(value) });
  }
  return next;
}

export function setExplicitPatchValue<K extends ConfigKey>(
  patch: ConfigPatch,
  key: K,
  value: ConfigValues[K],
): ConfigPatch {
  return { ...clonePatch(patch), [key]: cloneValue(value) };
}

function configValueEqual(left: ConfigValues[ConfigKey], right: ConfigValues[ConfigKey]): boolean {
  if (Array.isArray(left) && Array.isArray(right)) {
    return JSON.stringify([...left].sort()) === JSON.stringify([...right].sort());
  }
  if (isAliasMap(left) && isAliasMap(right)) {
    const entries = (value: Record<string, string>): Array<[string, string]> =>
      Object.entries(value).sort(([a], [b]) => a.localeCompare(b));
    return JSON.stringify(entries(left)) === JSON.stringify(entries(right));
  }
  return left === right;
}

function isAliasMap(value: ConfigValues[ConfigKey]): value is Record<string, string> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function cloneValue<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}
