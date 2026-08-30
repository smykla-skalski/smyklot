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

<!--
@component
A value a row is reporting, drawn as a mark. Where `StatusPill` says what is true of
the surface it stands on - a page's scope, a dependency's health - a chip carries the
value of one cell and takes its tone from the vocabulary that column draws, so the
same word is the same colour down the whole table.

A tone names what a state MEANS rather than which domain state it is, which is what
lets one set cover schedules, checks and sync alike. `absent` is the odd one and earns
its place: two solid neutral chips beside each other read as the same state at a
glance, so "there is nothing to show" is said with an unfilled dashed keyline - in the
shape itself, where colour is not carrying it alone.

`small` is for a chip set inside a line of text rather than beside it. A chip a reader
can remove is a different thing again and is not this.
-->

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
