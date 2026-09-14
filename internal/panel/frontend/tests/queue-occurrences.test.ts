import { expect, it } from 'vitest';
import { queueSeeds } from '../dev/fixtures';
import { scheduleMockOccurrence } from '../dev/queue-occurrences';
const now = Date.parse('2026-09-14T12:00:00Z');
it('keeps completed history and creates exactly one fresh occurrence', () => {
  const template = queueSeeds(() => new Date(now).toISOString())[3]!;
  const item = {
    ...template,
    state: 'succeeded' as const,
    finished_at: new Date(now).toISOString(),
  };
  const before = structuredClone(item);
  const state = {
    queue: [item],
    queueRest: new Map([[item.id, template]]),
    queueLoop: new Set([item.id]),
  };
  expect(scheduleMockOccurrence(state, item, now, 45000)).toBe(true);
  expect(scheduleMockOccurrence(state, item, now, 45000)).toBe(false);
  expect(state.queue[0]).toEqual(before);
  const next = state.queue[1]!;
  expect(next.id).not.toBe(item.id);
  expect(next.source_id).toBe(item.source_id);
  expect(next.state).toBe('scheduled');
  expect(next.finished_at).toBeUndefined();
  expect(next.attempt).toBe(0);
  expect(next.revision).toBe(1);
  expect(Date.parse(next.eligible_at)).toBe(now + 45000);
});
it.each(['cancelled', 'failed', 'superseded'] as const)('does not restart %s work', (status) => {
  const item = { ...queueSeeds(() => new Date(now).toISOString())[3]!, state: status };
  const state = {
    queue: [item],
    queueRest: new Map([[item.id, item]]),
    queueLoop: new Set([item.id]),
  };
  expect(scheduleMockOccurrence(state, item, now, 45000)).toBe(false);
  expect(state.queue).toHaveLength(1);
});
