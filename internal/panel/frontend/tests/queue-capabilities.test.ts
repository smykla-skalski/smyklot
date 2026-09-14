import { expect, it } from 'vitest';
import { mockQueueActions } from '../dev/queue-capabilities';
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
