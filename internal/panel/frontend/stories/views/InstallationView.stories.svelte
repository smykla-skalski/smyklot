<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import InstallationView from '#lib/components/InstallationView.svelte';

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
    args: { view: 'repositories' },
  });
</script>

<!-- The repositories the bot reaches, and whether it acts on each. -->
<Story name="Repositories">
  {#snippet template(args)}<InstallationView {...args} />{/snippet}
</Story>

<!-- Who may act here, and the invitations still outstanding. -->
<Story name="Access">
  {#snippet template(args)}<InstallationView {...args} view="access" />{/snippet}
</Story>

<!-- What changed and what failed to arrive. -->
<Story name="History">
  {#snippet template(args)}<InstallationView {...args} view="history" />{/snippet}
</Story>

<!-- The account defaults every repository inherits until it overrides one. -->
<Story name="Settings">
  {#snippet template(args)}<InstallationView {...args} view="settings" />{/snippet}
</Story>
