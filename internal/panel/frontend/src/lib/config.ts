import type { ConfigKey, ConfigPatch, ConfigValues } from './types';

export const BOOLEAN_FIELDS: ReadonlyArray<{
  key: Exclude<ConfigKey, 'allowed_commands' | 'command_aliases' | 'command_prefix'>;
  label: string;
  help: string;
}> = [
  {
    key: 'quiet_success',
    label: 'Quiet successful commands',
    help: 'When On, successful comment commands finish silently instead of posting a success response. Errors are still reported',
  },
  {
    key: 'quiet_reactions',
    label: 'Quiet reaction commands',
    help: 'When On, successful reaction commands finish silently instead of posting a success response. Errors are still reported',
  },
  {
    key: 'quiet_pending',
    label: 'Quiet pending checks',
    help: 'When On, Smyklot does not post a message while required checks are still pending',
  },
  {
    key: 'disable_mentions',
    label: 'Disable mentions',
    help: 'When On, @smyklot mentions do not invoke commands. Use the configured command prefix instead',
  },
  {
    key: 'disable_bare_commands',
    label: 'Disable bare commands',
    help: 'When On, bare command words are ignored. Commands must begin with the configured prefix or an enabled mention',
  },
  {
    key: 'disable_unapprove',
    label: 'Disable unapprove',
    help: 'When On, unapprove and disapprove commands are ignored even when those commands are otherwise allowed',
  },
  {
    key: 'disable_reactions',
    label: 'Disable reactions',
    help: 'When On, emoji reactions cannot invoke Smyklot actions. Comment commands continue to work',
  },
  {
    key: 'disable_deleted_comments',
    label: 'Disable deletion notices',
    help: 'When On, Smyklot does not announce that a previously processed command comment was deleted',
  },
  {
    key: 'allow_self_approval',
    label: 'Allow self-approval',
    help: 'When On, pull request authors may use Smyklot to approve their own changes',
  },
];

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
