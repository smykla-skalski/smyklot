<script lang="ts">
  import { page } from '$app/state';
  import { getContext } from 'svelte';
  import type { PanelSession } from '$lib/session.svelte';
  import HistoryPanel from '$lib/components/HistoryPanel.svelte';

  const session = getContext<PanelSession>('panel-session');
  const section = (page.params.section ?? 'audit') as 'audit' | 'failures';
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
    refreshVersion={session.rootDataVersion}
    fetchAudit={session.api.fetchRootAudit}
    fetchFailures={session.api.fetchRootFailures}
  />
</section>
