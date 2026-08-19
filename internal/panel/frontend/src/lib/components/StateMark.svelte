<script module lang="ts">
  /**
   * The four things one synchronized thing can be, in the panel's own words.
   *
   * One declaration, because the board's tiles and a row's mark report the same
   * fact about different subjects - and two names for one union is how a page
   * comes to say "pending" in one place and "would change" in the next. It was
   * written out twice, with a comment on each saying it had to match the other.
   */
  export type SyncState = 'settled' | 'change' | 'refused' | 'off';
</script>

<script lang="ts">
  /**
   * What one thing is, in the smallest shape that still says it.
   *
   * The same four states the board's tiles carry, drawn for a row rather than a
   * grid: in step is a check and no words, because the answer nobody needs to
   * read is the one that should take the least room; the other three carry
   * words, because a colour alone is not a channel - and refused carries a
   * glyph as well, since it is the one a reader must not miss.
   */
  import Icon from './Icon.svelte';

  const {
    state,
    label,
  }: {
    state: SyncState;
    /** The words - "2 differ", "refused". Absent for in step, which needs none. */
    label?: string;
  } = $props();
</script>

<span
  class="state-mark is-{state}"
  title={state === 'settled' && label === undefined ? 'In step' : undefined}
>
  {#if state === 'settled'}
    <Icon name="check" size={12} strokeWidth={2.25} />
  {:else if state === 'refused'}
    <Icon name="failure" size={12} strokeWidth={2} />
  {:else if state === 'off'}
    <span aria-hidden="true">—</span>
  {/if}
  {#if label !== undefined}
    <span class="cap-trim">{label}</span>
  {/if}
</span>

<style>
  /* Built the way `.chip-small` is built, because it stands beside one: the
     same height token and the same keyline mechanism. It had neither - padding
     alone decided the height, and the keyline was a real border - so a mark and
     a chip four lines apart in one row measured 19.63px and 20.00px. What
     differs below the height is only the palette, which is the board's four
     cell colours rather than a chip's tones. */
  .state-mark {
    align-items: center;
    border-radius: var(--r-chip);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    font-weight: 500;
    gap: 0.3rem;
    line-height: 1;
    min-block-size: var(--control-height-chip-small);
    padding-inline: 0.45rem;
    white-space: nowrap;
  }

  /* In step is the quietest thing on the row: no ground, no keyline, no words.
     A settled repository is the answer nobody came to read. */
  .is-settled {
    color: var(--cell-instep);
    padding-inline: 0.15rem;
  }

  .is-change {
    background: var(--cell-pending-bg);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--cell-pending) 38%, transparent);
    color: var(--cell-pending);
  }

  .is-refused {
    background: var(--cell-refused-bg);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--cell-refused) 38%, transparent);
    color: var(--cell-refused);
  }

  .is-off {
    color: var(--text-muted);
    padding-inline: 0.15rem;
  }
</style>
