<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';
  import type { HistorySection, RootInstallationView } from '#lib/routes.js';
  import RootInstallations from '#lib/components/RootInstallations.svelte';

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
    hrefFor={(a: string, v: RootInstallationView, s?: HistorySection) =>
      session.rootInstallationHref(a, v, s)}
    onList={() => session.selectRootInstallations()}
    onNavigate={(a: string, v: RootInstallationView) => session.selectRootInstallation(a, v)}
    historySection={session.currentHistorySection}
    onHistorySection={(s: 'audit' | 'failures') => session.selectRootInstallationHistory(s)}
  />
</section>
