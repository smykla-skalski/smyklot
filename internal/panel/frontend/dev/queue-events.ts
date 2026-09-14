import type { MockState } from './fixtures.js';
import type { QueueEvent, QueueItem } from '../src/lib/types.js';

/** Events stay attached to their occurrence after completion. */
export function recordMockQueueEvent(
  state: Pick<MockState, 'queueEvents'>,
  item: QueueItem,
  kind: string,
  summary: string,
  at: string,
): void {
  let events = state.queueEvents.get(item.id);
  if (!events) {
    events = [];
    state.queueEvents.set(item.id, events);
  }
  const event: QueueEvent = {
    id: events.length + 1,
    item_id: item.id,
    actor: 'Development simulation',
    kind,
    state: item.state,
    summary,
    created_at: at,
  };
  events.push(event);
}

/** Establish deterministic fixture history once, before the simulation advances. */
export function seedMockQueueEvents(items: QueueItem[]): Map<string, QueueEvent[]> {
  const state = { queueEvents: new Map<string, QueueEvent[]>() };
  for (const item of items) {
    recordMockQueueEvent(
      state,
      { ...item, state: 'scheduled' },
      'created',
      `Queued ${item.title}`,
      item.created_at,
    );
    if (item.started_at) {
      recordMockQueueEvent(
        state,
        { ...item, state: 'running' },
        'started',
        `Started ${item.title}`,
        item.started_at,
      );
    }
    if (item.state !== 'scheduled' && !(item.state === 'running' && item.started_at)) {
      recordMockQueueEvent(
        state,
        item,
        item.state,
        item.blocked_reason ?? `${item.title}: ${item.state}`,
        item.finished_at ?? item.updated_at,
      );
    }
  }
  return state.queueEvents;
}
