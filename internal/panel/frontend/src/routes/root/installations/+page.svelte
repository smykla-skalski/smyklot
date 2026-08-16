<script lang="ts">
  import { getPanelSession } from '$lib/session.svelte';
  import RootInstallations from '$lib/components/RootInstallations.svelte';

  const session = getPanelSession();
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
    listHref={session.rootInstallationsHref()}
    hrefFor={(a: string, v: string) => session.rootInstallationHref(a, v as never)}
    onList={() => session.selectRootInstallations()}
    onNavigate={(a: string, v: string) => session.selectRootInstallation(a, v as never)}
    historySection={session.currentHistorySection}
    onHistorySection={(s: 'audit' | 'failures') => session.selectRootInstallationHistory(s)}
  />
</section>
