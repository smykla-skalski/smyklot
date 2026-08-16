<script lang="ts">
  import type { Snippet } from 'svelte';

  import Icon, { type IconName } from './Icon.svelte';

  /**
   * Tones name what a state means rather than which domain state it represents.
   *
   * `absent` is the odd one and earns its place: two solid neutral chips beside
   * each other - "Running" and "No checks" - share a fill and read as the same
   * state at a glance. An unfilled dashed keyline says "there is nothing to
   * show" in the shape itself, and still keeps the column's rhythm.
   */
  export type ChipTone = 'neutral' | 'clear' | 'signal' | 'accent' | 'warning' | 'stop' | 'absent';

  const {
    tone = 'neutral',
    dot = false,
    icon,
    small = false,
    children,
  }: {
    tone?: ChipTone;
    /** Set for a state that is live right now, rather than a fixed attribute. */
    dot?: boolean;
    /** For a report the tone alone cannot name — an outcome rather than a state. */
    icon?: IconName;
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
  {#if icon !== undefined}
    <Icon name={icon} size={small ? 11 : 13} strokeWidth={2} />
  {/if}
  <span class="chip-label">{@render children()}</span>
</span>
