<script lang="ts">
  import GeneralQueueView from '#lib/components/GeneralQueueView.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
  const session = getPanelSession();
</script>

{#key params.section}
  <section
    class="root-workspace"
    class:root-table-view={session.tableScrollView}
    aria-labelledby="root-page-heading"
  >
    <GeneralQueueView
      api={session.api}
      rootRole={session.rootRole}
      canControl
      section={session.currentQueueSection}
    />
  </section>
{/key}
