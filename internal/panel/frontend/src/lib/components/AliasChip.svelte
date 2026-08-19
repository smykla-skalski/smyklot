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
  <span class="chip-from band-trim">{from}</span>
  <span class="chip-arrow band-trim" aria-hidden="true">→</span>
  <span class="chip-to band-trim">{to}</span>
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
  /* `.word-chip` and `.chip-x` live in `app.css`: they are the editable-entry
     control, worn here and by `PatternList`, and each had its own copy at its
     own height. What stays here is what an ALIAS adds to one - three words and
     an arrow rather than a single string. */
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
</style>
