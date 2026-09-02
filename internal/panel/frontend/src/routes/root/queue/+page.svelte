<script lang="ts">
  import GeneralQueueView from '#lib/components/GeneralQueueView.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<section class="root-workspace" aria-labelledby="root-page-heading">
  <GeneralQueueView
    api={session.api}
    canControl
    section={session.currentQueueSection}
    onSelectSection={(value) => session.selectRootQueueSection(value)}
  />
</section>
