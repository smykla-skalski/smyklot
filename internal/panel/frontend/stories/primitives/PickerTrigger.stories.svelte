<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import PickerTrigger from '#lib/components/PickerTrigger.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/PickerTrigger',
    component: PickerTrigger,
    args: { disabled: false },
    argTypes: { disabled: { control: 'boolean' } },
  });
</script>

<Story name="Playground">
  {#snippet template({ children, ...args })}
    <PickerTrigger {...args} aria-label="Repository protection">Checks</PickerTrigger>
  {/snippet}
</Story>

<Story name="Open">
  {#snippet template()}
    <PickerTrigger data-state="open" aria-expanded="true">Checks</PickerTrigger>
  {/snippet}
</Story>
