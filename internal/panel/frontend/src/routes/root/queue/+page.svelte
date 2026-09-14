<script lang="ts">
  import GeneralQueueView from '#lib/components/GeneralQueueView.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<div class="root-workspace">
  <GeneralQueueView
    api={session.api}
    canControl
    section={session.currentQueueSection}
    onSelectSection={(value) => session.selectRootQueueSection(value)}
  />
</div>
