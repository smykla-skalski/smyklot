import type { QueueItem, SyncPlan, SyncDispatchCapability } from '../src/lib/types.js';
import type { MockState } from './fixtures.js';

export function mockDispatchCapability(
  plan: SyncPlan,
  targetId: string,
  item: QueueItem | undefined,
  role: string,
  now: number,
): SyncDispatchCapability {
  const result: SyncDispatchCapability = {
    action: 'dispatch',
    available: false,
    reason: 'state_unsupported',
    effect: 'schedule_reviewed_changes_without_waiting_for_window',
    plan_id: plan.id,
  };
  if (!['admin', 'owner'].includes(role)) return { ...result, reason: 'admin_or_owner_required' };
  let reason: SyncDispatchCapability['reason'];
  switch (plan.state) {
    case 'applying':
      reason = 'already_running';
      break;
    case 'stale':
      reason = 'plan_changed';
      break;
    case 'expired':
      reason = 'plan_expired';
      break;
    case 'applied':
    case 'failed':
    case 'discarded':
      reason = 'plan_finished';
      break;
    case 'computed':
    case 'approved':
      if (!(Date.parse(plan.expires_at) > now)) reason = 'plan_expired';
      else if (plan.state === 'computed') reason = 'approval_required';
      else if (
        !item ||
        item.target_id !== targetId ||
        item.source_id !== plan.id ||
        item.source_kind !== 'sync_plan' ||
        item.kind !== 'sync_apply'
      )
        reason = 'queue_unavailable';
      else if (item.state === 'running') reason = 'already_running';
      else if (['succeeded', 'failed', 'cancelled', 'superseded'].includes(item.state))
        reason = 'queue_finished';
      else if (['scheduled', 'blocked', 'ready', 'retrying'].includes(item.state))
        reason = 'available';
      else reason = 'state_unsupported';
      break;
    default:
      reason = 'state_unsupported';
  }
  return {
    ...result,
    reason,
    available: reason === 'available',
    ...(reason === 'available' && item
      ? { queue_id: item.id, expected_revision: item.revision }
      : {}),
  };
}

/** Capabilities are current projections, never facts retained in history. */
export function projectMockSyncPlan(
  state: Pick<MockState, 'queue' | 'targets'>,
  targetId: string,
  plan: SyncPlan,
  now = Date.now(),
): SyncPlan {
  const item = state.queue.find(
    (q) =>
      q.target_id === targetId &&
      q.source_id === plan.id &&
      q.source_kind === 'sync_plan' &&
      q.kind === 'sync_apply',
  );
  const role = state.targets.find((t) => t.value.id === targetId)?.value.effective_role ?? 'none';
  return {
    ...plan,
    queue_item: item,
    dispatch: mockDispatchCapability(plan, targetId, item, role, now),
  };
}
