import { describe, expect, it } from 'vitest';
import { seed, VIEWER } from '../dev/fixtures';
import { mockLiveSyncPlan, mockSyncRunNow } from '../dev/sync-run-now';
import { mockSyncRequests } from '../dev/sync-request-history';

const now = Date.UTC(2026, 8, 14);
const input = { action: 'check', request_key: 'check-1', reason: 'Verify saved settings' };
function fixture() {
  const state = seed(undefined, now);
  const plan = mockLiveSyncPlan(state, '2001', now)!;
  const dispatch = {
    action: 'dispatch',
    request_key: 'dispatch-1',
    plan_id: plan.id,
    expected_revision: plan.queue_item!.revision,
    reason: 'Apply reviewed changes',
  };
  expect(mockSyncRunNow(state, '2001', dispatch, now).status).toBe(202);
  state.syncPlans.delete('2001');
  expect(mockSyncRunNow(state, '2001', input, now).status).toBe(202);
  return { state, dispatch, plan };
}

describe('mock accepted request history [Unit]', () => {
  it('pages acceptance across both actions with stable original time and subject', () => {
    const { state, dispatch, plan } = fixture();
    const first = mockSyncRequests(state, '2001', new URLSearchParams({ limit: '1' }));
    if (first.status !== 200) throw new Error('Expected history');
    expect(first.body.items).toEqual([
      {
        action: 'dispatch',
        request_key: dispatch.request_key,
        plan_id: dispatch.plan_id,
        queue_id: plan.queue_item!.id,
        expected_revision: dispatch.expected_revision,
        reason: dispatch.reason,
        accepted_at: new Date(now).toISOString(),
      },
    ]);
    expect(first.body.next_cursor).toBeTruthy();
    expect(
      mockSyncRunNow(state, '2001', { ...input, request_key: 'newer' }, now + 1_000).status,
    ).toBe(202);
    expect(mockSyncRunNow(state, '2001', input, now + 2_000).status).toBe(200);
    const next = mockSyncRequests(
      state,
      '2001',
      new URLSearchParams({ cursor: first.body.next_cursor! }),
    );
    if (next.status !== 200) throw new Error('Expected continuation');
    expect(next.body.items).toHaveLength(1);
    expect(next.body.items[0]).toMatchObject({
      action: 'check',
      request_key: input.request_key,
      accepted_at: new Date(now).toISOString(),
    });
    expect(next.body.next_cursor).toBeNull();
    expect(next.body).not.toHaveProperty('total');
  });

  it('retains history after pruning and cannot mutate queue state', () => {
    const { state } = fixture();
    const before = mockSyncRequests(state, '2001', new URLSearchParams());
    state.queue = [];
    expect(mockSyncRequests(state, '2001', new URLSearchParams())).toEqual(before);
    expect(state.queue).toEqual([]);
  });

  it('scopes receipts to actor and workspace while allowing Viewer reads', () => {
    const { state } = fixture();
    state.targets.find((target) => target.value.id === '2001')!.value.effective_role = 'viewer';
    expect(mockSyncRequests(state, '2001', new URLSearchParams()).status).toBe(200);
    expect(mockSyncRequests(state, '2001', new URLSearchParams(), 'another-actor')).toEqual({
      status: 200,
      body: { items: [], next_cursor: null },
    });
    expect(mockSyncRequests(state, '1001', new URLSearchParams())).toEqual({
      status: 200,
      body: { items: [], next_cursor: null },
    });
    state.targets.find((target) => target.value.id === '2001')!.value.effective_role = 'none';
    expect(mockSyncRequests(state, '2001', new URLSearchParams()).status).toBe(404);
  });

  it('rejects foreign or malformed continuations and unsupported queries', () => {
    const { state } = fixture();
    const first = mockSyncRequests(state, '2001', new URLSearchParams({ limit: '1' }));
    if (first.status !== 200 || !first.body.next_cursor) throw new Error('Expected cursor');
    const token = first.body.next_cursor;
    expect(mockSyncRequests(state, '1001', new URLSearchParams({ cursor: token })).status).toBe(
      400,
    );
    expect(
      mockSyncRequests(state, '2001', new URLSearchParams({ cursor: token }), 'another-actor')
        .status,
    ).toBe(400);
    for (const query of [
      'limit=0',
      'limit=101',
      'limit=1.5',
      'limit=1&limit=2',
      'cursor=x&cursor=y',
      'actor=someone',
      'cursor=!',
      `cursor=${'x'.repeat(4097)}`,
    ])
      expect(mockSyncRequests(state, '2001', new URLSearchParams(query)).status).toBe(400);
    const cursor = JSON.parse(Buffer.from(token, 'base64url').toString());
    for (const change of [
      { version: 2 },
      { actor_id: 'someone' },
      { target_id: 'elsewhere' },
      { action: 'retry' },
      { request_key: '' },
      { request_key: ' padded' },
      { request_key: 'ą'.repeat(101) },
      { accepted_at: 'invalid' },
      { extra: true },
    ]) {
      const cursorValue = Buffer.from(JSON.stringify({ ...cursor, ...change })).toString(
        'base64url',
      );
      expect(
        mockSyncRequests(state, '2001', new URLSearchParams({ cursor: cursorValue }), VIEWER.id)
          .status,
      ).toBe(400);
    }
  });
});
