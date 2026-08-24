<script lang="ts">
  import SchedulesView from '#lib/components/SchedulesView.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<section class="root-workspace" aria-labelledby="root-page-heading">
  <SchedulesView
    api={session.api}
    rootRole={session.rootRole}
    refreshRevision={session.queueRevision}
  />
</section>
