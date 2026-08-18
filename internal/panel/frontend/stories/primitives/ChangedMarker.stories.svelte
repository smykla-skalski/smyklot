<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ChangedMarker from '#lib/components/ChangedMarker.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ChangedMarker',
    component: ChangedMarker,
  });
</script>

<!--
  A field edited and not yet saved. `ConfigEditor` drew this four times, once per kind
  of row - four places for one of them to drift, and a mark meaning "unsaved" in three
  rows and something slightly different in the fourth is worse than no mark at all.

  One marker per meaning: the broken chain marks an override, this marks an unsaved
  edit, and they must not converge on one shape.
-->
<Story name="Unsaved">
  {#snippet template()}<ChangedMarker />{/snippet}
</Story>

<!--
  The word lives in the tooltip rather than beside the dot - a config row already
  carries a label, a value and a help tip, and a fourth piece of type reads as part of
  the setting's name. `aria-label` carries the same word, so what a pointer is shown a
  screen reader is told.
-->
<Story name="Named">
  {#snippet template()}<ChangedMarker label="Edited, not saved" />{/snippet}
</Story>
