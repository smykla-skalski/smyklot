<script module lang="ts">
  /**
   * The four things one thing can be, in the panel's own four words.
   *
   * Deliberately the same vocabulary as `BoardState`, spelled the same way: the
   * board's tiles and this row's mark report the same fact about different
   * things, and two sets of words for four states is how a page comes to say
   * "pending" in one place and "would change" in the next.
   */
  export type MarkState = 'settled' | 'change' | 'refused' | 'off';
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
    state: MarkState;
    /** The words - "2 differ", "refused". Absent for in step, which needs none. */
    label?: string;
  } = $props();
</script>

<span
  class="mark is-{state}"
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
  .mark {
    align-items: center;
    border-radius: var(--r-chip);
    display: inline-flex;
    font-family: var(--mono);
    font-size: var(--font-size-micro);
    font-variant-numeric: tabular-nums;
    font-weight: 500;
    gap: 0.3rem;
    line-height: 1;
    padding: 0.3rem 0.45rem;
    white-space: nowrap;
  }

  /* In step is the quietest thing on the row: no ground, no border, no words.
     A settled repository is the answer nobody came to read. */
  .is-settled {
    color: var(--cell-instep);
    padding-inline: 0.15rem;
  }

  .is-change {
    background: var(--cell-pending-bg);
    border: 1px solid color-mix(in srgb, var(--cell-pending) 38%, transparent);
    color: var(--cell-pending);
  }

  .is-refused {
    background: var(--cell-refused-bg);
    border: 1px solid color-mix(in srgb, var(--cell-refused) 38%, transparent);
    color: var(--cell-refused);
  }

  .is-off {
    color: var(--text-muted);
    padding-inline: 0.15rem;
  }
</style>
