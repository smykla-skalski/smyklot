<script lang="ts">
  import RootWorkspaces from '#lib/components/RootWorkspaces.svelte';
  import { getPanelSession } from '#lib/session.svelte.js';
  import type { RootWorkspaceView } from '#lib/routes.js';

  const session = getPanelSession();
</script>

<!-- One repository's page inside the console, at the same shape of address it has
     in a workspace. The console draws its own chrome around it, so this is the
     workspace page it always was; the repository is read from the address. -->
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
