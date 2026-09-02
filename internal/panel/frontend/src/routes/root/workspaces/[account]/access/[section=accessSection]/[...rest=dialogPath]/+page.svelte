<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';
  import type { RootWorkspaceView } from '#lib/routes.js';
  import RootWorkspaces from '#lib/components/RootWorkspaces.svelte';

  const session = getPanelSession();
</script>

<section class="root-workspace" aria-labelledby="root-page-heading">
  <RootWorkspaces
    route={session.currentRootRoute}
    api={session.api}
    actorLogin={session.viewer?.account.login ?? ''}
    listHref={session.rootWorkspacesHref()}
    hrefFor={(account: string, view: RootWorkspaceView) => session.rootWorkspaceHref(account, view)}
    onList={() => session.selectRootWorkspaces()}
    onNavigate={(account: string, view: RootWorkspaceView) =>
      session.selectRootWorkspace(account, view)}
    historySection={session.currentHistorySection}
  />
</section>
