<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * Tones name what a state means rather than which state it is, so the pairing
   * states arriving with the inventory view map onto these without new colours.
   */
  export type ChipTone = 'neutral' | 'clear' | 'signal' | 'stop';

  const {
    tone = 'neutral',
    dot = false,
    small = false,
    children,
  }: {
    tone?: ChipTone;
    /** Set for a state that is live right now, rather than a fixed attribute. */
    dot?: boolean;
    /** For a chip that sits inside a line of text rather than beside it. */
    small?: boolean;
    children: Snippet;
  } = $props();
</script>

<span class="chip chip-{tone}" class:chip-small={small}>
  {#if dot}
    <!-- Decoration for the label beside it. Without this some screen readers
         announce the empty element as a blank item of its own. -->
    <span class="chip-dot" aria-hidden="true"></span>
  {/if}
  {@render children()}
</span>
