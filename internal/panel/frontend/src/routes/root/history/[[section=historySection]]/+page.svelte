<script lang="ts">
  import { page } from '$app/state';
  import { getPanelSession } from '$lib/session.svelte';
  import HistoryPanel from '$lib/components/HistoryPanel.svelte';

  const session = getPanelSession();
  const section = $derived((page.params.section ?? 'audit') as 'audit' | 'failures');
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
