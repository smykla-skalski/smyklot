<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Switch from '#lib/components/Switch.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Switch',
    component: Switch,
    argTypes: {
      checked: { control: 'boolean' },
      disabled: { control: 'boolean' },
      label: { control: 'text' },
    },
    args: { checked: true, disabled: false, label: 'Syncing' },
  });
</script>

<script lang="ts">
  let flipped = $state(true);
</script>

<Story name="Playground">
  {#snippet template(args)}
    <Switch {...args} onChange={() => {}} />
  {/snippet}
</Story>

<!--
  The contract: flipping it is the act. Where a value waits for a Save it is a
  form control, where the options need sentences it is a radio card, and where
  it narrows what is on screen it is a segmented control. The old sync page
  said all four with segmented pairs.
-->
<Story name="States">
  {#snippet template()}
    <div class="row">
      <Switch checked label="Syncing" onChange={() => {}} />
      <Switch checked={false} label="Removal" onChange={() => {}} />
      <Switch checked disabled label="Read only" onChange={() => {}} />
      <Switch checked={false} disabled label="Off, read only" onChange={() => {}} />
    </div>
  {/snippet}
</Story>

<Story name="Live">
  {#snippet template()}
    <Switch
      checked={flipped}
      label={flipped ? 'Syncing' : 'Stood down'}
      onChange={(next) => (flipped = next)}
    />
  {/snippet}
</Story>

<style>
  .row {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-5);
  }
</style>
