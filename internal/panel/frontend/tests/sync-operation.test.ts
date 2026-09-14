import { describe, expect, it } from 'vitest';
import { seed, VIEWER } from '../dev/fixtures';
import { mockLiveSyncPlan, mockSyncRunNow } from '../dev/sync-run-now';
import { mockSyncOperation } from '../dev/sync-operation';
import { createPanelApi } from '../src/lib/api';

const now = Date.UTC(2026, 8, 14);
function fixture() {
  const state = seed(undefined, now);
  const plan = mockLiveSyncPlan(state, '2001', now)!;
  const dispatch = {
    action: 'dispatch',
    request_key: 'dispatch-1',
    plan_id: plan.id,
    expected_revision: plan.queue_item!.revision,
    reason: 'Apply original changes',
  };
  expect(mockSyncRunNow(state, '2001', dispatch, now).status).toBe(202);
  state.syncHistory.set('2001', [state.syncPlans.get('2001')!]);
  state.syncPlans.delete('2001');
  expect(
    mockSyncRunNow(
      state,
      '2001',
      { action: 'check', request_key: 'check-1', reason: 'Verify original settings' },
      now,
    ).status,
  ).toBe(202);
  return { state, dispatch };
}

describe('sync operation read model', () => {
  it('keeps exact check and dispatch subjects after worker cleanup', () => {
    const { state, dispatch } = fixture();
    const before = structuredClone(state);
    const check = mockSyncOperation(state, '2001', 'check', 'check-1', new URLSearchParams(), now);
    const run = mockSyncOperation(
      state,
      '2001',
      'dispatch',
      dispatch.request_key,
      new URLSearchParams(),
      now,
    );
    expect(state).toEqual(before);
    if (check.status !== 200 || run.status !== 200) throw new Error('Expected accepted operations');
    expect(check.body.acceptance).toMatchObject({
      action: 'check',
      reason: 'Verify original settings',
    });
    expect(check.body.plan).toBeNull();
    expect(run.body.acceptance).toMatchObject({
      action: 'dispatch',
      plan_id: dispatch.plan_id,
      expected_revision: dispatch.expected_revision,
    });
    expect(run.body.plan?.id).toBe(dispatch.plan_id);
    expect(run.body.comparison).toBeNull();
    expect(run.body.plan).not.toHaveProperty('actions');
    expect(run.body.execution).not.toHaveProperty('events');
    state.queue = [];
    const retained = mockSyncOperation(
      state,
      '2001',
      'dispatch',
      dispatch.request_key,
      new URLSearchParams(),
      now,
    );
    if (retained.status !== 200) throw new Error('Expected retained acceptance');
    expect(retained.body.acceptance).toEqual(run.body.acceptance);
    expect(retained.body.plan?.id).toBe(dispatch.plan_id);
    expect(retained.body.execution).toBeNull();
    state.syncHistory.clear();
    const missing = mockSyncOperation(
      state,
      '2001',
      'dispatch',
      dispatch.request_key,
      new URLSearchParams(),
      now,
    );
    if (missing.status !== 200) throw new Error('Acceptance must survive unavailable details');
    expect(missing.body.plan).toBeNull();
    expect(missing.body.dispatch).toBeNull();
    expect(missing.body.acceptance).toEqual(run.body.acceptance);
  });

  it('preserves retained comparison alongside failed execution', () => {
    const { state } = fixture();
    const receipt = state.syncCheckReceipts.get(JSON.stringify([VIEWER.id, 'check-1']))!;
    const result = { result_plan_id: 'original-plan' };
    state.syncCheckResults.set(receipt.checkId, { targetId: '2001', result });
    const worker = state.queue.find((item) => item.id === receipt.checkId)!;
    worker.state = 'failed';
    worker.summary = 'Worker failed';
    const reply = mockSyncOperation(state, '2001', 'check', 'check-1', new URLSearchParams(), now);
    if (reply.status !== 200) throw new Error('Expected operation');
    expect(reply.body.comparison).toEqual(result);
    expect(reply.body.execution).toMatchObject({ state: 'failed', summary: 'Worker failed' });
    worker.target_id = '1001';
    const foreign = mockSyncOperation(
      state,
      '2001',
      'check',
      'check-1',
      new URLSearchParams(),
      now,
    );
    if (foreign.status !== 200) throw new Error('Expected operation');
    expect(foreign.body.execution).toBeNull();
  });

  it('permits viewer recovery and scopes exact identity', () => {
    const { state } = fixture();
    state.targets.find((target) => target.value.id === '2001')!.value.effective_role = 'viewer';
    const reply = mockSyncOperation(
      state,
      '2001',
      'dispatch',
      'dispatch-1',
      new URLSearchParams(),
      now,
    );
    if (reply.status !== 200) throw new Error('Expected viewer read');
    expect(reply.body.check.available).toBe(false);
    expect(reply.body.dispatch?.available).toBe(false);
    expect(
      mockSyncOperation(state, '2001', 'check', 'dispatch-1', new URLSearchParams(), now).status,
    ).toBe(404);
    expect(
      mockSyncOperation(state, '1001', 'check', 'check-1', new URLSearchParams(), now).status,
    ).toBe(404);
    expect(
      mockSyncOperation(
        state,
        '2001',
        'check',
        'check-1',
        new URLSearchParams(),
        now,
        'another-actor',
      ).status,
    ).toBe(404);
    for (const key of ['', ' padded', 'ą'.repeat(101)])
      expect(
        mockSyncOperation(state, '2001', 'check', key, new URLSearchParams(), now).status,
      ).toBe(400);
    expect(
      mockSyncOperation(state, '2001', 'retry', 'check-1', new URLSearchParams(), now).status,
    ).toBe(400);
    expect(
      mockSyncOperation(
        state,
        '2001',
        'check',
        'check-1',
        new URLSearchParams('actor=someone'),
        now,
      ).status,
    ).toBe(400);
  });

  it('encodes operation identity in a read request', async () => {
    const calls: { url: string; method: string }[] = [];
    const api = createPanelApi('/panel', async (url, init) => {
      calls.push({ url, method: init?.method ?? 'GET' });
      return new Response(
        JSON.stringify({
          target_id: 'workspace',
          acceptance: { action: 'check', request_key: 'opaque/key' },
        }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      );
    });
    await api.fetchSyncOperation('workspace/name', 'check', 'opaque/key');
    expect(calls).toEqual([
      {
        url: '/panel/api/v1/targets/workspace%2Fname/sync/requests/check/opaque%2Fkey',
        method: 'GET',
      },
    ]);
  });
});
