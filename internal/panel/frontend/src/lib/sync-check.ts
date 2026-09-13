import { SYNC_KINDS, type SyncCheckOutcome, type SyncCheckObservation } from './types';

export const CHECK_OBSERVATION_LABELS: Record<SyncCheckObservation['outcome'], string> = {
  matched: 'Matched saved settings',
  different: 'Differences found',
  failed: 'Could not check',
  blocked: 'Could not proceed',
  proposed: 'Open proposal',
  declined: 'Declined proposal',
  applied: 'Changes applied',
  '': 'Not confirmed',
};

/** Queue details can predate this contract. Never turn missing or malformed evidence into a match. */
export function checkOutcome(value: unknown): SyncCheckOutcome | null {
  if (!value || typeof value !== 'object') return null;
  const held = value as Record<string, unknown>;
  if (
    typeof held.completed_at !== 'string' ||
    !Number.isFinite(Date.parse(held.completed_at)) ||
    !['checked', 'disabled', 'unpermitted', 'deferred'].includes(String(held.disposition)) ||
    typeof held.summary !== 'string' ||
    !held.summary.trim() ||
    (held.blocking_plan_id !== undefined &&
      (held.disposition !== 'deferred' ||
        typeof held.blocking_plan_id !== 'string' ||
        !held.blocking_plan_id.trim() ||
        held.blocking_plan_id !== held.blocking_plan_id.trim())) ||
    typeof held.cached !== 'number' ||
    !Number.isSafeInteger(held.cached) ||
    held.cached < 0 ||
    !held.counts ||
    typeof held.counts !== 'object' ||
    Array.isArray(held.counts) ||
    !Object.entries(held.counts).every(
      ([key, count]) =>
        Object.hasOwn(CHECK_OBSERVATION_LABELS, key) &&
        typeof count === 'number' &&
        Number.isSafeInteger(count) &&
        count >= 0,
    ) ||
    !(
      held.missing_permissions === null ||
      (Array.isArray(held.missing_permissions) &&
        held.missing_permissions.every((kind) => SYNC_KINDS.some((known) => known === kind)))
    )
  )
    return null;
  return held as unknown as SyncCheckOutcome;
}

export function checkOutcomeTitle(outcome: SyncCheckOutcome): string {
  if (outcome.disposition === 'disabled') return 'Sync was switched off';
  if (outcome.disposition === 'unpermitted') return 'Check needs GitHub permissions';
  if (outcome.disposition === 'deferred') return 'Check deferred';
  if (
    (outcome.counts.failed ?? 0) + (outcome.counts.blocked ?? 0) > 0 ||
    (outcome.missing_permissions?.length ?? 0) > 0
  )
    return 'Check finished with gaps';
  if ((outcome.counts.different ?? 0) > 0) return 'Check found differences';
  return 'Check finished';
}
