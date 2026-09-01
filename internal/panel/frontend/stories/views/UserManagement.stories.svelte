<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import UserManagement from '#lib/components/UserManagement.svelte';
  import { INVITATIONS, TARGET, USERS } from '../support/fixtures.js';

  /* Every fetcher is an ordinary prop, so nothing is seeded - the queries resolve
     against these. Seed a cache only where data arrives through `api`. */
  const base = {
    section: 'users' as const,
    targetId: TARGET.id,
    targetName: TARGET.account.display_name,
    actorLogin: 'bart',
    actorTargetRole: 'owner' as const,
    onSection: fn(),
    fetchTargetUsers: () =>
      Promise.resolve({ items: USERS, next_cursor: null, total: USERS.length }),
    addTargetUser: fn(),
    suggestUsers: () => Promise.resolve([]),
    updateTargetUser: fn(),
    fetchTargetInvitations: () =>
      Promise.resolve({ items: INVITATIONS, next_cursor: null, total: INVITATIONS.length }),
    createTargetInvitation: fn(),
    reissueInvitation: fn(),
    revokeInvitation: fn(),
    /* A plain list, not a page - the decision history is short by construction. */
    fetchUserDecisions: () => Promise.resolve([]),
  };

  const { Story } = defineMeta({
    title: 'Views/UserManagement',
    component: UserManagement,
    args: base,
  });
</script>

<!--
  Who may act in this workspace. A row with decision history opens it, and only
  that row is pressable - the chevron is drawn always so its neighbours' menus land
  at the same x, and the arrow only appears where there is somewhere to go.
-->
<Story name="Users">
  {#snippet template(args)}<UserManagement {...args} />{/snippet}
</Story>

<!--
  The invitations half. It is the same list saying different things about each row -
  a person who has accepted, and a person who has been asked.
-->
<Story name="Invitations">
  {#snippet template(args)}<UserManagement {...args} section="invitations" />{/snippet}
</Story>

<!-- Nobody added yet. -->
<Story name="Empty">
  {#snippet template(args)}
    <UserManagement
      {...args}
      fetchTargetUsers={() => Promise.resolve({ items: [], next_cursor: null, total: 0 })}
    />
  {/snippet}
</Story>

<!--
  A member who may read the list but not change it. Every control that would act goes;
  the columns stay, because who may do what is still worth reading.
-->
<Story name="Read only">
  {#snippet template(args)}
    <UserManagement {...args} readOnly actorTargetRole="viewer" />
  {/snippet}
</Story>
