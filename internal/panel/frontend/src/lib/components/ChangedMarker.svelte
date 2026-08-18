<script lang="ts">
  import AppTooltip from './AppTooltip.svelte';

  /**
   * The ringed dot that says a field has been edited and not yet saved.
   *
   * `ConfigEditor` wrote this four times - the tooltip, the span, the aria-label and
   * the inner dot, identical in each - once per kind of row it draws. Four copies of a
   * mark is four places for one of them to drift, and a mark that means "unsaved" in
   * three rows and something slightly different in the fourth is worse than no mark.
   *
   * **One marker per meaning.** The broken chain marks an override; this marks an
   * unsaved edit. They are different questions and must not converge on one shape.
   *
   * The word lives in the tooltip rather than beside the dot: a row already carries a
   * label, a value and a help tip, and a fourth piece of type in it reads as part of
   * the setting's name. `aria-label` carries the same word, so what a tooltip shows a
   * pointer a screen reader is told outright.
   */
  const { label = 'Unsaved' }: { label?: string } = $props();
</script>

<AppTooltip text={label}>
  {#snippet children(props)}
    <span {...props} class="changed-marker" aria-label={label}>
      <span class="changed-marker-dot"></span>
    </span>
  {/snippet}
</AppTooltip>

<style>
  .changed-marker {
    align-items: center;
    border: 1px solid color-mix(in srgb, var(--warning) 45%, transparent);
    border-radius: 999px;
    color: var(--warning);
    display: inline-flex;
    /* `flex: none`, because every one of these sits in a flex row beside a label that
       may be long enough to want the space. The mark is the fixed thing in the row. */
    flex: none;
    padding: 4px;
  }

  .changed-marker-dot {
    background: currentcolor;
    border-radius: var(--r-chip);
    display: block;
    height: 0.35rem;
    width: 0.35rem;
  }
</style>
