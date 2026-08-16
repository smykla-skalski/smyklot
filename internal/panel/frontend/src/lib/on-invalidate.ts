/**
 * Subscribe to query invalidation and call a callback when matching queries
 * are invalidated. Replaces the `refreshVersion` prop pattern: instead of
 * bumping a counter that components watch, the stream handler invalidates
 * query keys, and components subscribe to the keys they care about.
 */
import { onDestroy, onMount, untrack } from 'svelte';

import type { QueryClient } from '@tanstack/svelte-query';

export function onInvalidate(
  queryClient: QueryClient,
  keyPrefix: readonly unknown[],
  callback: () => void,
): () => void {
  let unsubscribe: (() => void) | undefined;

  onMount(() => {
    unsubscribe = queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== 'updated') return;
      if (event.action.type !== 'invalidate') return;
      const queryKey = event.query.queryKey;
      if (keyPrefix.length > queryKey.length) return;
      if (!keyPrefix.every((k, i) => queryKey[i] === k)) return;
      untrack(callback);
    });
  });

  onDestroy(() => {
    unsubscribe?.();
    unsubscribe = undefined;
  });

  return () => {
    unsubscribe?.();
    unsubscribe = undefined;
  };
}
