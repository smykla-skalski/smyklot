<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';
  import RootSettings from '#lib/components/RootSettings.svelte';

  const session = getPanelSession();
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <RootSettings
    rootRole={session.rootRole}
    fetchSettings={session.api.fetchRootRuntimeSettings}
    updateSettings={session.api.updateRootRuntimeSettings}
  />
</section>
