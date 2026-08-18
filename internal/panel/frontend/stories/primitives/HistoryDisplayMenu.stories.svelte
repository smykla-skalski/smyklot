<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import HistoryDisplayMenu from '#lib/components/HistoryDisplayMenu.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/HistoryDisplayMenu',
    component: HistoryDisplayMenu,
    argTypes: { value: { control: 'inline-radio', options: ['relative', 'absolute'] } },
    args: { value: 'relative', onSelect: fn() },
  });
</script>

<!--
  Whether a time reads as "4 minutes ago" or as a timestamp. It is a preference, so it
  syncs over the websocket and follows the reader to another tab.
-->
<Story name="Relative">
  {#snippet template(args)}
    <div class="frame"><HistoryDisplayMenu {...args} /></div>
  {/snippet}
</Story>

<Story name="Absolute" args={{ value: 'absolute' }}>
  {#snippet template(args)}
    <div class="frame"><HistoryDisplayMenu {...args} /></div>
  {/snippet}
</Story>

<style>
  .frame {
    padding-bottom: 12rem;
  }
</style>
