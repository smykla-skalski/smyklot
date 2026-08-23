<script lang="ts">
  import { QueryClientProvider, type QueryClient } from '@tanstack/svelte-query';
  import type { ComponentProps } from 'svelte';

  import HistoryPanel from '../../src/lib/components/HistoryPanel.svelte';

  const {
    queryClient,
    ...historyProps
  }: ComponentProps<typeof HistoryPanel> & { queryClient: QueryClient } = $props();
</script>

<QueryClientProvider client={queryClient}>
  <main class="app-shell">
    <HistoryPanel {...historyProps} />
  </main>
</QueryClientProvider>
