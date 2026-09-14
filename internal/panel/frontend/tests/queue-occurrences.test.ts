import { expect, it } from 'vitest';
import { seedMockQueueEvents } from '../dev/queue-events';
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
    queueEvents: new Map(),
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
  expect(state.queueEvents.get(next.id)).toMatchObject([
    { kind: 'created', state: 'scheduled', created_at: next.created_at },
  ]);
  expect(next.finished_at).toBeUndefined();
  expect(next.attempt).toBe(0);
  expect(next.revision).toBe(1);
  expect(Date.parse(next.eligible_at)).toBe(now + 45000);
});
it.each(['cancelled', 'failed', 'superseded'] as const)('does not restart %s work', (status) => {
  const item = { ...queueSeeds(() => new Date(now).toISOString())[3]!, state: status };
  const state = {
    queueEvents: new Map(),
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
    queueEvents: seedMockQueueEvents([seed, live, ...history]),
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
  expect(state.queueEvents.has(history[0]!.id)).toBe(false);
  expect(state.queueEvents.has(history[204]!.id)).toBe(true);
  expect(pruneMockOccurrences(state)).toBe(false);
});

it('retains cancellation history while scheduling one later recurring occurrence', () => {
  const template = queueSeeds(() => new Date(now).toISOString())[3]!;
  const item = {
    ...template,
    state: 'cancelled' as const,
    finished_at: new Date(now).toISOString(),
    actions: [],
  };
  const before = structuredClone(item);
  const state = {
    queueEvents: new Map(),
    queue: [item],
    queueRest: new Map([[item.id, template]]),
    queueLoop: new Set<string>(),
  };
  expect(scheduleMockOccurrence(state, item, now, 45000)).toBe(true);
  expect(scheduleMockOccurrence(state, item, now, 45000)).toBe(false);
  expect(state.queue[0]).toEqual(before);
  expect(state.queue).toHaveLength(2);
  expect(state.queue[1]).toMatchObject({
    state: 'scheduled',
    source_id: item.source_id,
    attempt: 0,
    revision: 1,
  });
  expect(state.queue[1]!.id).not.toBe(item.id);
  expect(Date.parse(state.queue[1]!.eligible_at)).toBe(now + 45000);
  expect(state.queue[1]!.actions).toEqual(template.actions);
});
