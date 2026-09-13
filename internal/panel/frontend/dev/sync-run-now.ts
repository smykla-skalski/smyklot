import { recordMockSyncEvent } from './sync-queue.js';
import { randomUUID } from 'node:crypto';
import type { MockState } from './fixtures.js';
import type { QueueItem, SyncPlan, SyncRunNowResponse } from '../src/lib/types.js';

type State = Pick<MockState, 'queue' | 'syncPlans' | 'syncQueueEvents'>;
type Reply =
  | { status: 200 | 202; body: SyncRunNowResponse }
  | { status: 400 | 404 | 409; body: { code: string; message: string } };
const terminal = new Set(['succeeded', 'failed', 'cancelled', 'superseded']);

/** Project the current queue revision, never a copy left in the seeded plan. */
export function mockLiveSyncPlan(state: State, targetId: string): SyncPlan | null {
  const plan = state.syncPlans.get(targetId);
  if (!plan || !['computed', 'approved', 'applying'].includes(plan.state)) return null;
  const queue = state.queue.find(
    (item) =>
      item.target_id === targetId &&
      item.kind === 'sync_apply' &&
      item.source_id === plan.id &&
      item.source_kind === 'sync_plan',
  );
  return { ...plan, queue_item: queue };
}

/** Mirror explicit check and exact-plan dispatch without changing request intent. */
export function mockSyncRunNow(state: State, targetId: string, input: unknown, now: number): Reply {
  if (
    input === null ||
    typeof input !== 'object' ||
    !('reason' in input) ||
    typeof input.reason !== 'string' ||
    !input.reason.trim()
  ) {
    return { status: 400, body: { code: 'invalid_request', message: 'run now requires a reason' } };
  }
  if (
    !('action' in input) ||
    (input.action !== 'check' && input.action !== 'dispatch') ||
    (input.action === 'check' &&
      (('plan_id' in input && input.plan_id !== '') ||
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
  const plan = mockLiveSyncPlan(state, targetId);
  if (input.action === 'dispatch') {
    const requested = state.syncPlans.get(targetId);
    if (!requested || !('plan_id' in input) || input.plan_id !== requested.id)
      return { status: 404, body: { code: 'not_found', message: 'sync plan not found' } };
    if (plan === null) return conflict();
  }
  if (plan !== null) {
    if (input.action === 'check') return { status: 200, body: { status: 'changes_pending', plan } };
    if (plan.state === 'computed')
      return { status: 200, body: { status: 'approval_required', plan } };
    if (plan.state === 'applying')
      return { status: 200, body: { status: 'already_running', plan } };
    if (plan.state !== 'approved') {
      return {
        status: 409,
        body: { code: 'unsupported_plan_state', message: 'sync plan cannot run now' },
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
    return {
      status: 202,
      body: {
        status: 'plan_dispatched',
        plan: { ...plan, queue_item: updated },
        queue_item: updated,
      },
    };
  }
  const held = state.queue.findLast(
    (item) => item.target_id === targetId && item.kind === 'sync_scan',
  );
  if (held?.state === 'running') return conflict();
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
  return {
    status: 202,
    body: { status: 'scan_queued', queue_item: request(state, item, reason, now) },
  };
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
