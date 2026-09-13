import { describe, expect, it } from 'vitest';
import { seed } from '../dev/fixtures';
import { mockSyncRunNow } from '../dev/sync-run-now';
import { finishMockSyncScan } from '../dev/sync-scan';
import { mockSyncCheckPage } from '../dev/sync-check-history';

const now = Date.UTC(2026, 8, 13);
const target = '2001';
function setup() {
  const state = seed(undefined, now);
  state.syncPlans.delete(target);
  state.queue = state.queue.filter((item) => item.kind !== 'sync_apply');
  const request = mockSyncRunNow(state, target, { reason: 'Check repositories' }, now);
  if (request.status !== 202 || request.body.status !== 'scan_queued')
    throw new Error('Expected a check');
  const item = state.queue.find((item) => item.id === request.body.queue_item!.id)!;
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
