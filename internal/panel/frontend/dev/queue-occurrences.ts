import type { MockState } from './fixtures';
import type { QueueItem } from '../src/lib/types';

/** A completed occurrence stays immutable while its next scheduled record is created. */
export function scheduleMockOccurrence(
  state: Pick<MockState, 'queue' | 'queueRest' | 'queueLoop'>,
  item: QueueItem,
  now: number,
  waitMs: number,
): boolean {
  const template = state.queueRest.get(item.id);
  if (
    !template ||
    item.state !== 'succeeded' ||
    item.source_kind !== 'recurring' ||
    !state.queueLoop.has(item.id)
  )
    return false;
  const next: QueueItem = {
    ...template,
    id: `${item.source_id ?? template.id}:occurrence:${now}`,
    state: 'scheduled',
    source_id: item.source_id ?? template.id,
    started_at: undefined,
    finished_at: undefined,
    blocked_reason: undefined,
    reason: undefined,
    immediate: false,
    window_mode: 'respect',
    attempt: 0,
    revision: 1,
    progress_current: 0,
    created_at: new Date(now).toISOString(),
    updated_at: new Date(now).toISOString(),
    not_before: new Date(now + waitMs).toISOString(),
    eligible_at: new Date(now + waitMs).toISOString(),
    estimated_start_at: new Date(now + waitMs).toISOString(),
  };
  state.queueRest.delete(item.id);
  state.queueLoop.delete(item.id);
  state.queueRest.set(next.id, next);
  state.queue.push(next);
  return true;
}

/** Bound generated demo history, preserving seeds, live work and other domains. */
export function pruneMockOccurrences(
  state: Pick<MockState, 'queue' | 'queueRest' | 'queueLoop'>,
): boolean {
  const history = state.queue
    .filter(
      (item) =>
        item.source_kind === 'recurring' &&
        item.id.includes(':occurrence:') &&
        ['succeeded', 'failed', 'cancelled', 'superseded'].includes(item.state),
    )
    .sort(
      (a, b) =>
        Date.parse(b.finished_at ?? b.updated_at) - Date.parse(a.finished_at ?? a.updated_at),
    );
  const removed = new Set(history.slice(200).map((item) => item.id));
  if (removed.size === 0) return false;
  state.queue = state.queue.filter((item) => !removed.has(item.id));
  for (const id of removed) {
    state.queueRest.delete(id);
    state.queueLoop.delete(id);
  }
  return true;
}
