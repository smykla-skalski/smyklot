import { expect, it } from 'vitest';
import { queueSeeds } from '../dev/fixtures';
import { pruneMockOccurrences, scheduleMockOccurrence } from '../dev/queue-occurrences';
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

it('bounds generated history while retaining seeds and active work', () => {
  const seed = queueSeeds(() => new Date(now).toISOString())[3]!;
  const history = Array.from({ length: 205 }, (_, index) => ({
    ...seed,
    id: `scan:occurrence:${index}`,
    state: 'succeeded' as const,
    finished_at: new Date(now + index).toISOString(),
  }));
  const live = { ...seed, id: 'scan:occurrence:live' };
  const state = {
    queue: [seed, live, ...history],
    queueRest: new Map(history.map((item) => [item.id, item])),
    queueLoop: new Set(history.map((item) => item.id)),
  };
  expect(pruneMockOccurrences(state)).toBe(true);
  expect(state.queue).toHaveLength(202);
  expect(state.queue).toContain(seed);
  expect(state.queue).toContain(live);
  expect(state.queue).not.toContain(history[0]);
  expect(state.queue).toContain(history[204]);
  expect(state.queueRest.has(history[0]!.id)).toBe(false);
  expect(state.queueLoop.has(history[0]!.id)).toBe(false);
  expect(pruneMockOccurrences(state)).toBe(false);
});
