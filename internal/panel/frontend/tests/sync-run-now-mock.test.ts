import { describe, expect, it } from 'vitest';
import { mockSyncHistory } from '../dev/sync-history.js';
import { seed } from '../dev/fixtures.js';
import { mockLiveSyncPlan, mockSyncRunNow } from '../dev/sync-run-now.js';

const now = Date.UTC(2026, 8, 13);
const input = { action: 'check', reason: 'Check sync from the status view' };

describe('mock sync request contract [Unit]', () => {
  it.each([null, {}, { reason: ' ' }, { reason: 42 }])(
    'rejects invalid input %j without mutation',
    (value) => {
      const state = seed(undefined, now);
      const before = structuredClone(state.queue);
      expect(mockSyncRunNow(state, '2001', value, now).status).toBe(400);
      expect(state.queue).toEqual(before);
    },
  );

  it('projects the current queue revision and rejects an outdated dispatch', () => {
    const state = seed(undefined, now);
    const plan = mockLiveSyncPlan(state, '2001')!;
    expect(plan.queue_item?.source_id).toBe(plan.id);
    const revision = plan.queue_item!.revision;
    plan.queue_item!.revision++;
    expect(
      mockSyncRunNow(
        state,
        '2001',
        { ...input, action: 'dispatch', plan_id: plan.id, expected_revision: revision },
        now,
      ).status,
    ).toBe(409);
    const result = mockSyncRunNow(
      state,
      '2001',
      { ...input, action: 'dispatch', plan_id: plan.id, expected_revision: revision + 1 },
      now,
    );
    expect(result.status).toBe(202);
    if (result.status !== 202) throw new Error('Expected accepted dispatch');
    expect(result.body.status).toBe('plan_dispatched');
    expect(result.body.queue_item).toMatchObject({
      state: 'ready',
      immediate: true,
      window_mode: 'bypass',
      revision: revision + 2,
    });
    expect(mockLiveSyncPlan(state, '2001')!.queue_item).toEqual(result.body.queue_item);
    expect(
      mockSyncHistory(state, '2001').find((entry) => entry.id === plan.id)?.queue_item,
    ).toEqual(result.body.queue_item);
    expect(state.queue.filter((item) => item.kind === 'sync_scan')).toHaveLength(0);
  });

  it.each([
    ['computed', 'changes_pending'],
    ['applying', 'changes_pending'],
    ['approved', 'changes_pending'],
  ] as const)('returns %s without creating another check', (phase, response) => {
    const state = seed(undefined, now);
    state.syncPlans.get('2001')!.state = phase;
    const before = structuredClone(state.queue);
    expect(mockSyncRunNow(state, '2001', input, now)).toMatchObject({
      status: 200,
      body: { status: response },
    });
    expect(state.queue).toEqual(before);
  });

  it.each(['missing', 'terminal'] as const)(
    'never turns a %s dispatch into a fresh check',
    (kind) => {
      const state = seed(undefined, now);
      const plan = state.syncPlans.get('2001')!;
      const request = {
        action: 'dispatch',
        plan_id: plan.id,
        expected_revision: 1,
        reason: 'Run selected changes',
      };
      if (kind === 'missing') request.plan_id = 'another-plan';
      else plan.state = 'expired';
      const before = structuredClone(state.queue);
      expect(mockSyncRunNow(state, '2001', request, now).status).toBe(
        kind === 'missing' ? 404 : 409,
      );
      expect(state.queue).toEqual(before);
    },
  );

  it('reuses a waiting occurrence, rejects a running one and preserves completed history', () => {
    const state = seed(undefined, now);
    state.syncPlans.delete('2001');
    const first = mockSyncRunNow(state, '2001', input, now);
    if (first.status !== 202) throw new Error('Expected queued scan');
    const id = first.body.queue_item!.id;
    const second = mockSyncRunNow(state, '2001', input, now + 1_000);
    expect(second).toMatchObject({ status: 202, body: { queue_item: { id, revision: 3 } } });
    const item = state.queue.find((entry) => entry.id === id)!;
    item.state = 'running';
    expect(mockSyncRunNow(state, '2001', input, now + 2_000).status).toBe(409);
    item.state = 'succeeded';
    const completed = structuredClone(item);
    const third = mockSyncRunNow(state, '2001', input, now + 3_000);
    if (third.status !== 202) throw new Error('Expected new occurrence');
    expect(third.body.queue_item!.id).not.toBe(id);
    expect(state.queue.find((entry) => entry.id === id)).toEqual(completed);
  });

  it('does not reuse another workspace occurrence or a terminal plan', () => {
    const state = seed(undefined, now);
    state.syncPlans.get('2001')!.state = 'expired';
    const first = mockSyncRunNow(state, '2001', input, now);
    const second = mockSyncRunNow(state, '2002', input, now);
    if (first.status !== 202 || second.status !== 202) throw new Error('Expected scans');
    expect(first.body.queue_item!.target_id).toBe('2001');
    expect(second.body.queue_item!.target_id).toBe('2002');
    expect(first.body.queue_item!.id).not.toBe(second.body.queue_item!.id);
    expect(mockLiveSyncPlan(state, '2001')).toBeNull();
  });
});
