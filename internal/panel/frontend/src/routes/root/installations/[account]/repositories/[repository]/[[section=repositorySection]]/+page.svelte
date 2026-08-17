<script lang="ts">
  import RootInstallations from '#lib/components/RootInstallations.svelte';
  import type { RootInstallationView } from '#lib/routes.js';
  import { getPanelSession } from '#lib/session.svelte.js';

  const session = getPanelSession();
</script>

<!-- One repository's page inside the console, at the same shape of address it has
     in a workspace. The console draws its own chrome around it, so this is the
     installation page it always was; the repository is read from the address. -->
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
    hrefFor={(a: string, v: RootInstallationView) => session.rootInstallationHref(a, v)}
    onList={() => session.selectRootInstallations()}
    onNavigate={(a: string, v: RootInstallationView) => session.selectRootInstallation(a, v)}
    historySection={session.currentHistorySection}
    onHistorySection={(s: 'audit' | 'failures') => session.selectRootInstallationHistory(s)}
  />
</section>
