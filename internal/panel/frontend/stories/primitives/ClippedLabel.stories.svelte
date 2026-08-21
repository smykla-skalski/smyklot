<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ClippedLabel from '#lib/components/ClippedLabel.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ClippedLabel',
    component: ClippedLabel,
  });
</script>

<!--
  A one-line label that knows when it has been cut: while the text fits it
  is a plain span, and the moment the ellipsis appears, hovering answers
  with the whole text. The wrapper owns the clipping styles.
-->
<Story name="Clipped">
  {#snippet template()}
    <div style:width="12rem" style:overflow="hidden">
      <ClippedLabel
        class="story-clip"
        text="Commit title, or the pull request's when squashing many"
      />
    </div>
  {/snippet}
</Story>

<!-- Fits, so hovering says nothing - a tip repeating visible text is noise. -->
<Story name="Fits">
  {#snippet template()}
    <div style:width="20rem">
      <ClippedLabel class="story-clip" text="Pull request title" />
    </div>
  {/snippet}
</Story>

<style>
  /* The wrapper owns the clipping, exactly as a menu row would. */
  :global(.story-clip) {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
