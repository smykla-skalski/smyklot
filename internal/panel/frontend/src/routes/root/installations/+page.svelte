<script lang="ts">
  import { getContext } from 'svelte';
  import type { PanelSession } from '$lib/session.svelte';
  import RootInstallations from '$lib/components/RootInstallations.svelte';

  const session = getContext<PanelSession>('panel-session');
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <RootInstallations
    route={session.currentRootRoute}
    api={session.api}
    rootRole={session.rootRole}
    actorLogin={session.viewer?.account.login ?? ''}
    refreshVersion={session.rootDataVersion}
    listHref={session.rootInstallationsHref()}
    hrefFor={(a: string, v: string) => session.rootInstallationHref(a, v as never)}
    onList={() => session.selectRootInstallations()}
    onNavigate={(a: string, v: string) => session.selectRootInstallation(a, v as never)}
    historySection={session.currentHistorySection}
    onHistorySection={(s: 'audit' | 'failures') => session.selectRootInstallationHistory(s)}
  />
</section>
