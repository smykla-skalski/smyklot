import { expect, it } from 'vitest';
import { queueSeeds } from '../dev/fixtures';
import { recordMockQueueEvent, seedMockQueueEvents } from '../dev/queue-events';
const now = Date.parse('2026-09-14T12:00:00Z');
const items = queueSeeds((offset) => new Date(now + offset).toISOString());
it('separates fixture creation, execution and completion timestamps', () => {
  const item = items.find((item) => item.id === 'queue-catalog-complete')!;
  const events = seedMockQueueEvents(items).get(item.id)!;
  expect(events.map((event) => [event.kind, event.state, event.created_at])).toEqual([
    ['created', 'scheduled', item.created_at],
    ['started', 'running', item.started_at],
    ['succeeded', 'succeeded', item.finished_at],
  ]);
});
it('retains original event facts after later actions and mutable item updates', () => {
  const item = { ...items[3]! };
  const state = { queueEvents: seedMockQueueEvents([item]) };
  const original = structuredClone(state.queueEvents.get(item.id)!);
  item.state = 'ready';
  item.updated_at = new Date(now).toISOString();
  recordMockQueueEvent(state, item, 'run_now', 'Run requested', item.updated_at);
  item.state = 'running';
  item.started_at = new Date(now + 1000).toISOString();
  recordMockQueueEvent(state, item, 'started', 'Started', item.started_at);
  item.updated_at = new Date(now + 5000).toISOString();
  const events = state.queueEvents.get(item.id)!;
  expect(events.slice(0, original.length)).toEqual(original);
  expect(events.at(-2)).toMatchObject({ state: 'ready', created_at: new Date(now).toISOString() });
  expect(events.at(-1)).toMatchObject({ state: 'running', created_at: item.started_at });
  expect(new Set(events.map((event) => event.id)).size).toBe(events.length);
});
