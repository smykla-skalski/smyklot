<script lang="ts">
  import { QueryClientProvider, type QueryClient } from '@tanstack/svelte-query';
  import type { ComponentProps } from 'svelte';

  import ServicePerformance from '../../src/lib/components/ServicePerformance.svelte';

  const {
    queryClient,
    ...performanceProps
  }: ComponentProps<typeof ServicePerformance> & { queryClient: QueryClient } = $props();
</script>

<QueryClientProvider client={queryClient}>
  <main class="app-shell">
    <ServicePerformance {...performanceProps} />
  </main>
</QueryClientProvider>
