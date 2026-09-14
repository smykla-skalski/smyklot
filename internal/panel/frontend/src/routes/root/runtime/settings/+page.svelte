<script lang="ts">
  import RootSettings from '#lib/components/RootSettings.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<div class="root-workspace">
  <RootSettings
    section="settings"
    fetchSettings={session.api.fetchRootRuntimeSettings}
    saveSettings={session.api.saveRootRuntimeSettings}
  />
</div>
