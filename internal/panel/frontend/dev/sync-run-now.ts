import { projectMockSyncPlan } from './sync-capability.js';
import { recordMockSyncEvent } from './sync-queue.js';
import { randomUUID } from 'node:crypto';
import { VIEWER, type MockState } from './fixtures.js';
import type {
  QueueItem,
  SyncPlan,
  SyncRunNowResponse,
  SyncDispatchCapability,
} from '../src/lib/types.js';

type State = Pick<
  MockState,
  | 'syncHistory'
  | 'targets'
  | 'queue'
  | 'syncPlans'
  | 'syncQueueEvents'
  | 'syncCheckReceipts'
  | 'syncDispatchReceipts'
>;
type Reply =
  | { status: 200 | 202; body: SyncRunNowResponse }
  | {
      status: 400 | 404 | 409;
      body: { code: string; message: string; dispatch?: SyncDispatchCapability };
    };
const terminal = new Set(['succeeded', 'failed', 'cancelled', 'superseded']);

/** Project the current queue revision, never a copy left in the seeded plan. */
export function mockLiveSyncPlan(
  state: State,
  targetId: string,
  now = Date.now(),
): SyncPlan | null {
  const plan = state.syncPlans.get(targetId);
  if (!plan || !['computed', 'approved', 'applying'].includes(plan.state)) return null;
  return projectMockSyncPlan(state, targetId, plan, now);
}

/** Mirror explicit check and exact-plan dispatch without changing request intent. */
export function mockSyncRunNow(
  state: State,
  targetId: string,
  input: unknown,
  now: number,
  actorId = VIEWER.id,
): Reply {
  if (
    input === null ||
    typeof input !== 'object' ||
    !('reason' in input) ||
    typeof input.reason !== 'string' ||
    !input.reason.trim()
  ) {
    return { status: 400, body: { code: 'invalid_request', message: 'run now requires a reason' } };
  }
  const key = 'request_key' in input ? (input.request_key ?? '') : '';
  if (
    typeof key !== 'string' ||
    key === '' ||
    key.trim() !== key ||
    new TextEncoder().encode(key).length > 200
  )
    return { status: 400, body: { code: 'invalid_request', message: 'invalid sync request key' } };
  if (
    !('action' in input) ||
    (input.action !== 'check' && input.action !== 'dispatch') ||
    (input.action === 'check' &&
      (key === '' ||
        ('plan_id' in input && input.plan_id !== '') ||
        ('expected_revision' in input && input.expected_revision !== 0))) ||
    (input.action === 'dispatch' &&
      (!('plan_id' in input) ||
        typeof input.plan_id !== 'string' ||
        !input.plan_id.trim() ||
        input.plan_id.trim() !== input.plan_id ||
        !('expected_revision' in input) ||
        !Number.isSafeInteger(input.expected_revision) ||
        Number(input.expected_revision) <= 0))
  ) {
    return {
      status: 400,
      body: { code: 'invalid_request', message: 'a check or exact plan dispatch is required' },
    };
  }
  const reason = input.reason.trim();
  const receiptKey = JSON.stringify([actorId, key]);
  if (input.action === 'check') {
    const receipt = state.syncCheckReceipts.get(receiptKey);
    if (receipt) {
      if (receipt.targetId !== targetId || receipt.reason !== reason) return conflict();
      return {
        status: 200,
        body: { status: 'check_accepted', check_id: receipt.checkId, repeated: true },
      };
    }
  }
  if (input.action === 'dispatch') {
    const receipt = state.syncDispatchReceipts.get(receiptKey);
    if (receipt) {
      if (
        receipt.targetId !== targetId ||
        !('plan_id' in input) ||
        receipt.planId !== input.plan_id ||
        !('expected_revision' in input) ||
        receipt.expectedRevision !== input.expected_revision ||
        receipt.reason !== reason
      )
        return conflict();
      return {
        status: 200,
        body: {
          status: 'dispatch_accepted',
          plan_id: receipt.planId,
          queue_id: receipt.queueId,
          repeated: true,
        },
      };
    }
  }
  let plan = mockLiveSyncPlan(state, targetId, now);
  if (input.action === 'dispatch') {
    const requested = [
      state.syncPlans.get(targetId),
      ...(state.syncHistory.get(targetId) ?? []),
    ].find((p) => p && 'plan_id' in input && p.id === input.plan_id);
    if (!requested || !('plan_id' in input) || input.plan_id !== requested.id)
      return { status: 404, body: { code: 'not_found', message: 'sync plan not found' } };
    plan = projectMockSyncPlan(state, targetId, requested, now);
  }
  const expiredWaiting =
    plan !== null &&
    ['computed', 'approved'].includes(plan.state) &&
    Date.parse(plan.expires_at) <= now;
  if (plan !== null && !(input.action === 'check' && expiredWaiting)) {
    if (input.action === 'check') return { status: 200, body: { status: 'changes_pending', plan } };
    if (plan.dispatch?.reason === 'approval_required')
      return { status: 200, body: { status: 'approval_required', plan } };
    if (plan.dispatch?.reason === 'already_running')
      return { status: 200, body: { status: 'already_running', plan } };
    if (!plan.dispatch?.available) {
      return {
        status: 409,
        body: {
          code: 'unsupported_plan_state',
          message: 'these changes cannot run now',
          dispatch: plan.dispatch,
        },
      };
    }
    const item = plan.queue_item;
    if (!item || !('expected_revision' in input) || input.expected_revision !== item.revision) {
      return {
        status: 409,
        body: {
          code: 'stale_revision',
          message: 'sync queue item changed; review the latest state',
        },
      };
    }
    if (item.state === 'running' || terminal.has(item.state)) return conflict();
    const updated = request(state, item, reason, now);
    state.syncDispatchReceipts.set(receiptKey, {
      targetId,
      planId: plan.id,
      expectedRevision: Number(input.expected_revision),
      reason,
      queueId: updated.id,
    });
    return {
      status: 202,
      body: { status: 'dispatch_accepted', plan_id: plan.id, queue_id: updated.id },
    };
  }
  const held = state.queue.findLast(
    (item) => item.target_id === targetId && item.kind === 'sync_scan',
  );
  if (held?.state === 'running') return conflict();
  if (plan && expiredWaiting) retireExpiredPlan(state, targetId, plan, now);
  let item = held;
  if (!item || terminal.has(item.state)) {
    const at = new Date(now).toISOString();
    item = {
      id: `sync-scan:${randomUUID()}`,
      kind: 'sync_scan',
      lane: 'maintenance',
      target_id: targetId,
      title: 'Check which repositories are in step',
      state: 'scheduled',
      priority: 'normal',
      priority_overridden: false,
      window_mode: 'respect',
      immediate: false,
      not_before: at,
      cadence_anchor_at: at,
      eligible_at: at,
      work_ahead: 0,
      progress_current: 0,
      progress_total: 0,
      attempt: 0,
      revision: 1,
      created_at: at,
      updated_at: at,
      actions: ['run_now', 'next_window', 'schedule_at', 'set_priority', 'cancel'],
    };
    state.queue.push(item);
    recordMockSyncEvent(state, item, 'created', item.title, at);
  }
  const accepted = request(state, item, reason, now);
  state.syncCheckReceipts.set(receiptKey, { targetId, reason, checkId: accepted.id });
  return { status: 202, body: { status: 'check_accepted', check_id: accepted.id } };
}

