<script lang="ts">
  import { untrack, type ComponentProps } from 'svelte';

  import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  import SyncView from '../../src/lib/components/SyncView.svelte';
  import {
    setSettingsDraftRegistry,
    type SettingsDraftRegistry,
  } from '../../src/lib/settings-drafts.svelte';

  const {
    drafts,
    ...viewProps
  }: ComponentProps<typeof SyncView> & { drafts: SettingsDraftRegistry } = $props();

  setSettingsDraftRegistry(untrack(() => drafts));
</script>

<QueryClientProvider {client}><SyncView {...viewProps} /></QueryClientProvider>
