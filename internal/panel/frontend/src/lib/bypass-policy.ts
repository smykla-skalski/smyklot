import { numericValue } from './merge.js';
import type { SettingsJson } from './settings-draft-storage.js';
import type { BypassActorIdentity, SyncRulesetBypassActor } from './types.js';

export type BypassPolicyDocument = Record<string, SettingsJson> & {
  allow: boolean;
  actors: (Record<string, SettingsJson> & SyncRulesetBypassActor)[];
};

export const BYPASS_ACTOR_TYPES = [
  { value: 'Integration', label: 'App' },
  { value: 'Team', label: 'Team' },
  { value: 'User', label: 'Person' },
  { value: 'RepositoryRole', label: 'Repository role' },
  { value: 'OrganizationAdmin', label: 'Organization admin' },
  { value: 'DeployKey', label: 'Deploy keys' },
] as const;

export const BYPASS_MODES = [
  { value: 'always', label: 'Always allow' },
  { value: 'pull_request', label: 'Pull requests only' },
  { value: 'exempt', label: 'Exempt from rules' },
] as const;

export const BYPASS_ROLES = [
  { value: 5, label: 'Repository admin' },
  { value: 4, label: 'Maintainers' },
  { value: 2, label: 'Writers' },
] as const;

export function bypassActorKey(
  actor: Pick<SyncRulesetBypassActor, 'actor_type' | 'actor_id'>,
): string {
  return `${actor.actor_type}:${numericValue(actor.actor_id) ?? 'unknown'}`;
}

export function bypassActorName(
  actor: SyncRulesetBypassActor,
  identities: BypassActorIdentity[],
): string {
  const identity = identities.find((item) => bypassActorKey(item) === bypassActorKey(actor));
  if (identity) return identity.name;
  if (actor.actor_type === 'OrganizationAdmin') return 'Organization admin';
  if (actor.actor_type === 'DeployKey') return 'Deploy keys';
  if (actor.actor_type === 'RepositoryRole') {
    return (
      BYPASS_ROLES.find((role) => role.value === actor.actor_id)?.label ?? 'Custom repository role'
    );
  }
  return actor.actor_type === 'Integration'
    ? 'Unavailable app'
    : actor.actor_type === 'Team'
      ? 'Unavailable team'
      : 'Unavailable person';
}

export function parseBypassPolicy(value: unknown): BypassPolicyDocument | null | undefined {
  if (value === null) return null;
  if (
    !record(value) ||
    Object.keys(value).length !== 2 ||
    typeof value.allow !== 'boolean' ||
    !Array.isArray(value.actors)
  )
    return undefined;
  const actors: BypassPolicyDocument['actors'] = [];
  const keys = new Set<string>();
  for (const actor of value.actors) {
    if (
      !record(actor) ||
      Object.keys(actor).length !== 3 ||
      !BYPASS_ACTOR_TYPES.some((type) => type.value === actor.actor_type) ||
      !BYPASS_MODES.some((mode) => mode.value === actor.bypass_mode)
    )
      return undefined;
    if (
      typeof actor.actor_id !== 'number' ||
      !Number.isSafeInteger(actor.actor_id) ||
      actor.actor_id < 0
    )
      return undefined;
    const noIdentity = actor.actor_type === 'OrganizationAdmin' || actor.actor_type === 'DeployKey';
    if (!noIdentity && actor.actor_id === 0) return undefined;
    if (actor.actor_type === 'DeployKey' && actor.bypass_mode === 'pull_request') return undefined;
    const parsed = {
      actor_id: noIdentity ? 0 : actor.actor_id,
      actor_type: String(actor.actor_type),
      bypass_mode: String(actor.bypass_mode),
    };
    const key = bypassActorKey(parsed);
    if (keys.has(key)) return undefined;
    keys.add(key);
    actors.push(parsed);
  }
  return { allow: value.allow, actors };
}

/** Compare permissions without changing the saved or displayed actor order. */
export function sameBypassPolicy(
  left: BypassPolicyDocument | null,
  right: BypassPolicyDocument | null,
): boolean {
  if (left === null || right === null) return left === right;
  const actors = (policy: BypassPolicyDocument) =>
    policy.actors
      .map((actor) => JSON.stringify([actor.actor_type, actor.actor_id, actor.bypass_mode]))
      .sort();
  return (
    left.allow === right.allow && JSON.stringify(actors(left)) === JSON.stringify(actors(right))
  );
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function bypassInstallationLabel(identity: BypassActorIdentity | undefined): string {
  switch (identity?.installation_status) {
    case 'all_repositories':
      return 'All repositories';
    case 'selected_repositories':
      return 'Repository access unverified';
    case 'not_installed':
      return 'Not installed';
    case 'suspended':
      return 'Installation suspended';
    default:
      return 'Installation unverified';
  }
}
