<script lang="ts">
  import Icon from './Icon.svelte';

  /**
   * One alias, as a chip: the word a comment can use, an arrow, and the command it
   * stands for.
   *
   * The arrow is `aria-hidden`. It is punctuation between two values rather than a
   * word, and read out it says "right arrow" in the middle of a pair a reader is
   * trying to hear as one thing. The pair's meaning is carried by the group's own
   * label instead.
   *
   * `added` is the chip's ONE state, and it means unsaved rather than new: an alias
   * whose command has been changed is added too, because what will be written differs
   * from what is stored. The editor decides that by comparing against the saved map;
   * the chip only draws it.
   */
  const {
    from,
    to,
    added = false,
    disabled = false,
    onRemove,
  }: {
    /** The word a comment writes. */
    from: string;
    /** The canonical command it resolves to. */
    to: string;
    /** Unsaved: this alias, or its command, differs from what is stored. */
    added?: boolean;
    disabled?: boolean;
    onRemove?: () => void;
  } = $props();
</script>

<span class="word-chip" class:added>
  <span class="chip-from">{from}</span>
  <span class="chip-arrow" aria-hidden="true">→</span>
  <span class="chip-to">{to}</span>
  <button
    class="chip-x"
    aria-label="Delete alias {from}"
    title="Delete alias {from}"
    {disabled}
    onclick={() => onRemove?.()}
  >
    <Icon name="close" size={13} />
  </button>
</span>

<style>
  .word-chip {
    align-items: center;
    background: var(--strip-lift);
    border: 1px solid var(--rule);
    border-radius: var(--r-chip);
    display: inline-flex;
    font: 500 var(--font-size-control) / 1 var(--mono);
    gap: 0.4375rem;
    min-height: 2rem;
    padding: 0 0.375rem 0 0.875rem;
  }

  .word-chip.added {
    background: var(--brand-action-tint);
    border-color: var(--brand-action);
  }

  .chip-from {
    color: var(--text);
    font-weight: 500;
  }

  .chip-arrow {
    color: var(--dim);
    font-size: var(--font-size-micro);
  }

  .chip-to {
    color: var(--brand-action-text);
  }

  .chip-x {
    align-items: center;
    background: none;
    border: 0;
    border-radius: 50%;
    color: var(--dim);
    cursor: pointer;
    display: inline-flex;
    height: 1.25rem;
    justify-content: center;
    padding: 0;
    width: 1.25rem;
  }

  .chip-x:hover:not(:disabled) {
    background: var(--stop-tint);
    color: var(--stop);
  }

  /* The trim the three words take. It sat in a list of every trimmed thing in the
     editor, which is a list a chip in any other file would never have reached. */
  .chip-from,
  .chip-arrow,
  .chip-to {
    text-box: trim-both cap alphabetic;
  }
</style>
