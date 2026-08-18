<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import ActionMenu, { type ActionMenuItem } from '#lib/components/ActionMenu.svelte';

  const ITEMS: ActionMenuItem[] = [
    { id: 'promote', icon: 'admin', label: 'Make Root', description: 'Every Root permission' },
    { id: 'restore', icon: 'refresh', label: 'Restore access' },
    { id: 'ban', icon: 'ban', label: 'Ban', description: 'Blocks every workspace', tone: 'danger' },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/ActionMenu',
    component: ActionMenu,
    args: { label: 'Actions for @ada', items: ITEMS, onSelect: fn(), onOpenChange: fn() },
  });
</script>

<!--
  The kebab at the end of a row. Its slot is reserved whether or not a row has one,
  so every menu in a column lands at the same x - the gap on the right is not a gap.
-->
<Story name="Default">
  {#snippet template(args)}
    <div class="frame"><ActionMenu {...args} /></div>
  {/snippet}
</Story>

<!-- A destructive item takes the danger tone; the rest stay plain. -->
<Story
  name="With a disabled item"
  args={{ items: [...ITEMS, { id: 'remove', icon: 'trash', label: 'Remove', disabled: true }] }}
>
  {#snippet template(args)}
    <div class="frame"><ActionMenu {...args} /></div>
  {/snippet}
</Story>

<style>
  .frame {
    display: flex;
    justify-content: flex-end;
    padding-bottom: 14rem;
    width: 12rem;
  }
</style>
