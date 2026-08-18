<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RootAccess from '#lib/components/RootAccess.svelte';
  import type { PanelInvitation, RootPanelUser } from '#lib/types.js';
  import Seeded from '../support/Seeded.svelte';
  import { INSTALLATIONS, INVITATIONS, ROOT_USERS } from '../support/fixtures.js';

  /*
   * Three keys, because this view runs three queries at once - the accounts, the
   * invitations behind its other tab, and the installation list its "add to
   * installation" dialog searches. Each carries every input to its request, so all
   * three are written out with the component's own initial `$state` beside them.
   */
  const USERS_KEY = ['root-access', 'users', '', 'name_asc', [], [], 20] as const;
  const INVITATIONS_KEY = ['root-access', 'invitations', '', 'created_newest', [], 20] as const;
  const INSTALLATIONS_KEY = ['root-installations'] as const;

  const page = <T,>(items: T[]) => ({
    pages: [{ items, next_cursor: null, total: items.length }],
    pageParams: [undefined],
  });

  const users = (items: RootPanelUser[] = ROOT_USERS) =>
    Promise.resolve({ items, next_cursor: null, total: items.length });
  const invitations = (items: PanelInvitation[] = INVITATIONS) =>
    Promise.resolve({ items, next_cursor: null, total: items.length });

  const base = {
    rootRole: 'Super Root',
    section: 'users' as const,
    onSection: fn(),
    fetchUsers: () => users(),
    updateUser: fn(),
    fetchInvitations: () => invitations(),
    createInvitation: fn(),
    reissueInvitation: fn(),
    revokeInvitation: fn(),
    canManageInvitations: true,
    actorLogin: 'bart',
    fetchInstallations: () => Promise.resolve(INSTALLATIONS),
    addInstallationUser: fn(),
    suggestUsers: () => Promise.resolve([]),
    onOpenInstallationAccess: fn(),
  };

  /* Typed rather than `as const`: `Seeded` wants a mutable pair per entry, and a
     `const` assertion makes the inner tuples readonly. */
  const seeded: [readonly unknown[], unknown][] = [
    [USERS_KEY, page(ROOT_USERS)],
    [INVITATIONS_KEY, page(INVITATIONS)],
    [INSTALLATIONS_KEY, INSTALLATIONS],
  ];

  const { Story } = defineMeta({
    title: 'Views/RootAccess',
    component: RootAccess,
    args: base,
  });
</script>

<!--
  Every account the panel knows, and what each may do with it. The two counts on the
  right - owned and assigned - are the Root console's own question and are the reason
  this is not the same table as an installation's user list.
-->
<Story name="Accounts">
  {#snippet template(args)}
    <Seeded seed={seeded}><RootAccess {...args} /></Seeded>
  {/snippet}
</Story>

<!--
  The invitations tab, which is the same table with different columns - one of the
  two halves that motivated `DataTable`.
-->
<Story name="Invitations">
  {#snippet template(args)}
    <Seeded seed={seeded}><RootAccess {...args} section="invitations" /></Seeded>
  {/snippet}
</Story>

<!--
  No account matches the search. The empty row keeps the header standing so the
  columns still say what was being looked for.
-->
<Story name="Empty">
  {#snippet template(args)}
    <Seeded
      seed={[
        [USERS_KEY, page([])],
        [INVITATIONS_KEY, page([])],
      ] as [readonly unknown[], unknown][]}
    >
      <RootAccess {...args} fetchUsers={() => users([])} />
    </Seeded>
  {/snippet}
</Story>

<!--
  A Root who may read but not invite. Search and filters stay - reading is still
  reading - and every control that would change something goes.
-->
<Story name="Read only">
  {#snippet template(args)}
    <Seeded seed={seeded}>
      <RootAccess {...args} rootRole="Root" canManageInvitations={false} />
    </Seeded>
  {/snippet}
</Story>
