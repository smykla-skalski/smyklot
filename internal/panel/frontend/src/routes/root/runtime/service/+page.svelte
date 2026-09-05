<script lang="ts">
  import RootSettings from '#lib/components/RootSettings.svelte';
  import ServicePerformance from '#lib/components/ServicePerformance.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<section class="root-workspace" aria-labelledby="root-page-heading">
  <div class="card-stack">
    <RootSettings section="service" fetchSettings={session.api.fetchRootRuntimeSettings} />
    <ServicePerformance fetchPerformance={session.api.fetchRootPerformance} />
  </div>
</section>
