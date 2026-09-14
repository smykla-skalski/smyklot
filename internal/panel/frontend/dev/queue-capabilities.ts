import type { QueueActionType, QueueItem } from '../src/lib/types';

/** State safety shared by mock projections and writes. Keep narrower fixture policy. */
export function mockQueueActions(item: QueueItem): QueueActionType[] {
  if (['succeeded', 'failed', 'cancelled', 'superseded'].includes(item.state)) return [];
  return (item.actions ?? []).filter((action) => {
    if (item.state === 'running' || item.state === 'awaiting_approval')
      return action === 'set_priority';
    if (action === 'next_window' || action === 'schedule_at')
      return item.kind !== 'webhook_delivery';
    if (action === 'cancel') return !item.source_kind || item.source_kind === 'recurring';
    return true;
  });
}

export function projectMockQueueItem(item: QueueItem): QueueItem {
  return { ...item, actions: mockQueueActions(item) };
}
