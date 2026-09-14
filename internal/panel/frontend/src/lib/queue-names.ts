import type { QueueItem } from './types';

export function queueRepositoryName(
  item: Pick<QueueItem, 'repository_id' | 'repository_name'>,
  short = false,
): string | null {
  const name = item.repository_name?.trim();
  if (name) return short ? name.split('/').at(-1)! : name;
  return item.repository_id ? `Repository ${item.repository_id} (name unavailable)` : null;
}

export function queueProfileName(id: string, name?: string): string {
  if (id === 'immediate') return 'Immediate';
  return name?.trim() || `Hours profile ${id} (name unavailable)`;
}
