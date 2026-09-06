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
    actions,
    class: extra = '',
    children,
    ...rest
  }: {
    tone?: CalloutTone;
    /** The mark beside the words, when the words alone do not carry the weight. */
    icon?: Snippet;
    /** Adjacent controls, centered beside the message without changing its text rhythm. */
    actions?: Snippet;
    /** The caller's own layout - a grid column to span, say. Never the box's paint. */
    class?: string;
    children: Snippet;
  } & HTMLAttributes<HTMLDivElement> = $props();
</script>

<!--
@component
A short consequence, set off from the words around it - beside the work rather than
instead of it. That is the line between this and `EmptyState`: a callout says
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

Wrap message text in a paragraph or span. A heading and body use `.callout-copy`;
the shared stack owns their spacing. Pass adjacent controls through `actions`, so
the message stays vertically balanced independently of the icon and button size.
-->

<div class="callout callout-{tone} {extra}" {...rest}>
  <div class="callout-message">
    {#if icon !== undefined}
      <span class="callout-icon">{@render icon()}</span>
    {/if}
    <div class="callout-content">{@render children()}</div>
  </div>
  {#if actions !== undefined}
    <div class="callout-actions">{@render actions()}</div>
  {/if}
</div>

<style>
  .callout {
    align-items: center;
    border-radius: var(--radius-control);
    color: var(--text-secondary);
    container: callout / inline-size;
    display: flex;
    gap: var(--space-3);
    line-height: var(--row-copy-leading);
    padding: var(--space-3);
  }

  .callout-message {
    align-items: flex-start;
    display: flex;
    flex: 1;
    gap: inherit;
    min-inline-size: 0;
  }

  /* The icon overflows a cap-height track centered on the first text line. Its
     larger viewport never adds a line strut or changes the message's height. */
  .callout-icon {
    align-items: center;
    block-size: 1cap;
    display: flex;
    flex: none;
  }

  .callout-content {
    display: grid;
    flex: 1;
    gap: var(--row-copy-gap);
    min-inline-size: 0;
  }

  .callout-content > :global(*) {
    margin: 0;
    text-box: trim-both cap alphabetic;
  }

  .callout-content > :global(.callout-copy) {
    display: grid;
    gap: var(--row-copy-gap);
    min-inline-size: 0;
  }

  .callout-content > :global(.callout-copy) > :global(*) {
    margin: 0;
    text-box: trim-both cap alphabetic;
  }

  .callout-content > :global(.callout-copy) > :global(strong) {
    color: var(--text-primary);
  }

  .callout-actions {
    align-items: center;
    display: flex;
    flex: none;
    gap: var(--space-2);
  }

  .callout-quiet {
    background: var(--interactive-hover);
    border: 1px solid var(--border-subtle);
  }

  .callout-warning {
    background: var(--warning-tint);
    border: 1px solid color-mix(in srgb, var(--warning) 30%, var(--warning-tint));
    font-size: var(--font-size-compact);
    gap: var(--space-2);
  }

  @container callout (max-width: 24rem) {
    .callout-actions {
      align-items: stretch;
      flex-direction: column;
    }
  }
</style>
