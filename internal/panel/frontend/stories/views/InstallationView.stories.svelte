<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import InstallationView from '#lib/components/InstallationView.svelte';
  import { NOW } from '../support/fixtures.js';

  /*
   * One prop, and everything else through the session.
   *
   * `view` is passed rather than read from the address on purpose - there is no single
   * address that reaches all of these, because a view hosting a dialog is routed with
   * the segments after it and one hosting none is routed without them.
   *
   * Its data comes from `session.api`, which `PanelShell` backs with the mock's own
   * fixtures. That is why there is nothing to seed and nothing to stub here: twenty
   * methods are reached from this one component, and a story has no say in which.
   */
  const { Story } = defineMeta({
    title: 'Views/InstallationView',
    component: InstallationView,
    args: { view: 'repositories', clock: () => NOW },
  });
</script>

<!-- The repositories the bot reaches, and whether it acts on each. -->
<Story name="Repositories">
  {#snippet template(args)}<InstallationView {...args} />{/snippet}
</Story>

<!--
  Who may act here. `users`, not `access`: the pane is reached by two of the six
  `PANEL_VIEWS` and neither is spelled that way, so the story used to match no branch
  and draw a 48px empty frame - which `svelte-check` and `eslint` both pass.
-->
<Story name="Users">
  {#snippet template(args)}<InstallationView {...args} view="users" />{/snippet}
</Story>

<!-- The other half of the same pane: invitations still outstanding. -->
<Story name="Invitations">
  {#snippet template(args)}<InstallationView {...args} view="invitations" />{/snippet}
</Story>

<!-- What changed and what failed to arrive. -->
<Story name="History">
  {#snippet template(args)}<InstallationView {...args} view="history" />{/snippet}
</Story>

<!-- The account defaults every repository inherits until it overrides one. -->
<Story name="Workspace defaults">
  {#snippet template(args)}<InstallationView {...args} view="defaults" />{/snippet}
</Story>

<!--
  What every repository here should share, and what would change to make it true. The
  fifth view this component hosts, and the one it had no story for - so nothing showed
  the sync page standing in the shell it is actually read in.
-->
<Story name="Sync">
  {#snippet template(args)}<InstallationView {...args} view="sync" />{/snippet}
</Story>
