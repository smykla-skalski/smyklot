<script module lang="ts">
  /**
   * `quiet` states a consequence the reader should take in before confirming;
   * `warning` is for the one that is genuinely hazardous, and carries the tint.
   */
  export type CalloutTone = 'quiet' | 'warning';
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  const {
    tone = 'quiet',
    icon,
    act,
    class: extra = '',
    children,
    ...rest
  }: {
    tone?: CalloutTone;
    /** The mark beside the words, when the words alone do not carry the weight. */
    icon?: Snippet;
    /**
     * The one thing to do about it, riding the end of the line.
     *
     * At most one, and only where the consequence has a next step: a second act here is
     * a decision, and a decision belongs in a dialog where it can be read before it is
     * made. It sits at the end because that is where this family has always put it -
     * the words are what the reader came for, and the act is what they leave by.
     */
    act?: Snippet;
    /** The caller's own layout - a grid column to span, say. Never the box's paint. */
    class?: string;
    children: Snippet;
  } & HTMLAttributes<HTMLDivElement> = $props();
</script>

<!--
@component
A short consequence, set off from the words around it - beside the work rather than
instead of it. That is the line between this and `TableEmptyState`: a callout says
something about what the reader is looking at, and an empty state stands where the
work would have been.

Two tones and no more. `quiet` states a consequence the reader should take in before
confirming; `warning` is for the one that is genuinely hazardous and carries the tint.
A third tone would be a third weight for a reader to rank, and the point of two is that
the tinted one means something.

Eight of these were written by hand under three names - `.confirmation-note`,
`.root-warning`, `.elevation-note` - and three of the four declarations were already
the same box. The fourth differed only in a background, by less than an eye can
resolve.

Rest props pass through, so a caller can still hand the box an id, a role or a
`tabindex` of its own.
-->

<!--
  The children are not wrapped. Three call sites pass a `<span>`, one passes a `<p>`
  and one a `<div>` holding a heading and a paragraph - all of them direct flex
  children beside the mark. A wrapper here would make the caller's element a
  grandchild and take the gap away from it.
-->
<div class="callout callout-{tone} {extra}" {...rest}>
  {#if icon !== undefined}
    {@render icon()}
  {/if}
  {@render children()}
  {#if act !== undefined}
    <span class="callout-act">{@render act()}</span>
  {/if}
</div>

<style>
  /* Wrapped, unlike the children: the act is one element and needs a box that can be
     pushed to the end without the caller knowing how this row is laid out. */
  .callout-act {
    display: flex;
    flex: none;
    margin-inline-start: auto;
  }

  .callout {
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  /* Centred, because a one-line consequence beside a mark is a row. */
  .callout-quiet {
    align-items: center;
    background: var(--interactive-hover);
    border: 1px solid var(--border-subtle);
  }

  /* Top-aligned instead: a warning runs to several lines, and a mark centred
     against a paragraph floats in the middle of it rather than marking its start. */
  .callout-warning {
    align-items: flex-start;
    background: var(--warning-tint);
    border: 1px solid color-mix(in srgb, var(--warning) 30%, var(--warning-tint));
    font-size: var(--font-size-compact);
    gap: var(--space-2);
  }
</style>
