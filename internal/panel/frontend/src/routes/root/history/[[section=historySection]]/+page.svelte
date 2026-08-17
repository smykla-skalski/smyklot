<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';
  import HistoryPanel from '#lib/components/HistoryPanel.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
  const session = getPanelSession();
  // History opens on its first table when the address does not name one.
  const section = $derived(params.section ?? 'audit');
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <HistoryPanel
    context="root"
    targetId="root"
    rootRole={session.rootRole}
    {section}
    onSection={(s: 'audit' | 'failures') => session.selectRootHistorySection(s)}
    fetchAudit={session.api.fetchRootAudit}
    fetchFailures={session.api.fetchRootFailures}
  />
</section>
