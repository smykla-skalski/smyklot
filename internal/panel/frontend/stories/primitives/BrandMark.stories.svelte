<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import BrandMark from '#lib/components/BrandMark.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/BrandMark',
    component: BrandMark,
    argTypes: {
      heading: { control: 'boolean' },
      stacked: { control: 'boolean' },
      interior: { control: 'inline-radio', options: ['solid', 'clear'] },
      size: { control: { type: 'range', min: 16, max: 96, step: 4 } },
    },
    args: { heading: false, stacked: false, interior: 'solid', size: 36 },
  });
</script>

<Story name="Playground" />

<Story name="Interiors">
  {#snippet template()}
    <div class="row">
      <BrandMark interior="solid" size={48} />
      <BrandMark interior="clear" size={48} />
    </div>
  {/snippet}
</Story>

<Story name="Sizes">
  {#snippet template()}
    <div class="row">
      {#each [20, 28, 36, 48, 72] as size (size)}
        <BrandMark {size} />
      {/each}
    </div>
  {/snippet}
</Story>

<style>
  .row {
    align-items: center;
    display: flex;
    gap: var(--space-4);
  }
</style>
