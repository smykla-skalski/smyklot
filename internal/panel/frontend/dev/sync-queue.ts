import type { MockState } from './fixtures.js';
import type { QueueEvent, QueueItem } from '../src/lib/types.js';

/** Sync records are occurrences. Their events stay attached after completion. */
export function recordMockSyncEvent(
  state: Pick<MockState, 'syncQueueEvents'>,
  item: QueueItem,
  kind: string,
  summary: string,
  at: string,
): void {
  let events = state.syncQueueEvents.get(item.id);
  if (!events) {
    events = [];
    state.syncQueueEvents.set(item.id, events);
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
