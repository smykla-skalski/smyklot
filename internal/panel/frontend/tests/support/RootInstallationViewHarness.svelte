<script lang="ts">
  import { QueryClientProvider, type QueryClient } from '@tanstack/svelte-query';
  import { untrack, type ComponentProps } from 'svelte';

  import RootInstallationView from '../../src/lib/components/RootInstallationView.svelte';
  import {
    setSettingsDraftRegistry,
    type SettingsDraftRegistry,
  } from '../../src/lib/settings-drafts.svelte';

  const {
    drafts,
    queryClient,
    ...viewProps
  }: ComponentProps<typeof RootInstallationView> & {
    drafts: SettingsDraftRegistry;
    queryClient: QueryClient;
  } = $props();

  setSettingsDraftRegistry(untrack(() => drafts));
</script>

<QueryClientProvider client={queryClient}>
  <div class="app-shell root-mode">
    <RootInstallationView {...viewProps} />
  </div>
</QueryClientProvider>
