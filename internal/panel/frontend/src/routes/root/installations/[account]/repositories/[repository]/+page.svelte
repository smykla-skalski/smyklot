<script lang="ts">
  import RootInstallations from '#lib/components/RootInstallations.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';
  import type { RootInstallationView } from '#lib/routes.js';

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
    hrefFor={(account: string, view: RootInstallationView) =>
      session.rootInstallationHref(account, view)}
    onList={() => session.selectRootInstallations()}
    onNavigate={(account: string, view: RootInstallationView) =>
      session.selectRootInstallation(account, view)}
    historySection={session.currentHistorySection}
  />
</section>
