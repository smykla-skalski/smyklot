<script lang="ts">
  import { QueryClientProvider, type QueryClient } from '@tanstack/svelte-query';
  import { untrack, type ComponentProps } from 'svelte';

  import RootInstallations from '../../src/lib/components/RootInstallations.svelte';
  import {
    setSettingsDraftRegistry,
    type SettingsDraftRegistry,
  } from '../../src/lib/settings-drafts.svelte';

  const {
    drafts,
    queryClient,
    ...installationProps
  }: ComponentProps<typeof RootInstallations> & {
    drafts: SettingsDraftRegistry;
    queryClient: QueryClient;
  } = $props();

  setSettingsDraftRegistry(untrack(() => drafts));
</script>

<QueryClientProvider client={queryClient}>
  <div class="app-shell root-mode">
    <RootInstallations {...installationProps} />
  </div>
</QueryClientProvider>
