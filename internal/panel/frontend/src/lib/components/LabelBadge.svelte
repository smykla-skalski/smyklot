<script lang="ts">
  import type { SyncLabelDetail } from '../types';

  const { label, size = 'default' }: { label: SyncLabelDetail; size?: 'default' | 'compact' } =
    $props();

  /* GitHub stores six hex digits with no `#`, and has since the API's first
     version. Anything else is somebody's label reaching us through a path that
     did not come from GitHub, and painting it would let a stored value decide a
     CSS declaration - so an unreadable colour draws the muted ring instead. */
  const swatch = $derived(/^[0-9a-fA-F]{6}$/u.test(label.color) ? `#${label.color}` : null);
</script>

<!--
@component
A label as GitHub means it: the colour, then the name.

REACH FOR THIS over writing the hex beside the name. A label's colour is how it
is recognised in an issue list, and `#0e8a16` is the same fact spelled in a way
nobody reads a label by - the plan page printed it, and the reader had to hold a
hex value in their head to know which label was meant.

The dot is a DOT, never the whole chip: a filled badge in the label's own colour
is what GitHub draws, and it cannot be done here without either failing contrast
on a pale label or picking the ink from the colour - which is a computation on a
value the workspace controls. The ring around the dot keeps a white or very pale
label visible on the card.

`size="compact"` is the plan row, where the badge sits in a line of 12px mono
and must not set the line's height.

Pass the description separately if a caller wants it; this draws the label.
-->

<span class="label-badge" class:is-compact={size === 'compact'}>
  {#if swatch !== null}
    <span class="label-dot" style:--swatch={swatch}></span>
  {:else}
    <span class="label-dot is-unknown"></span>
  {/if}
  <span class="label-badge-name">{label.name}</span>
</span>

<style>
  .label-badge {
    align-items: center;
    display: inline-flex;
    /* Never `auto`: a flex or grid parent blockifies an inline-flex item and
       stretches it to the column, which puts a two-word badge at the full width
       of the row it sits in. */
    inline-size: fit-content;
    gap: var(--space-2);
    min-inline-size: 0;
  }

  .label-dot {
    background: var(--swatch);
    border-radius: 50%;
    /* 12px whole - 1cap resolved to 11.17 and the disc edge blurred. Matches
       the swatch the labels editor draws, which is the same object. */
    block-size: 12px;
    /* A pale label needs an edge or it is a hole in the card. Mixed from the
       ink rather than a border colour, so it reads as the disc's own edge at
       every skin. */
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text-primary) 14%, transparent);
    display: inline-block;
    flex: none;
    inline-size: 12px;
  }

  .label-dot.is-unknown {
    background: var(--surface-inset);
  }

  .is-compact .label-dot {
    block-size: 9px;
    inline-size: 9px;
  }

  .label-badge-name {
    min-inline-size: 0;
    overflow-wrap: anywhere;
    text-box: trim-both cap alphabetic;
  }

  .is-compact .label-badge-name {
    font-family: var(--mono);
  }
</style>
