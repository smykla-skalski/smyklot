import { describe, expect, it } from 'vitest';
import { seed } from '../dev/fixtures';
import { mockLiveSyncPlan, mockSyncRunNow } from '../dev/sync-run-now';
const now = Date.UTC(2026, 8, 14);
function fixture() {
  const state = seed(undefined, now);
  const plan = mockLiveSyncPlan(state, '2001')!;
  const input = {
    action: 'dispatch',
    request_key: 'dispatch-1',
    plan_id: plan.id,
    expected_revision: plan.queue_item!.revision,
    reason: 'Run selected changes',
  };
  return { state, plan, input };
}
describe('mock dispatch acceptance [Unit]', () => {
  it('recovers original acceptance after pruning and newer work without mutation', () => {
    const { state, plan, input } = fixture();
    const first = mockSyncRunNow(state, '2001', input, now);
    expect(first).toMatchObject({
      status: 202,
      body: { status: 'dispatch_accepted', plan_id: plan.id, queue_id: plan.queue_item!.id },
    });
    state.queue = state.queue.filter((item) => item.id !== plan.queue_item!.id);
    state.syncPlans.set('2001', { ...plan, id: 'newer', state: 'approved' });
    const before = structuredClone(state.queue);
    expect(mockSyncRunNow(state, '2001', input, now + 86400000)).toEqual({
      status: 200,
      body: {
        status: 'dispatch_accepted',
        plan_id: plan.id,
        queue_id: plan.queue_item!.id,
        repeated: true,
      },
    });
    expect(state.queue).toEqual(before);
  });
  it('rejects changed input and isolates actors', () => {
    const { state, input } = fixture();
    expect(mockSyncRunNow(state, '2001', input, now).status).toBe(202);
    for (const change of [{ reason: 'other' }, { plan_id: 'newer' }, { expected_revision: 99 }])
      expect(mockSyncRunNow(state, '2001', { ...input, ...change }, now).status).toBe(409);
    expect(mockSyncRunNow(state, '1001', input, now).status).toBe(409);
    expect(mockSyncRunNow(state, '2001', input, now, 'other-actor').status).toBe(409);
  });
  it.each([undefined, null, '', ' padded', 'ą'.repeat(101)])(
    'rejects invalid key %j without mutation',
    (request_key) => {
      const { state, input } = fixture();
      const before = structuredClone(state.queue);
      expect(mockSyncRunNow(state, '2001', { ...input, request_key }, now).status).toBe(400);
      expect(state.queue).toEqual(before);
      expect(state.syncDispatchReceipts.size).toBe(0);
    },
  );
});
