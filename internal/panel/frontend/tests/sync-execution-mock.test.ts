import { describe, expect, it } from 'vitest';
import { seed } from '../dev/fixtures.js';
import { advanceMockSync } from '../dev/sync-execution.js';
import { mockLiveSyncPlan, mockSyncRunNow } from '../dev/sync-run-now.js';

const now = Date.UTC(2026, 8, 13);
const target = '2001';
function complete(state: ReturnType<typeof seed>, at = now) {
  expect(advanceMockSync(state, at)).toBe(true);
  expect(advanceMockSync(state, at + 4_000)).toBe(true);
}
function scan(state: ReturnType<typeof seed>, at = now) {
  const response = mockSyncRunNow(state, target, { reason: 'Verify outcomes' }, at);
  if (response.status !== 202 || response.body.status !== 'scan_queued')
    throw new Error('Expected scan');
  complete(state, at);
  return state.queue.find((item) => item.id === response.body.queue_item!.id)!;
}

describe('finite mock sync execution [Unit]', () => {
  it('updates the original plan, queue and observations once, then retains immutable history', () => {
    const state = seed(undefined, now);
    const planId = state.syncPlans.get(target)!.id;
    const beforeHistory = state.syncHistory.get(target)!.length;
    const blocked = structuredClone(
      state.syncStatus.get(target)!.repositories.find((row) => row.repository === 'legacy-service'),
    );
    expect(advanceMockSync(state, now)).toBe(true);
    expect(mockLiveSyncPlan(state, target)).toMatchObject({
      id: planId,
      state: 'applying',
      queue_item: { state: 'running' },
    });
    expect(advanceMockSync(state, now + 3_999)).toBe(false);
    expect(advanceMockSync(state, now + 4_000)).toBe(true);
    expect(mockLiveSyncPlan(state, target)).toBeNull();
    const history = state.syncHistory.get(target)!;
    expect(history).toHaveLength(beforeHistory + 1);
    expect(history.at(-1)).toMatchObject({
      id: planId,
      state: 'applied',
      finished_at: new Date(now + 4_000).toISOString(),
    });
    const rows = state.syncStatus.get(target)!.repositories;
    expect(rows.find((row) => row.repository === 'platform-infra')!.cells).toMatchObject({
      files: {
        state: 'proposed',
        proposal_url: 'https://github.com/smykla-skalski/platform-infra/pull/42',
      },
      labels: { state: 'applied' },
    });
    expect(rows.find((row) => row.repository === 'legacy-service')).toEqual(blocked);
    const snapshot = structuredClone({
      history,
      rows,
      queue: state.queue,
      events: state.syncQueueEvents,
    });
    expect(advanceMockSync(state, now + 600_000)).toBe(false);
    expect({ history, rows, queue: state.queue, events: state.syncQueueEvents }).toEqual(snapshot);
    expect(state.syncQueueEvents.get('queue-sync-apply')!.map((event) => event.kind)).toEqual([
      'started',
      'succeeded',
    ]);
  });

  it('keeps apply failures alongside successfully proposed files', () => {
    const state = seed(undefined, now);
    state.syncPlans.get(target)!.actions[0]!.error = 'GitHub refused the label';
    complete(state);
    expect(state.syncHistory.get(target)!.at(-1)!.state).toBe('failed');
    const cells = state.syncStatus
      .get(target)!
      .repositories.find((row) => row.repository === 'platform-infra')!.cells;
    expect(cells.labels).toMatchObject({
      state: 'check_failed',
      reason: 'GitHub refused the label',
    });
    expect(cells.files.state).toBe('proposed');
    expect(state.queue.find((item) => item.id === 'queue-sync-apply')).toMatchObject({
      state: 'failed',
    });
  });

  it('refuses to apply a plan after its saved inputs change', () => {
    const state = seed(undefined, now);
    state.syncStatus
      .get(target)!
      .repositories.find((row) => row.repository === 'platform-infra')!.cells.files.state =
      'outdated';
    const before = structuredClone(state.syncStatus.get(target));
    complete(state);
    expect(state.syncHistory.get(target)!.at(-1)!.state).toBe('stale');
    expect(state.syncStatus.get(target)).toEqual(before);
    expect(state.queue.find((item) => item.id === 'queue-sync-apply')!.state).toBe('cancelled');
  });

  it('scans completed results without re-proposing files or erasing declined and blocked evidence', () => {
    const state = seed(undefined, now);
    complete(state);
    const cells = state.syncStatus
      .get(target)!
      .repositories.find((row) => row.repository === 'platform-infra')!.cells;
    cells.files = {
      ...cells.files,
      state: 'declined',
      observed_outcome: 'declined',
      reason: 'Pull request closed without merging',
    };
    const item = scan(state, now + 10_000);
    expect(item.summary).toContain('found a declined proposal');
    expect(item.summary).toContain('could not proceed');
    expect(cells.files.state).toBe('declined');
    expect(cells.labels.state).toBe('in_step');
    expect(mockLiveSyncPlan(state, target)).toBeNull();
    expect(state.syncQueueEvents.get(item.id)!.map((event) => event.kind)).toEqual([
      'created',
      'action.run_now',
      'started',
      'succeeded',
    ]);
  });

  it('retains scan failures while creating a distinct plan only for supported differences', () => {
    const state = seed(undefined, now);
    state.syncPlans.delete(target);
    state.queue = state.queue.filter((item) => item.kind !== 'sync_apply');
    const cells = state.syncStatus
      .get(target)!
      .repositories.find((row) => row.repository === 'smyklot')!.cells;
    cells.files = { state: 'outdated', observed_at: new Date(now - 10_000).toISOString() };
    const item = scan(state);
    expect(item.state).toBe('succeeded');
    expect(item.summary).toContain('failed');
    expect(item.summary).toContain('changes queued');
    expect(cells.files).toMatchObject({ state: 'check_failed', observed_outcome: 'failed' });
    expect(cells.files.reason).toContain('no fresh repository evidence');
    const plan = mockLiveSyncPlan(state, target)!;
    expect(plan.state).toBe('approved');
    expect(plan.id).not.toBe('plan-1');
    expect(plan.queue_item!.id).not.toBe(item.id);
    expect(plan.queue_item!.source_id).toBe(plan.id);
    expect(plan.actions.every((action) => action.repository !== 'smyklot')).toBe(true);
  });

  it('does not revive a discarded plan or start a computed one', () => {
    const state = seed(undefined, now);
    state.syncPlans.get(target)!.state = 'computed';
    expect(advanceMockSync(state, now)).toBe(false);
    state.syncPlans.delete(target);
    expect(advanceMockSync(state, now)).toBe(true);
    expect(state.queue.find((item) => item.id === 'queue-sync-apply')!.state).toBe('cancelled');
    expect(advanceMockSync(state, now + 100_000)).toBe(false);
  });
});
