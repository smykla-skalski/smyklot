<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import Sidebar, { type SidebarPage } from '#lib/components/Sidebar.svelte';

  /* The approved mock's workspace map: Sync is the open page, its sections
     nested under it with the Plan count speaking as a signal. */
  const WORKSPACE_PAGES: SidebarPage[] = [
    { id: 'settings', label: 'Settings', icon: 'sliders', href: '#/settings', active: false },
    {
      id: 'repositories',
      label: 'Repositories',
      icon: 'repositories',
      href: '#/repositories',
      active: false,
    },
    {
      id: 'sync',
      label: 'Sync',
      icon: 'refresh',
      href: '#/sync',
      active: true,
      kids: [
        { id: 'overview', label: 'Overview', href: '#/sync', active: true },
        { id: 'labels', label: 'Labels', href: '#/sync/labels', active: false },
        { id: 'settings', label: 'Settings', href: '#/sync/settings', active: false },
        { id: 'rulesets', label: 'Rulesets', href: '#/sync/rulesets', active: false },
        { id: 'files', label: 'Files', href: '#/sync/files', active: false },
        { id: 'plan', label: 'Plan', href: '#/sync/plan', active: false, count: 14, signal: true },
      ],
    },
    { id: 'access', label: 'Access', icon: 'users', href: '#/users', active: false },
    { id: 'history', label: 'History', icon: 'history', href: '#/history', active: false },
  ];

  const ROOT_PAGES: SidebarPage[] = [
    { id: 'overview', label: 'Overview', icon: 'system', href: '#/root', active: true },
    {
      id: 'installations',
      label: 'Installations',
      icon: 'repositories',
      href: '#/root/installations',
      active: false,
    },
    { id: 'queue', label: 'Queue', icon: 'pending', href: '#/root/queue', active: false },
    { id: 'access', label: 'Access', icon: 'users', href: '#/root/access', active: false },
    { id: 'history', label: 'History', icon: 'history', href: '#/root/history', active: false },
    { id: 'settings', label: 'Settings', icon: 'sliders', href: '#/root/settings', active: false },
  ];

  const kidActive = (kid: string): SidebarPage[] =>
    WORKSPACE_PAGES.map((page) =>
      page.id === 'sync'
        ? {
            ...page,
            kids: page.kids!.map((entry) => ({ ...entry, active: entry.id === kid })),
          }
        : page,
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
      pages: WORKSPACE_PAGES,
      collapsed: false,
      onToggleCollapsed: fn(),
      onSelectPage: fn(),
      onSelectKid: fn(),
    },
  });
</script>

<!--
  The active console's map: its pages, with the open page's sections nested
  under it and counts on the rows. The selected row's ground is one thumb
  that travels - between pages, and between a page row and its kids.
-->
<Story name="Workspace">
  {#snippet template(args)}
    <div class="stage"><Sidebar {...args} /></div>
  {/snippet}
</Story>

<!-- The waiting plan open: the thumb morphs to the kid's 28px, 6px-corner shape. -->
<Story name="Plan open" args={{ pages: kidActive('plan') }}>
  {#snippet template(args)}
    <div class="stage"><Sidebar {...args} /></div>
  {/snippet}
</Story>

<!-- The Root console's own map, same component, its palette from the shell. -->
<Story
  name="Root console"
  args={{ kicker: 'Root console', title: 'Operations', pages: ROOT_PAGES }}
>
  {#snippet template(args)}
    <div class="stage"><Sidebar {...args} /></div>
  {/snippet}
</Story>

<!--
  Folded to the 4.5rem strip: rows become centred glyphs with names as
  tooltips, sections open as a flyout, the Plan count survives as a dot on
  its page's glyph. Needs a shell wider than 64rem, like the app's.
-->
<Story name="Collapsed strip" args={{ collapsed: true }}>
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
