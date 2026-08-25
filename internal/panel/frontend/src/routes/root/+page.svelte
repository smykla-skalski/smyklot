<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';
  import RootOverview from '#lib/components/RootOverview.svelte';

  const session = getPanelSession();
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <RootOverview
    api={session.api}
    rootRole={session.rootRole}
    installationsHref={session.rootInstallationsHref()}
    elevationsHref={session.rootAuditHref()}
    failuresHref={session.rootFailuresHref()}
    inboxHref={session.inboxHref()}
    onOpenInstallations={() => session.selectRootInstallations()}
    onOpenElevations={() => session.selectRootHistorySection('audit')}
    onOpenFailures={() => session.selectRootHistorySection('failures')}
    onOpenInbox={() => session.openInbox()}
    queueHref={session.queueHref()}
    onOpenQueue={() => session.selectRootSection('queue')}
  />
</section>
