<script lang="ts">
  import QueueView from '$lib/components/QueueView.svelte';
  import { getPanelSession } from '$lib/session.svelte';

  const session = getPanelSession();
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <QueueView
    api={session.api}
    rootRole={session.rootRole}
    section="recent"
    onSection={(next) => session.selectQueueSection(next)}
    onOpenRequest={(request) => session.openQueueRequest(request)}
  />
</section>
