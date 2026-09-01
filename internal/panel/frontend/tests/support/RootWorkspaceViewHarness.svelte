<script lang="ts">
  import { QueryClientProvider, type QueryClient } from '@tanstack/svelte-query';
  import { untrack, type ComponentProps } from 'svelte';

  import RootWorkspaceView from '../../src/lib/components/RootWorkspaceView.svelte';
  import {
    setSettingsDraftRegistry,
    type SettingsDraftRegistry,
  } from '../../src/lib/settings-drafts.svelte';

  const {
    drafts,
    queryClient,
    ...viewProps
  }: ComponentProps<typeof RootWorkspaceView> & {
    drafts: SettingsDraftRegistry;
    queryClient: QueryClient;
  } = $props();

  setSettingsDraftRegistry(untrack(() => drafts));
</script>

<QueryClientProvider client={queryClient}>
  <div class="app-shell root-mode">
    <RootWorkspaceView {...viewProps} />
  </div>
</QueryClientProvider>
