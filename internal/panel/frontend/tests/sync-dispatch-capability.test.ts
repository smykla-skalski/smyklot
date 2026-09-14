import { describe, expect, it } from 'vitest';
import { seed } from '../dev/fixtures';
import { projectMockSyncPlan } from '../dev/sync-capability';
import { mockSyncRunNow } from '../dev/sync-run-now';
import {
  syncDispatchGuidance,
  syncDispatchIntent,
  syncPlanExecutionProblem,
} from '../src/lib/sync-dispatch-guidance';
import type { SyncDispatchCapability } from '../src/lib/types';
const now = Date.UTC(2026, 8, 14);
describe('current dispatch capabilities', () => {
  it.each(['owner', 'admin', 'editor', 'viewer', 'none'] as const)(
    'projects current %s authority without changing stored history',
    (role) => {
      const state = seed(undefined, now);
      const target = state.targets.find((t) => t.value.id === '2001')!;
      target.value.effective_role = role;
      const saved = state.syncPlans.get('2001')!;
      const before = structuredClone(saved);
      const p = projectMockSyncPlan(state, '2001', saved, now);
      expect(p.dispatch?.available).toBe(['owner', 'admin'].includes(role));
      expect(syncDispatchIntent(p) !== null).toBe(['owner', 'admin'].includes(role));
      expect(saved).toEqual(before);
    },
  );
  it('expires a capability without a sweep and agrees with refusal', () => {
    const state = seed(undefined, now);
    const saved = state.syncPlans.get('2001')!;
    saved.expires_at = new Date(now).toISOString();
    const p = projectMockSyncPlan(state, '2001', saved, now);
    expect(p.dispatch?.reason).toBe('plan_expired');
    expect(syncDispatchIntent(p)).toBeNull();
    const reply = mockSyncRunNow(
      state,
      '2001',
      {
        action: 'dispatch',
        plan_id: p.id,
        expected_revision: p.queue_item!.revision,
        request_key: 'expired',
        reason: 'Run reviewed changes',
      },
      now,
    );
    expect(reply).toMatchObject({ status: 409, body: { dispatch: p.dispatch } });
    expect(state.syncDispatchReceipts.size).toBe(0);
  });
  it('does not reuse retained queue snapshots as current command authority', () => {
    const state = seed(undefined, now);
    const saved = state.syncPlans.get('2001')!;
    state.queue = [];
    const p = projectMockSyncPlan(state, '2001', saved, now);
    expect(p.queue_item).toBeUndefined();
    expect(p.dispatch?.reason).toBe('queue_unavailable');
    expect(syncDispatchIntent(p)).toBeNull();
  });
  it.each([
    'admin_or_owner_required',
    'approval_required',
    'already_running',
    'plan_expired',
    'plan_changed',
    'plan_finished',
    'queue_unavailable',
    'queue_finished',
    'state_unsupported',
  ] as const)('explains %s without constructing a command', (reason) => {
    const state = seed(undefined, now);
    const p = projectMockSyncPlan(state, '2001', state.syncPlans.get('2001')!, now);
    p.dispatch = { ...p.dispatch!, available: false, reason };
    expect(syncDispatchGuidance(p)).toBeTruthy();
    expect(syncDispatchIntent(p)).toBeNull();
  });
  it.each([
    undefined,
    { plan_id: 'another' },
    { expected_revision: 0 },
    { expected_revision: 1.2 },
    { queue_id: '' },
  ])('refuses incomplete or foreign capability %j', (change) => {
    const state = seed(undefined, now);
    const p = projectMockSyncPlan(state, '2001', state.syncPlans.get('2001')!, now);
    p.dispatch =
      change === undefined ? undefined : ({ ...p.dispatch!, ...change } as SyncDispatchCapability);
    expect(syncDispatchIntent(p)).toBeNull();
  });
});

describe('execution status evidence', () => {
  it.each([
    'plan_expired',
    'plan_changed',
    'plan_finished',
    'queue_finished',
    'queue_unavailable',
    'state_unsupported',
  ] as const)('qualifies queued claims for %s', (reason) => {
    const state = seed(undefined, now);
    const p = projectMockSyncPlan(state, '2001', state.syncPlans.get('2001')!, now);
    p.dispatch = { ...p.dispatch!, available: false, reason };
    expect(syncPlanExecutionProblem(p)).toBeTruthy();
  });
  it('keeps role restrictions separate from current execution', () => {
    const state = seed(undefined, now);
    const p = projectMockSyncPlan(state, '2001', state.syncPlans.get('2001')!, now);
    p.dispatch = { ...p.dispatch!, available: false, reason: 'admin_or_owner_required' };
    expect(syncPlanExecutionProblem(p)).toBeNull();
    delete p.queue_item;
    expect(syncPlanExecutionProblem(p)).toBe('Execution status unavailable');
  });
  it('never replaces a completed result with a capability message', () => {
    const state = seed(undefined, now);
    const p = projectMockSyncPlan(state, '2001', state.syncPlans.get('2001')!, now);
    p.state = 'applied';
    p.dispatch = { ...p.dispatch!, available: false, reason: 'plan_expired' };
    expect(syncPlanExecutionProblem(p)).toBeNull();
  });
});
