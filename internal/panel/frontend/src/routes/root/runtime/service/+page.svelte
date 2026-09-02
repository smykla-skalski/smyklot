<script lang="ts">
  import RootSettings from '#lib/components/RootSettings.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<section class="root-workspace" aria-labelledby="root-page-heading">
  <RootSettings section="service" fetchSettings={session.api.fetchRootRuntimeSettings} />
</section>
