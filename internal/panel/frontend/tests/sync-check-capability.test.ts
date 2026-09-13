import { describe, expect, it } from 'vitest';
import { seed } from '../dev/fixtures';
import { mockCheckCapability } from '../dev/sync-capability';
import { mockSyncRunNow } from '../dev/sync-run-now';
const now = Date.UTC(2026, 8, 14);
describe('current check capability [Unit]', () => {
  it.each(['computed', 'approved'] as const)(
    'uses the current %s plan and respects expiry without mutation',
    (stateName) => {
      const state = seed(undefined, now);
      const plan = state.syncPlans.get('2001')!;
      plan.state = stateName;
      plan.expires_at = new Date(now + 1000).toISOString();
      expect(mockCheckCapability(state, '2001', now)).toMatchObject({
        available: false,
        reason: 'changes_pending',
        blocking_plan_id: plan.id,
      });
      const before = structuredClone(state);
      expect(mockCheckCapability(state, '2001', now + 1000)).toMatchObject({
        available: true,
        reason: 'available',
        target_id: '2001',
        action: 'check',
        effect: 'request_repository_check',
      });
      expect(state).toEqual(before);
    },
  );
  it('keeps applying work as a blocker beyond expiry', () => {
    const state = seed(undefined, now);
    const plan = state.syncPlans.get('2001')!;
    plan.state = 'applying';
    plan.expires_at = new Date(now).toISOString();
    expect(mockCheckCapability(state, '2001', now)).toMatchObject({
      available: false,
      reason: 'changes_running',
      blocking_plan_id: plan.id,
    });
  });
  it('names a running check when there is no live plan', () => {
    const state = seed(undefined, now);
    state.syncPlans.delete('2001');
    const accepted = mockSyncRunNow(
      state,
      '2001',
      { action: 'check', request_key: 'capability', reason: 'Check' },
      now,
    );
    if (accepted.status !== 202) throw new Error('Expected acceptance');
    state.queue.find((item) => item.id === accepted.body.check_id)!.state = 'running';
    expect(mockCheckCapability(state, '2001', now)).toMatchObject({
      available: false,
      reason: 'check_running',
      running_check_id: accepted.body.check_id,
    });
  });
  it.each(['viewer', 'editor', 'none'] as const)(
    'restricts %s without exposing blocker identity',
    (role) => {
      const state = seed(undefined, now);
      state.targets.find((target) => target.value.id === '2001')!.value.effective_role = role;
      expect(mockCheckCapability(state, '2001', now)).toEqual({
        available: false,
        reason: 'admin_or_owner_required',
        action: 'check',
        target_id: '2001',
        effect: 'request_repository_check',
      });
    },
  );
});
