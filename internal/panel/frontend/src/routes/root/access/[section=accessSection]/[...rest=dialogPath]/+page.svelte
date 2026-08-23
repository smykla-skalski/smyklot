<script lang="ts">
  import { getPanelSession } from '#lib/session.svelte.js';
  import RootAccess from '#lib/components/RootAccess.svelte';

  import type { PageProps } from './$types';

  const { params }: PageProps = $props();
  const session = getPanelSession();
  const section = $derived(params.section);
</script>

<section
  class="root-workspace"
  class:root-table-view={session.tableScrollView}
  aria-labelledby="root-page-heading"
>
  <RootAccess
    rootRole={session.rootRole}
    {section}
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
