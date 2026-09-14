import { expect, it } from 'vitest';
import { mockQueueActions, projectMockQueueItem } from '../dev/queue-capabilities';
import { queueSeeds } from '../dev/fixtures';
const base = queueSeeds(() => '2026-09-14T12:00:00Z')[1]!;
it.each(['succeeded', 'failed', 'cancelled', 'superseded'] as const)(
  'removes stale actions from %s work',
  (state) => {
    expect(mockQueueActions({ ...base, state })).toEqual([]);
  },
);
it.each(['running', 'awaiting_approval'] as const)('only permits priority for %s work', (state) => {
  expect(mockQueueActions({ ...base, state })).toEqual(['set_priority']);
});
it('preserves a narrower source policy', () => {
  expect(mockQueueActions({ ...base, actions: [] })).toEqual([]);
});
it('excludes window changes for webhook delivery and source cancellation', () => {
  expect(
    mockQueueActions({
      ...base,
      kind: 'webhook_delivery',
      details: undefined,
      source_kind: 'delivery',
    }),
  ).toEqual(['run_now', 'set_priority']);
});
it('retains the waiting recurring occurrence controls', () => {
  expect(mockQueueActions({ ...base, source_kind: 'recurring' })).toEqual(base.actions);
});

it('clears blocked estimates without mutating the stored mock occurrence', () => {
  const item = {
    ...base,
    state: 'blocked' as const,
    estimated_start_at: '2026-09-14T13:00:00Z',
    work_ahead: 7,
  };
  const projected = projectMockQueueItem(item);
  expect(projected.estimated_start_at).toBeUndefined();
  expect(projected.work_ahead).toBe(0);
  expect(item.estimated_start_at).toBe('2026-09-14T13:00:00Z');
  expect(item.work_ahead).toBe(7);
  expect(projectMockQueueItem({ ...item, state: 'scheduled' }).estimated_start_at).toBe(
    item.estimated_start_at,
  );
});
