<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ApplyBar from '#lib/components/ApplyBar.svelte';
  import Button from '#lib/components/Button.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ApplyBar',
    component: ApplyBar,
  });
</script>

<!--
  The sticky decision bar. At rest it is a plain card; glued to the viewport
  it turns to glass, rows dissolve into their own surface before they pass
  under its edge, and a halo radiates up. Scroll the story to watch the
  melt - the page above carries `timeline-scope: --bar-slot`.
-->
<Story name="Seated and glued">
  {#snippet template()}
    <div style="timeline-scope: --bar-slot; max-width: 60rem;">
      {#each Array.from({ length: 30 }, (_, at) => at) as row (row)}
        <p style="padding: 0.5rem 0; border-bottom: 1px solid var(--border-subtle);">
          Row {row + 1} slides under the glued bar
        </p>
      {/each}
      <ApplyBar>
        <span style="flex: 1; color: var(--text-secondary); font-size: var(--font-size-meta);"
          >Nothing reaches GitHub until you apply</span
        >
        <Button tone="quiet">Discard</Button>
        <Button tone="signal">Apply to 3 repositories</Button>
      </ApplyBar>
      {#each Array.from({ length: 6 }, (_, at) => at) as row (row)}
        <p style="padding: 0.5rem 0;">After the bar, row {row + 1}</p>
      {/each}
    </div>
  {/snippet}
</Story>
