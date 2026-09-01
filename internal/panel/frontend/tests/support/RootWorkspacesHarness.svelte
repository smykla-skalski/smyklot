<script lang="ts">
  import { QueryClientProvider, type QueryClient } from '@tanstack/svelte-query';
  import { untrack, type ComponentProps } from 'svelte';

  import RootWorkspaces from '../../src/lib/components/RootWorkspaces.svelte';
  import {
    setSettingsDraftRegistry,
    type SettingsDraftRegistry,
  } from '../../src/lib/settings-drafts.svelte';

  const {
    drafts,
    queryClient,
    ...workspaceProps
  }: ComponentProps<typeof RootWorkspaces> & {
    drafts: SettingsDraftRegistry;
    queryClient: QueryClient;
  } = $props();

  setSettingsDraftRegistry(untrack(() => drafts));
</script>

<QueryClientProvider client={queryClient}>
  <div class="app-shell root-mode">
    <RootWorkspaces {...workspaceProps} />
  </div>
</QueryClientProvider>
