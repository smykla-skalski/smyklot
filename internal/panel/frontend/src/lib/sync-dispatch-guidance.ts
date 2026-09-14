import type { SyncPlan } from './types';

export function syncDispatchGuidance(plan: SyncPlan): string | null {
  const capability = plan.dispatch;
  if (!capability || capability.plan_id !== plan.id)
    return 'Current run availability is unavailable. Reload these changes before requesting a run.';
  switch (capability.reason) {
    case 'available':
      return syncDispatchIntent(plan)
        ? null
        : 'Current run availability is unavailable. Reload these changes before requesting a run.';
    case 'admin_or_owner_required':
      return 'An Admin or Owner can request that these changes run now.';
    case 'approval_required':
      return 'Review and approve these changes before requesting a run.';
    case 'already_running':
      return 'These changes are already running. Their result will update here.';
    case 'plan_expired':
      return 'These changes have expired. Check repositories again to prepare current changes.';
    case 'plan_changed':
      return 'The saved configuration changed. Check repositories again to prepare current changes.';
    case 'plan_finished':
      return 'These changes have finished or were discarded. A new check prepares current changes.';
    case 'queue_unavailable':
      return 'The execution record is unavailable. Reload to check its current state.';
    case 'queue_finished':
      return 'This execution has finished. Check repositories again to prepare current changes.';
    default:
      return 'These changes cannot run in their current state. Reload to check for an update.';
  }
}

export function syncDispatchIntent(plan: SyncPlan) {
  const c = plan.dispatch;
  return c?.available &&
    c.action === 'dispatch' &&
    c.effect === 'schedule_reviewed_changes_without_waiting_for_window' &&
    c.reason === 'available' &&
    c.plan_id === plan.id &&
    c.queue_id &&
    Number.isSafeInteger(c.expected_revision) &&
    c.expected_revision! > 0
    ? { action: 'dispatch' as const, plan_id: c.plan_id, expected_revision: c.expected_revision! }
    : null;
}

/** Current execution claims must not outlive the evidence that supports them. */
export function syncPlanExecutionProblem(plan: SyncPlan): string | null {
  if (!['approved', 'computed'].includes(plan.state)) return null;
  const reason = plan.dispatch?.plan_id === plan.id ? plan.dispatch.reason : undefined;
  switch (reason) {
    case 'plan_expired':
      return 'These changes have expired';
    case 'plan_changed':
      return 'These changes are out of date';
    case 'plan_finished':
    case 'queue_finished':
      return 'This execution has finished';
    case 'queue_unavailable':
      return 'Execution status unavailable';
    case 'state_unsupported':
      return 'Execution status needs an update';
    default:
      return plan.state === 'approved' && !plan.queue_item ? 'Execution status unavailable' : null;
  }
}
