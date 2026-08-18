<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Select from '#lib/components/Select.svelte';

  const UNITS = [
    { value: 'seconds', label: 'Seconds' },
    { value: 'minutes', label: 'Minutes' },
    { value: 'hours', label: 'Hours' },
  ];

  const { Story } = defineMeta({
    title: 'Primitives/Select',
    component: Select,
    argTypes: { disabled: { control: 'boolean' } },
    args: { disabled: false },
  });
</script>

<!--
  A native select cannot be given an indicator of its own, so the panel wraps it and
  draws the chevron alongside - which is why the wrapper, and not the select, is the
  layout box. Nine call sites wrote that wrapper and that chevron, at that exact size
  and stroke, by hand.
-->
<Story name="Playground">
  {#snippet template({ children, ...args })}
    <Select {...args} options={UNITS} value="minutes" aria-label="Unit" />
  {/snippet}
</Story>

<Story name="Disabled" args={{ disabled: true }}>
  {#snippet template({ children, ...args })}
    <Select {...args} options={UNITS} value="minutes" aria-label="Unit" />
  {/snippet}
</Story>

<!--
  Options come as data or as markup. Most call sites have a fixed list; three build
  their `<option>`s in a loop, and forcing either shape into the other would be worse
  than accepting both.
-->
<Story name="Options as markup">
  {#snippet template()}
    <Select class="mono" value="approve" aria-label="Command">
      {#each ['approve', 'merge', 'squash', 'cleanup'] as command (command)}
        <option value={command}>{command}</option>
      {/each}
    </Select>
  {/snippet}
</Story>

<!-- The page-size control is the one select that sets its own width. -->
<Story name="Narrow">
  {#snippet template()}
    <div class="narrow">
      <Select
        value={20}
        aria-label="Rows per page"
        options={[
          { value: 20, label: '20' },
          { value: 50, label: '50' },
          { value: 100, label: '100' },
        ]}
      />
    </div>
  {/snippet}
</Story>

<style>
  .narrow :global(.select-wrap) {
    width: 4rem;
  }
</style>
