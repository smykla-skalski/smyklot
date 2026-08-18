<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import RootInstallationView from '#lib/components/RootInstallationView.svelte';
  import { fixtureApi } from '../support/api.js';
  import { INSTALLATIONS } from '../support/fixtures.js';

  /*
   * The Root console's reading of one installation. It takes its `api` as a prop, so
   * this could be a `stubApi` naming each method - but it draws the same four views
   * `InstallationView` does, and each reaches its own set. `fixtureApi` answers all of
   * them from the mock's data, which is the point of having one.
   */
  const installation = INSTALLATIONS[0]!;

  const base = {
    installation,
    view: 'repositories' as const,
    api: fixtureApi(),
    actorLogin: 'bart',
    historySection: 'audit' as const,
    onHistorySection: fn(),
    listHref: '#/root/installations',
    hrefFor: (account: string, view: string) => `#/root/installations/${account}/${view}`,
    onList: fn(),
    onNavigate: fn(),
  };

  const { Story } = defineMeta({
    title: 'Views/RootInstallationView',
    component: RootInstallationView,
    args: base,
  });
</script>

<!--
  What the Root console shows before it has elevated into an installation, which is
  what a Root sees first and most often: the identity, the tabs, the way back to the
  catalogue, and a body saying this view is not theirs to read yet. The button that
  changes that is the only thing on the page that acts.

  The back link is an anchor rather than a button because it is an address, and a
  colleague should be able to open it in a new tab.
-->
<Story name="Before elevation">
  {#snippet template(args)}<RootInstallationView {...args} />{/snippet}
</Story>

<!--
  Who may act in it. The Root console splits what an installation's own panel keeps
  together: `users` and `invitations` are two views here and two tabs there.
-->
<Story name="Users">
  {#snippet template(args)}<RootInstallationView {...args} view="users" />{/snippet}
</Story>

<Story name="Invitations">
  {#snippet template(args)}<RootInstallationView {...args} view="invitations" />{/snippet}
</Story>

<!-- What changed, and what failed to arrive. -->
<Story name="History">
  {#snippet template(args)}<RootInstallationView {...args} view="history" />{/snippet}
</Story>
