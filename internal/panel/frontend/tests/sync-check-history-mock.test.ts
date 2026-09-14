import { describe, expect, it } from 'vitest';
import { seed, syncPlanSeed } from '../dev/fixtures';
import { mockSyncRunNow } from '../dev/sync-run-now';
import { finishMockSyncScan } from '../dev/sync-scan';
import { mockSyncCheckPage } from '../dev/sync-check-history';
import { mockSyncCheck } from '../dev/sync-check';

const now = Date.UTC(2026, 8, 13);
const target = '2001';
function setup() {
  const state = seed(undefined, now);
  state.syncPlans.delete(target);
  state.queue = state.queue.filter((item) => item.kind !== 'sync_apply');
  const request = mockSyncRunNow(
    state,
    target,
    { action: 'check', request_key: crypto.randomUUID(), reason: 'Check repositories' },
    now,
  );
  if (request.status !== 202 || request.body.status !== 'check_accepted')
    throw new Error('Expected a check');
  const item = state.queue.find((item) => item.id === request.body.check_id)!;
  return { state, item };
}

describe('retained mock check evidence', () => {
  it('pages its original evidence and never replaces it with current repository state', () => {
    const { state, item } = setup();
    const summary = finishMockSyncScan(state, item, new Date(now).toISOString());
    const original = structuredClone(state.syncCheckObservations.get(item.id)!);
    expect(original.length).toBeGreaterThan(2);
    expect(item.details).toMatchObject({ outcome: { summary, disposition: 'checked', cached: 0 } });
    const first = mockSyncCheckPage(state, target, item.id, new URLSearchParams('limit=2'));
    expect(first.status).toBe(200);
    if (!('items' in first.body)) throw new Error('Expected evidence');
    expect(first.body.items).toEqual(original.slice(0, 2));
    expect(first.body.total).toBe(original.length);
    state.syncStatus.get(target)!.repositories = [];
    expect(finishMockSyncScan(state, item, new Date(now + 60_000).toISOString())).toBe(summary);
    const second = mockSyncCheckPage(
      state,
      target,
      item.id,
      new URLSearchParams({ limit: '100', cursor: first.body.next_cursor! }),
    );
    expect(second.status).toBe(200);
    if (!('items' in second.body)) throw new Error('Expected evidence');
    expect(second.body.items).toEqual(original.slice(2, 102));
    expect(state.syncCheckObservations.get(item.id)).toEqual(original);
    expect(
      mockSyncCheckPage(state, 'another-workspace', item.id, new URLSearchParams()).status,
    ).toBe(404);
    for (const query of [
      'limit=0',
      'limit=101',
      'limit=2&limit=3',
      'limit=1.5',
      'sort=name',
      'cursor=broken',
      'cursor=e30',
      'cursor=bnVsbA',
      'cursor=a&cursor=b',
    ])
      expect(mockSyncCheckPage(state, target, item.id, new URLSearchParams(query)).status).toBe(
        400,
      );
    const foreign = Buffer.from(JSON.stringify({ check_id: 'another-check', after: 2 })).toString(
      'base64url',
    );
    expect(
      mockSyncCheckPage(state, target, item.id, new URLSearchParams({ cursor: foreign })).status,
    ).toBe(400);
  });

  it('retains the earlier plan when newer work replaces it', () => {
    const { state, item } = setup();
    const plan = syncPlanSeed((offset) => new Date(now + offset).toISOString());
    state.syncPlans.set(target, { ...plan, id: 'earlier', state: 'approved' });
    finishMockSyncScan(state, item, new Date(now).toISOString());
    expect(item.details).toMatchObject({
      outcome: { disposition: 'deferred', blocking_plan_id: 'earlier' },
    });
    expect(item.details).not.toHaveProperty('result_plan_id');
    state.syncPlans.set(target, { ...plan, id: 'newer', state: 'approved' });
    finishMockSyncScan(state, item, new Date(now + 60_000).toISOString());
    expect(item.details).toMatchObject({ outcome: { blocking_plan_id: 'earlier' } });
  });

  it('distinguishes an unrecorded check from a recorded empty result', () => {
    const { state, item } = setup();
    expect(mockSyncCheckPage(state, target, item.id, new URLSearchParams()).status).toBe(404);
    state.syncStatus.get(target)!.repositories = [];
    finishMockSyncScan(state, item, new Date(now).toISOString());
    expect(mockSyncCheckPage(state, target, item.id, new URLSearchParams())).toEqual({
      status: 200,
      body: { items: [], total: 0, next_cursor: null },
    });
  });
});

describe('task-oriented check reads', () => {
  it('retains original results after queue cleanup and allows read-only members', () => {
    const { state, item } = setup();
    finishMockSyncScan(state, item, new Date(now).toISOString());
    const before = mockSyncCheck(state, target, item.id, now);
    if (!('result' in before.body)) throw new Error('Expected a check');
    expect(before.body.result?.outcome).toBeDefined();
    const original = structuredClone(before.body.result);
    item.details = { outcome: { summary: 'Replaced queue display' } };
    state.queue = state.queue.filter((candidate) => candidate.id !== item.id);
    state.targets.find((entry) => entry.value.id === target)!.value.effective_role = 'viewer';
    const after = mockSyncCheck(state, target, item.id, now + 1000);
    expect(after.status).toBe(200);
    if (!('result' in after.body)) throw new Error('Expected retained result');
    expect(after.body.result).toEqual(original);
    expect(after.body.execution).toBeNull();
    expect(after.body.observed_at).toBe(new Date(now + 1000).toISOString());
    expect(after.body.check).toMatchObject({ available: false, reason: 'admin_or_owner_required' });
    expect(mockSyncCheckPage(state, target, item.id, new URLSearchParams()).status).toBe(200);
    expect(mockSyncCheck(state, '1001', item.id, now).status).toBe(404);
    state.targets.find((entry) => entry.value.id === target)!.value.effective_role = 'none';
    expect(mockSyncCheck(state, target, item.id, now).status).toBe(404);
  });

  it('keeps unrecorded and successful empty comparisons distinct', () => {
    const { state, item } = setup();
    const pending = mockSyncCheck(state, target, item.id, now);
    if (!('result' in pending.body)) throw new Error('Expected pending check');
    expect(pending.body.result).toBeNull();
    expect(pending.body.execution).not.toBeNull();
    state.syncStatus.get(target)!.repositories = [];
    finishMockSyncScan(state, item, new Date(now).toISOString());
    const completed = mockSyncCheck(state, target, item.id, now);
    if (!('result' in completed.body)) throw new Error('Expected comparison');
    expect(completed.body.result?.outcome).toMatchObject({ counts: {}, disposition: 'checked' });
    expect(mockSyncCheck(state, target, 'missing', now).status).toBe(404);
  });
});
