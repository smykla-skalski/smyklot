import { describe, expect, it } from 'vitest';
import { seed } from '../dev/fixtures.js';
import { mockSyncRunNow } from '../dev/sync-run-now.js';

const now = Date.UTC(2026, 8, 14);
const input = { action: 'check', request_key: 'request-1', reason: 'Check saved settings' };
function fixture() {
  const state = seed(undefined, now);
  state.syncPlans.delete('2001');
  return state;
}

describe('mock accepted check receipts [Unit]', () => {
  it('recovers the original identity after completion, pruning and newer changes', () => {
    const state = fixture();
    const accepted = mockSyncRunNow(state, '2001', input, now);
    expect(accepted.status).toBe(202);
    if (accepted.status !== 202) throw new Error('Expected acceptance');
    expect(accepted.body.status).toBe('check_accepted');
    const id = accepted.body.check_id;
    expect(id).toBeTruthy();
    expect(accepted.body.queue_item).toBeUndefined();
    state.queue.find((item) => item.id === id)!.state = 'succeeded';
    state.queue = state.queue.filter((item) => item.id !== id);
    state.syncPlans.set('2001', seed(undefined, now).syncPlans.get('2001')!);
    const before = structuredClone(state.queue);
    expect(mockSyncRunNow(state, '2001', input, now + 60_000)).toEqual({
      status: 200,
      body: { status: 'check_accepted', check_id: id, repeated: true },
    });
    expect(state.queue).toEqual(before);
  });

  it('reuses acceptance while running without changing events or revision', () => {
    const state = fixture();
    const first = mockSyncRunNow(state, '2001', input, now);
    if (first.status !== 202) throw new Error('Expected acceptance');
    state.queue.find((item) => item.id === first.body.check_id)!.state = 'running';
    const before = structuredClone({ queue: state.queue, events: state.queueEvents });
    const recovered = mockSyncRunNow(state, '2001', input, now + 1_000);
    expect(recovered).toMatchObject({ status: 200, body: { check_id: first.body.check_id } });
    expect({ queue: state.queue, events: state.queueEvents }).toEqual(before);
  });

  it('binds the key to actor, workspace and normalized reason', () => {
    const state = fixture();
    const first = mockSyncRunNow(state, '2001', input, now, 'actor-a');
    if (first.status !== 202) throw new Error('Expected acceptance');
    expect(mockSyncRunNow(state, '2002', input, now, 'actor-a').status).toBe(409);
    expect(
      mockSyncRunNow(state, '2001', { ...input, reason: 'Different' }, now, 'actor-a').status,
    ).toBe(409);
    expect(
      mockSyncRunNow(state, '2001', { ...input, reason: ` ${input.reason} ` }, now, 'actor-a')
        .status,
    ).toBe(200);
    const other = mockSyncRunNow(state, '2002', input, now, 'actor-b');
    if (other.status !== 202) throw new Error('Expected separate acceptance');
    expect(other.body.check_id).not.toBe(first.body.check_id);
  });

  it('does not retain a blocked request as accepted work', () => {
    const state = seed(undefined, now);
    expect(mockSyncRunNow(state, '2001', input, now)).toMatchObject({
      status: 200,
      body: { status: 'changes_pending' },
    });
    expect(state.syncCheckReceipts.size).toBe(0);
    state.syncPlans.delete('2001');
    expect(mockSyncRunNow(state, '2001', input, now).status).toBe(202);
  });

  it.each([undefined, null, '', ' ', ' padded', 'padded ', 'ą'.repeat(101), 123])(
    'rejects malformed key %j without mutation',
    (request_key) => {
      const state = fixture();
      const before = structuredClone(state.queue);
      expect(mockSyncRunNow(state, '2001', { ...input, request_key }, now).status).toBe(400);
      expect(state.queue).toEqual(before);
      expect(state.syncCheckReceipts.size).toBe(0);
    },
  );
});

describe('mock fresh checks after expiry [Unit]', () => {
  it.each(['computed', 'approved'] as const)(
    'retires expired %s work only when accepting a check',
    (phase) => {
      const state = seed(undefined, now);
      const plan = state.syncPlans.get('2001')!;
      plan.state = phase;
      plan.expires_at = new Date(now).toISOString();
      const result = mockSyncRunNow(state, '2001', input, now);
      expect(result).toMatchObject({ status: 202, body: { status: 'check_accepted' } });
      expect(state.syncPlans.get('2001')?.state).toBe('expired');
      expect(state.syncHistory.get('2001')?.find((p) => p.id === plan.id)?.state).toBe('expired');
      expect(
        state.queue.find((item) => item.source_kind === 'sync_plan' && item.source_id === plan.id)
          ?.state,
      ).toBe('superseded');
      const before = structuredClone(state);
      expect(mockSyncRunNow(state, '2001', input, now)).toMatchObject({
        status: 200,
        body: { repeated: true },
      });
      expect(state).toEqual(before);
    },
  );
  it('preserves an expired plan when a running check prevents acceptance', () => {
    const state = seed(undefined, now);
    const plan = state.syncPlans.get('2001')!;
    state.syncPlans.delete('2001');
    const accepted = mockSyncRunNow(state, '2001', input, now);
    if (accepted.status !== 202) throw new Error('Expected acceptance');
    state.queue.find((item) => item.id === accepted.body.check_id)!.state = 'running';
    plan.expires_at = new Date(now).toISOString();
    state.syncPlans.set('2001', plan);
    const before = structuredClone(state);
    expect(mockSyncRunNow(state, '2001', { ...input, request_key: 'new-check' }, now).status).toBe(
      409,
    );
    expect(state).toEqual(before);
  });
  it('does not retire applying work after approval expiry', () => {
    const state = seed(undefined, now);
    const plan = state.syncPlans.get('2001')!;
    plan.state = 'applying';
    plan.expires_at = new Date(now).toISOString();
    const before = structuredClone(state);
    expect(mockSyncRunNow(state, '2001', input, now)).toMatchObject({
      status: 200,
      body: { status: 'changes_pending' },
    });
    expect(state).toEqual(before);
  });
});
