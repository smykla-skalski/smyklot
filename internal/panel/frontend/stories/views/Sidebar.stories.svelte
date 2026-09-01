<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import Sidebar, { type SidebarEntry } from '#lib/components/Sidebar.svelte';

  /* The approved mock's workspace map: every page one row, grouped under the
     headings that name them, with the Plan count speaking as a signal and
     Workspace settings standing apart at the foot. */
  const WORKSPACE_ENTRIES: SidebarEntry[] = [
    {
      id: 'repositories',
      label: 'Repositories',
      icon: 'book',
      href: '#/repositories',
      active: false,
    },
    { id: 'queue', label: 'Queue', icon: 'pending', href: '#/queue', active: false },
    { id: 'schedules', label: 'Schedules', icon: 'calendar', href: '#/schedules', active: false },
    { kind: 'group', id: 'group-sync', label: 'Sync' },
    { id: 'sync-overview', label: 'Sync status', icon: 'refresh', href: '#/sync', active: true },
    { id: 'sync-labels', label: 'Labels', icon: 'tag', href: '#/sync/labels', active: false },
    {
      id: 'sync-settings',
      label: 'Repository options',
      icon: 'sliders',
      href: '#/sync/settings',
      active: false,
    },
    {
      id: 'sync-rulesets',
      label: 'Rulesets',
      icon: 'branch',
      href: '#/sync/rulesets',
      active: false,
    },
    { id: 'sync-files', label: 'Shared files', icon: 'file', href: '#/sync/files', active: false },
    {
      id: 'sync-plan',
      label: 'Plan',
      icon: 'plan',
      href: '#/sync/plan',
      active: false,
      count: 14,
      signal: true,
    },
    { kind: 'group', id: 'group-access', label: 'Access' },
    { id: 'access-users', label: 'Users', icon: 'users', href: '#/users', active: false },
    {
      id: 'access-invitations',
      label: 'Invitations',
      icon: 'mail',
      href: '#/invitations',
      active: false,
    },
    { kind: 'group', id: 'group-activity', label: 'Activity' },
    { id: 'history-audit', label: 'Audit', icon: 'history', href: '#/history', active: false },
    {
      id: 'history-failures',
      label: 'Failures',
      icon: 'failure',
      href: '#/history/failures',
      active: false,
      count: 2,
    },
    {
      id: 'settings',
      label: 'Workspace settings',
      icon: 'gear',
      href: '#/settings',
      active: false,
      foot: true,
    },
  ];

  const ROOT_ENTRIES: SidebarEntry[] = [
    { id: 'overview', label: 'Overview', icon: 'gauge', href: '#/root', active: true },
    {
      id: 'workspaces',
      label: 'Workspaces',
      icon: 'book',
      href: '#/root/workspaces',
      active: false,
    },
    { id: 'queue', label: 'Queue', icon: 'pending', href: '#/root/queue', active: false },
    {
      id: 'schedules',
      label: 'Schedules',
      icon: 'calendar',
      href: '#/root/schedules',
      active: false,
    },
    { id: 'history-audit', label: 'Audit', icon: 'history', href: '#/root/history', active: false },
    {
      id: 'history-failures',
      label: 'Failures',
      icon: 'failure',
      href: '#/root/history/failures',
      active: false,
      count: 3,
      signal: true,
    },
    { kind: 'group', id: 'group-access', label: 'Access' },
    { id: 'access-users', label: 'Users', icon: 'users', href: '#/root/access', active: false },
    {
      id: 'access-invitations',
      label: 'Invitations',
      icon: 'mail',
      href: '#/root/access/invitations',
      active: false,
    },
    { kind: 'group', id: 'group-system', label: 'System' },
    {
      id: 'runtime-service',
      label: 'Service health',
      icon: 'server',
      href: '#/root/runtime/service',
      active: false,
    },
    {
      id: 'runtime-settings',
      label: 'Service settings',
      icon: 'gear',
      href: '#/root/runtime/settings',
      active: false,
    },
  ];

  const rowActive = (id: string): SidebarEntry[] =>
    WORKSPACE_ENTRIES.map((entry) =>
      entry.kind === 'group' ? entry : { ...entry, active: entry.id === id },
    );

  const DIRTY_WORKSPACE_ENTRIES: SidebarEntry[] = WORKSPACE_ENTRIES.map((entry) =>
    entry.kind !== 'group' && (entry.id === 'settings' || entry.id === 'sync-settings')
      ? { ...entry, dirty: true }
      : entry,
  );

  const { Story } = defineMeta({
    title: 'Views/Sidebar',
    component: Sidebar,
    argTypes: {
      collapsed: { control: 'boolean' },
    },
    args: {
      kicker: 'Workspace',
      title: 'Smykla Skalski',
      entries: WORKSPACE_ENTRIES,
      collapsed: false,
      onToggleCollapsed: fn(),
      onSelectRow: fn(),
    },
  });
</script>

<!--
  The active console's map: every page a row of its own, under the headings
  that group them. The selected row's ground is one thumb that travels.
-->
<Story name="Workspace">
  {#snippet template(args)}
    <div class="stage"><Sidebar {...args} /></div>
  {/snippet}
</Story>

<!-- The waiting plan open: the count keeps speaking on the selected row. -->
<Story name="Plan open" args={{ entries: rowActive('sync-plan') }}>
  {#snippet template(args)}
    <div class="stage"><Sidebar {...args} /></div>
  {/snippet}
</Story>

<!--
  Dirty is not the Plan's signal: each row marks its own unsaved configuration,
  wherever in the tree that page sits.
-->
<Story name="Unsaved trail" args={{ entries: DIRTY_WORKSPACE_ENTRIES }}>
  {#snippet template(args)}
    <div class="stage"><Sidebar {...args} /></div>
  {/snippet}
</Story>

<!-- The Root console's own map, same component, its palette from the shell. -->
<Story name="Root console" args={{ kicker: 'Console', title: 'Operations', entries: ROOT_ENTRIES }}>
  {#snippet template(args)}
    <div class="stage"><Sidebar {...args} /></div>
  {/snippet}
</Story>

<!--
  Folded to the icon strip: rows become centred glyphs with names as tooltips,
  a group's heading becomes the hairline that kept its boundary, and a signal
  survives as a dot. Needs a shell wider than 64rem, like the app's.
-->
<Story name="Collapsed strip" args={{ collapsed: true, entries: DIRTY_WORKSPACE_ENTRIES }}>
  {#snippet template(args)}
    <div class="stage app-shell sidebar-collapsed"><Sidebar {...args} /></div>
  {/snippet}
</Story>

<style>
  .stage {
    block-size: 40rem;
    contain: size layout;
    display: flex;
    overflow: visible;
  }

  .stage :global(.side) {
    height: 100%;
  }
</style>
