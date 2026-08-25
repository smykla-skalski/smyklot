/** Queue list queries shared by the full Queue and its Root Overview summary. */
export const ROOT_OVERVIEW_ACTIVE_QUEUE =
  '?limit=3&order=dispatch&state=scheduled,blocked,ready,running,retrying';
export const ROOT_OVERVIEW_APPROVAL_QUEUE = '?limit=1&state=awaiting_approval';
export const ROOT_OVERVIEW_REVIEW_QUEUE = '?limit=1&state=blocked,retrying';

export function queueListScopeKey(targetId?: string): readonly string[] {
  return targetId === undefined ? ['queue', 'root'] : ['queue', 'target', targetId];
}

export function queueListKey(targetId: string | undefined, query: string): readonly string[] {
  return [...queueListScopeKey(targetId), query];
}

export function queueDetailScopeKey(targetId?: string): readonly string[] {
  return targetId === undefined ? ['queue-detail', 'root'] : ['queue-detail', 'target', targetId];
}

export function queueDetailKey(targetId: string | undefined, itemId: string): readonly string[] {
  return [...queueDetailScopeKey(targetId), itemId];
}
