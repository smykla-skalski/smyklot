<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import TokenTable from '../support/TokenTable.svelte';

  const SPACE = Array.from({ length: 9 }, (_, index) => ({
    name: `space-${index}`,
    note: index === 0 ? 'Nothing' : '',
  }));

  const SHAPE = [
    { name: 'radius-control', note: 'A button, an input, a select' },
    { name: 'radius-chip', note: 'A chip and a status pill' },
    { name: 'radius-surface', note: 'A card' },
    { name: 'radius-dialog', note: 'A dialog' },
    { name: 'radius-popover', note: 'A popover and a tooltip' },
  ];

  const HEIGHTS = [
    { name: 'control-height', note: 'The tall control' },
    { name: 'control-height-compact', note: '34px - what the content panes use' },
    { name: 'content-max', note: 'How wide a page is allowed to run' },
  ];

  const ELEVATION = [
    { name: 'shadow-plate', note: 'A single 1px drop at 10% - not a glow' },
    { name: 'shadow-popover', note: 'A layer above the page' },
    { name: 'shadow-dialog', note: 'The dialog, above everything' },
  ];

  const { Story } = defineMeta({ title: 'Foundations/Spacing, shape and elevation' });
</script>

<Story name="Spacing">
  {#snippet template()}
    <TokenTable tokens={SPACE} swatch={false} />
    <div class="ramp">
      {#each SPACE.slice(1) as step (step.name)}
        <span class="bar" style:width={`var(--${step.name})`}></span>
      {/each}
    </div>
  {/snippet}
</Story>

<Story name="Shape">
  {#snippet template()}
    <TokenTable tokens={SHAPE} swatch={false} />
    <div class="shapes">
      {#each SHAPE as shape (shape.name)}
        <span class="box" style:border-radius={`var(--${shape.name})`}></span>
      {/each}
    </div>
  {/snippet}
</Story>

<Story name="Heights">
  {#snippet template()}
    <TokenTable tokens={HEIGHTS} swatch={false} />
  {/snippet}
</Story>

<!--
  `--shadow-plate` is a single 1px drop at 10%: not a glow, and not enough to read as
  a raised object. It is what stops a card whose border is 1.4:1 against the page from
  having only that border to say where it ends.
-->
<Story name="Elevation">
  {#snippet template()}
    <TokenTable tokens={ELEVATION} swatch={false} />
    <div class="shapes">
      {#each ELEVATION as level (level.name)}
        <span class="box lifted" style:box-shadow={`var(--${level.name})`}></span>
      {/each}
    </div>
  {/snippet}
</Story>

<style>
  .ramp,
  .shapes {
    align-items: flex-end;
    display: flex;
    gap: var(--space-4);
    margin-top: var(--space-5);
  }
  .bar {
    background: var(--accent);
    display: block;
    height: 1.5rem;
  }
  .box {
    background: var(--surface-inset);
    border: 1px solid var(--control-border);
    display: block;
    height: 3rem;
    width: 3rem;
  }
  .lifted {
    background: var(--surface-base);
  }
</style>
