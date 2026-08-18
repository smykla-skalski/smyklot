<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import ViewTabs from '#lib/components/ViewTabs.svelte';

  const { Story } = defineMeta({
    title: 'Views/ViewTabs',
    component: ViewTabs,
    argTypes: {
      collapsed: { control: 'boolean' },
      rootMode: { control: 'boolean' },
      showUsers: { control: 'boolean' },
      showViews: { control: 'boolean' },
      inboxActive: { control: 'boolean' },
    },
    args: {
      value: 'repositories',
      hrefFor: (view: string) => `#/${view}`,
      onSelect: fn(),
      showUsers: true,
      showViews: true,
      collapsed: false,
      rootMode: false,
      rootEnabled: true,
      rootValue: 'overview',
      rootHrefFor: (section: string) => `#/root/${section}`,
      onSelectRoot: fn(),
      rootEntryHref: '#/root',
      onEnterRoot: fn(),
      returnHref: '#/',
      onReturnToPanel: fn(),
      inboxHref: '#/inbox',
      inboxActive: false,
      onSelectInbox: fn(),
      unreadCount: 3,
    },
  });
</script>

<!--
  The sidebar rail. Collapsed it keeps the icons and drops the labels, so the column
  narrows without the rows moving - and the tooltip that replaces each label is the
  panel's one tooltip rather than a title attribute.
-->
<Story name="Panel">
  {#snippet template(args)}
    <div class="rail"><ViewTabs {...args} /></div>
  {/snippet}
</Story>

<Story name="Collapsed" args={{ collapsed: true }}>
  {#snippet template(args)}
    <div class="rail narrow"><ViewTabs {...args} /></div>
  {/snippet}
</Story>

<!-- The Root console's own sections, which replace the workspace's while it is on. -->
<Story name="Root console" args={{ rootMode: true }}>
  {#snippet template(args)}
    <div class="rail"><ViewTabs {...args} /></div>
  {/snippet}
</Story>

<!-- Nothing unread, so the badge is not drawn at all rather than drawn as a zero. -->
<Story name="Inbox clear" args={{ unreadCount: 0, inboxActive: true }}>
  {#snippet template(args)}
    <div class="rail"><ViewTabs {...args} /></div>
  {/snippet}
</Story>

<!-- A viewer who cannot manage users never sees that row. -->
<Story name="Without users" args={{ showUsers: false }}>
  {#snippet template(args)}
    <div class="rail"><ViewTabs {...args} /></div>
  {/snippet}
</Story>

<style>
  .rail {
    background: var(--sidebar-bg);
    border-radius: var(--radius-surface);
    min-height: 24rem;
    padding: var(--space-3);
    width: var(--sidebar-width, 15rem);
  }
  .narrow {
    width: var(--sidebar-width-collapsed, 4rem);
  }
</style>
