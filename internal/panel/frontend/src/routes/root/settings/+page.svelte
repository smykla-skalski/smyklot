<script lang="ts">
  import { getContext } from 'svelte';
  import type { PanelSession } from '$lib/session.svelte';
  import RootSettings from '$lib/components/RootSettings.svelte';

  const session = getContext<PanelSession>('panel-session');
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <RootSettings
    rootRole={session.rootRole}
    refreshVersion={session.runtimeSettingsVersion}
    fetchSettings={session.api.fetchRootRuntimeSettings}
    updateSettings={session.api.updateRootRuntimeSettings}
  />
</section>
