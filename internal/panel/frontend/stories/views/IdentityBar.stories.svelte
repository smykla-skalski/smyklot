<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import IdentityBar from '#lib/components/IdentityBar.svelte';
  import type { PanelTarget } from '#lib/types.js';
  import { ACCOUNT, TARGET } from '../support/fixtures.js';

  /*
   * The shell's own furniture, and the one component here that takes everything as a
   * prop - no session, no queries. That is why it is the sidebar and not a page: it
   * says where you are and lets you go elsewhere, and deciding either is its caller's
   * job.
   */
  const base = {
    viewer: {
      account: ACCOUNT,
      system_role: 'super_root' as const,
      status: 'active' as const,
      target_count: 1,
    },
    targets: [TARGET],
    selectedId: TARGET.id,
    targetHref: (target: PanelTarget) => `#/i/${target.account.login}`,
    onSelectTarget: fn(),
    onSignOut: fn(),
    view: 'repositories' as const,
    viewHref: (view: string) => `#/i/${TARGET.account.login}/${view}`,
    onSelectView: fn(),
    showUsers: true,
    showViews: true,
    showNavigation: true,
    collapsed: false,
    onToggleCollapsed: fn(),
    theme: 'dark' as const,
    onSelectTheme: fn(),
    rootMode: false,
    rootValue: 'overview' as const,
    rootHrefFor: (section: string) => `#/root/${section}`,
    onSelectRoot: fn(),
    rootDashboardHref: '#/root',
    onEnterRoot: fn(),
    returnHref: '#/',
    onReturnToPanel: fn(),
    inboxHref: '#/inbox',
    inboxActive: false,
    onSelectInbox: fn(),
    unreadCount: 3,
  };

  const { Story } = defineMeta({
    title: 'Views/IdentityBar',
    component: IdentityBar,
    args: base,
  });
</script>

<!-- The panel's sidebar: the mark, the workspace, the views, and the account below. -->
<Story name="Panel">
  {#snippet template(args)}<IdentityBar {...args} />{/snippet}
</Story>

<!-- Collapsed to its rail. Every row keeps its icon and loses only its word. -->
<Story name="Collapsed">
  {#snippet template(args)}<IdentityBar {...args} collapsed />{/snippet}
</Story>

<!--
  In the Root console, which is a different palette rather than a different shape -
  flip the console toolbar to see the aliases the shell re-declares.
-->
<Story name="Root console">
  {#snippet template(args)}<IdentityBar {...args} rootMode />{/snippet}
</Story>

<!-- Nothing installed yet, so there is no workspace to switch between. -->
<Story name="No workspaces">
  {#snippet template(args)}
    <IdentityBar {...args} targets={[]} selectedId={null} showViews={false} />
  {/snippet}
</Story>