function conflict(): Reply {
  return {
    status: 409,
    body: { code: 'conflict', message: 'queue item changed; reload and try again' },
  };
}

function request(state: State, item: QueueItem, reason: string, now: number): QueueItem {
  const at = new Date(now).toISOString();
  const updated: QueueItem = {
    ...item,
    state: 'ready',
    window_mode: 'bypass',
    immediate: true,
    reason,
    not_before: at,
    eligible_at: at,
    estimated_start_at: at,
    updated_at: at,
    revision: item.revision + 1,
  };
  delete updated.blocked_reason;
  delete updated.lease_expires_at;
  delete updated.finished_at;
  state.queue[state.queue.indexOf(item)] = updated;
  recordMockSyncEvent(state, updated, 'action.run_now', `Run now requested: ${reason}`, at);
  return updated;
}

function retireExpiredPlan(state: State, targetId: string, plan: SyncPlan, now: number): void {
  const at = new Date(now).toISOString();
  const retired: SyncPlan = { ...plan, state: 'expired', finished_at: at };
  state.syncPlans.set(targetId, retired);
  const history = state.syncHistory.get(targetId) ?? [];
  state.syncHistory.set(targetId, [...history.filter((entry) => entry.id !== plan.id), retired]);
  for (const item of state.queue) {
    if (
      item.source_kind === 'sync_plan' &&
      item.source_id === plan.id &&
      ['awaiting_approval', 'blocked', 'scheduled', 'ready', 'retrying'].includes(item.state)
    ) {
      item.state = 'superseded';
      item.finished_at = at;
      item.updated_at = at;
      item.revision++;
    }
  }
}
