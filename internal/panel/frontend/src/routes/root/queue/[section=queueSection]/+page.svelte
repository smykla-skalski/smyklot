<script lang="ts">
  import GeneralQueueView from '#lib/components/GeneralQueueView.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
  const session = getPanelSession();
</script>

{#key params.section}
  <div class="root-workspace">
    <GeneralQueueView
      api={session.api}
      canControl
      section={session.currentQueueSection}
      onSelectSection={(value) => session.selectRootQueueSection(value)}
    />
  </div>
{/key}
