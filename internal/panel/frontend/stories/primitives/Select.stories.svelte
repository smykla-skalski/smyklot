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

<Story name="Playground">
  {#snippet template(args)}
    <Select disabled={args.disabled} options={UNITS} value="minutes" aria-label="Unit" />
  {/snippet}
</Story>

<Story name="Disabled" args={{ disabled: true }}>
  {#snippet template(args)}
    <Select disabled={args.disabled} options={UNITS} value="minutes" aria-label="Unit" />
  {/snippet}
</Story>

<Story name="Command values">
  {#snippet template()}
    <Select
      class="mono"
      value="approve"
      aria-label="Command"
      options={['approve', 'merge', 'squash', 'cleanup'].map((value) => ({ value, label: value }))}
    />
  {/snippet}
</Story>

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
