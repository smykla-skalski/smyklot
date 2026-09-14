import type { SyncCheckCapability } from './types';

export function syncCheckIntent(capability: SyncCheckCapability | undefined, targetId: string) {
  return capability?.action === 'check' &&
    capability.target_id === targetId &&
    targetId !== '' &&
    capability.effect === 'request_repository_check' &&
    capability.available &&
    capability.reason === 'available'
    ? { action: 'check' as const }
    : null;
}

export function syncCheckGuidance(
  capability: SyncCheckCapability | undefined,
  targetId: string,
): string {
  if (
    !capability ||
    capability.target_id !== targetId ||
    capability.action !== 'check' ||
    capability.effect !== 'request_repository_check'
  )
    return 'Refresh status to check whether a new repository check is available.';
  if (syncCheckIntent(capability, targetId))
    return 'Check repositories against your saved configuration. Automatic sync settings still apply.';
  switch (capability.reason) {
    case 'admin_or_owner_required':
      return 'An Admin or Owner can request a repository check.';
    case 'changes_pending':
      return 'Review the current changes before requesting another check.';
    case 'changes_running':
      return 'Current changes are running. Wait for their result before requesting another check.';
    case 'check_running':
      return 'A repository check is already running. Open it to follow its result.';
    default:
      return 'Refresh status to check whether a new repository check is available.';
  }
}

export function syncCheckBlocker(capability: SyncCheckCapability | undefined, targetId: string) {
  if (
    !capability ||
    !targetId ||
    capability.target_id !== targetId ||
    capability.action !== 'check' ||
    capability.effect !== 'request_repository_check' ||
    capability.available
  )
    return null;
  if (
    ['changes_pending', 'changes_running'].includes(capability.reason) &&
    capability.blocking_plan_id
  )
    return { kind: 'plan' as const, id: capability.blocking_plan_id };
  if (capability.reason === 'check_running' && capability.running_check_id)
    return { kind: 'check' as const, id: capability.running_check_id };
  return null;
}
