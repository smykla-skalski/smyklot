<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import Sidebar, { type SidebarPage } from '#lib/components/Sidebar.svelte';

  /* The approved mock's workspace map: Sync is the open page, its sections
     nested under it with the Plan count speaking as a signal. */
  const WORKSPACE_PAGES: SidebarPage[] = [
    { id: 'defaults', label: 'Defaults', icon: 'sliders', href: '#/defaults', active: false },
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
    { id: 'runtime', label: 'Runtime', icon: 'sliders', href: '#/root/runtime', active: false },
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

  const DIRTY_WORKSPACE_PAGES: SidebarPage[] = WORKSPACE_PAGES.map((page) => {
    if (page.id === 'defaults') return { ...page, dirty: true };
    if (page.id !== 'sync') return page;
    return {
      ...page,
      kids: page.kids?.map((kid) => (kid.id === 'settings' ? { ...kid, dirty: true } : kid)),
    };
  });

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

<!--
  Dirty is not the Plan's signal: Defaults marks its own row, while Sync keeps
  the mark on the precise Settings child because that group is visible.
-->
<Story name="Unsaved trail" args={{ pages: DIRTY_WORKSPACE_PAGES }}>
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
  tooltips, sections open as a flyout, and hidden child state bubbles to its
  page glyph. Needs a shell wider than 64rem, like the app's.
-->
<Story name="Collapsed strip" args={{ collapsed: true, pages: DIRTY_WORKSPACE_PAGES }}>
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
