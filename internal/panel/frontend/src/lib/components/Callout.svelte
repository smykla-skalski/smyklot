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

  /**
   * A short consequence, set off from the words around it.
   *
   * Eight of these were written by hand under three names - `.confirmation-note`,
   * `.root-warning` and `.elevation-note` - and three of the four declarations were
   * the same box. The fourth reached for `--well`, `--rule` and `--r-well`, which are
   * aliases of `--surface-inset`, `--border-subtle` and `--radius-control`; only the
   * background genuinely differed, `#eeebf4` against `#f0ecf6`, which is under a
   * just-noticeable difference. So they are one box now, and the one that moved
   * moved by less than an eye can resolve.
   *
   * Rest props pass through so a caller can still hand the box an id, a role or a
   * `tabindex` of its own.
   */
  const {
    tone = 'quiet',
    icon,
    class: extra = '',
    children,
    ...rest
  }: {
    tone?: CalloutTone;
    /** The mark beside the words, when the words alone do not carry the weight. */
    icon?: Snippet;
    /** The caller's own layout - a grid column to span, say. Never the box's paint. */
    class?: string;
    children: Snippet;
  } & HTMLAttributes<HTMLDivElement> = $props();
</script>

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
</div>

<style>
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
