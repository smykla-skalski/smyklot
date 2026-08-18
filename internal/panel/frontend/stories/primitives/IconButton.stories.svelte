<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { fn } from 'storybook/test';

  import IconButton from '#lib/components/IconButton.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/IconButton',
    component: IconButton,
    argTypes: {
      disabled: { control: 'boolean' },
      busy: { control: 'boolean' },
    },
    args: { icon: 'refresh', label: 'Refresh', disabled: false, busy: false, onclick: fn() },
  });
</script>

<!-- The label is the accessible name and the tooltip both, so it is never optional. -->
<Story name="Playground" />

<Story name="States">
  {#snippet template()}
    <div class="row">
      <IconButton icon="refresh" label="Refresh" onclick={() => {}} />
      <IconButton icon="refresh" label="Refreshing" busy onclick={() => {}} />
      <IconButton icon="trash" label="Remove" disabled onclick={() => {}} />
    </div>
  {/snippet}
</Story>

<style>
  .row {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }
</style>
