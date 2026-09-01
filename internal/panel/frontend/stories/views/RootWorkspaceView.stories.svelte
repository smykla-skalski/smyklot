<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RootWorkspaceView from '#lib/components/RootWorkspaceView.svelte';
  import { fixtureApi } from '../support/api.js';
  import { ROOT_WORKSPACE, ROOT_TARGET } from '../support/fixtures.js';

  /*
   * The Root console's reading of one workspace. It takes its `api` as a prop, so
   * this could be a `stubApi` naming each method - but it draws the same four views
   * `WorkspaceView` does, and each reaches its own set. `fixtureApi` answers all of
   * them from the mock's data, which is the point of having one.
   */
  const workspace = ROOT_WORKSPACE;

  const base = {
    workspace,
    view: 'repositories' as const,
    api: fixtureApi({ fetchRootTargetSettings: async () => ROOT_TARGET }),
    actorLogin: 'bart',
    historySection: 'audit' as const,
    onHistorySection: fn(),
    listHref: '#/root/workspaces',
    hrefFor: (account: string, view: string) => `#/root/workspaces/${account}/${view}`,
    onList: fn(),
    onNavigate: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/RootWorkspaceView',
    component: RootWorkspaceView,
    args: base,
  });
</script>

<!--
  What the Root console shows before it has elevated into a workspace, which is
  what a Root sees first and most often: the identity, the tabs, the way back to the
  catalogue, and readable diagnostics with every write control locked. The access
  button is the only thing on the page that acts.

  The back link is an anchor rather than a button because it is an address, and a
  colleague should be able to open it in a new tab.
-->
<Story name="Before elevation">
  {#snippet template(args)}<RootWorkspaceView {...args} />{/snippet}
</Story>

<!--
  Who may act in it. The Root console splits what a workspace's own panel keeps
  together: `users` and `invitations` are two views here and two tabs there.
-->
<Story name="Users">
  {#snippet template(args)}<RootWorkspaceView {...args} view="users" />{/snippet}
</Story>

<Story name="Invitations">
  {#snippet template(args)}<RootWorkspaceView {...args} view="invitations" />{/snippet}
</Story>

<!-- What changed, and what failed to arrive. -->
<Story name="History">
  {#snippet template(args)}<RootWorkspaceView {...args} view="history" />{/snippet}
</Story>
