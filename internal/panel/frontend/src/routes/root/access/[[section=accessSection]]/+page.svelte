<script lang="ts">
  import { page } from '$app/state';
  import { getContext } from 'svelte';
  import type { PanelSession } from '$lib/session.svelte';
  import RootAccess from '$lib/components/RootAccess.svelte';

  const session = getContext<PanelSession>('panel-session');
  const section = (page.params.section ?? 'users') as 'users' | 'invitations';
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <RootAccess
    rootRole={session.rootRole}
    {section}
    refreshVersion={session.rootDataVersion}
    onSection={(s: 'users' | 'invitations') => session.selectRootAccessSection(s)}
    fetchUsers={session.api.fetchRootUsers}
    updateUser={session.api.updateRootUser}
    fetchInvitations={session.api.fetchRootInvitations}
    createInvitation={session.api.createRootInvitation}
    reissueInvitation={session.api.reissueRootInvitation}
    revokeInvitation={session.api.revokeRootInvitation}
    canManageInvitations={session.viewer?.system_role === 'super_root'}
    actorLogin={session.viewer?.account.login ?? ''}
    fetchInstallations={session.api.fetchRootInstallations}
    addInstallationUser={session.api.addRootTargetUser}
    suggestUsers={session.api.suggestRootTargetUsers}
    onOpenInstallationAccess={(account: string) => session.selectRootInstallation(account, 'users')}
  />
</section>
