<script lang="ts">
  import RootSettings from '#lib/components/RootSettings.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <RootSettings
    section="settings"
    rootRole={session.rootRole}
    fetchSettings={session.api.fetchRootRuntimeSettings}
    saveSettings={session.api.saveRootRuntimeSettings}
  />
</section>
