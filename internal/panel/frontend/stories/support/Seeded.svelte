<script lang="ts">
  import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
  import type { Snippet } from 'svelte';

  /**
   * A query cache with answers already in it.
   *
   * The query-connected views fetch through `createQuery`, and a story has no service
   * to answer them. Seeding the cache is better than stubbing the fetch: the view
   * takes the path it takes when data has *arrived*, which is the state worth looking
   * at, and nothing is left in flight to resolve later and move the page.
   *
   * `retry: false` and an infinite `staleTime` because a story must not refetch. It
   * would hit `stubApi`, which rejects on purpose, and the story would flip to its
   * error state a moment after opening - which reads as a bug in the component.
   *
   * Nested inside `PanelShell`'s own provider, and the innermost one wins.
   */
  const {
    seed = [],
    children,
  }: {
    /** `[queryKey, data]` pairs, exactly as the view asks for them. */
    seed?: ReadonlyArray<[readonly unknown[], unknown]>;
    children: Snippet;
  } = $props();

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
  });
  /* Read once, deliberately: a story's seed is fixed, and re-seeding a live cache
     when a control changes would write over whatever the view had done with it. */
  // svelte-ignore state_referenced_locally
  for (const [key, data] of seed) client.setQueryData(key, data);
</script>

<QueryClientProvider {client}>
  {@render children()}
</QueryClientProvider>
