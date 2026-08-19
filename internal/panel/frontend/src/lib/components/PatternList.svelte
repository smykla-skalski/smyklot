<script lang="ts">
  /**
   * A list of strings somebody types one at a time, worn as chips.
   *
   * Six places on the sync pages hold one - paths to retire, patterns to leave
   * alone, refs a ruleset covers, checks that must pass. Every one of them used
   * to be a textarea, which is a control that asks a reader to know that lines
   * are the separator, hides how many entries there are behind however many
   * happen to fit, and answers a typo with nothing at all.
   *
   * A chip is an entry. Adding one is a press; removing one is a press. The
   * field only appears once somebody asks for it, because a box standing open
   * beside a full list reads as a thing that must be filled in.
   */
  import Icon from './Icon.svelte';

  const {
    values,
    label,
    placeholder = '',
    addLabel = 'Add',
    empty = 'None',
    disabled = false,
    onChange,
  }: {
    values: readonly string[];
    /** Names the control - "Paths to remove". Also names each chip's own remove. */
    label: string;
    placeholder?: string;
    /** What the add press says - "Add a pattern", "Add a check". */
    addLabel?: string;
    /** What stands in for an empty list, so the row is never blank. */
    empty?: string;
    disabled?: boolean;
    onChange: (next: string[]) => void;
  } = $props();

  let adding = $state(false);
  let draft = $state('');
  let field = $state<HTMLInputElement | null>(null);

  function commit(): void {
    const value = draft.trim();
    draft = '';
    adding = false;
    // Silently, because the entry is already there: telling somebody they have
    // added a duplicate is a message about the list's own bookkeeping.
    if (value === '' || values.includes(value)) return;
    onChange([...values, value]);
  }

  function remove(value: string): void {
    onChange(values.filter((kept) => kept !== value));
  }

  function open(): void {
    adding = true;
    // After the field exists: the press that opens it is the same tick.
    queueMicrotask(() => field?.focus());
  }
</script>

<span class="pattern-list">
  {#each values as value (value)}
    <span class="word-chip">
      <span class="chip-label band-trim">{value}</span>
      {#if !disabled}
        <button
          type="button"
          class="chip-x"
          aria-label="Remove {value} from {label}"
          onclick={() => remove(value)}
        >
          <Icon name="close" size={13} />
        </button>
      {/if}
    </span>
  {:else}
    {#if !adding}
      <span class="pattern-empty">{empty}</span>
    {/if}
  {/each}

  {#if adding}
    <input
      bind:this={field}
      bind:value={draft}
      class="text-input pattern-field"
      type="text"
      spellcheck="false"
      autocomplete="off"
      aria-label={addLabel}
      {placeholder}
      onblur={commit}
      onkeydown={(event) => {
        if (event.key === 'Enter') {
          event.preventDefault();
          commit();
        }
        if (event.key === 'Escape') {
          draft = '';
          adding = false;
        }
      }}
    />
  {:else if !disabled}
    <button type="button" class="add-chip" onclick={open}>
      <Icon name="plus" size={11} strokeWidth={2} />
      <span class="cap-trim">{addLabel}</span>
    </button>
  {/if}
</span>

<style>
  .pattern-list {
    align-items: center;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .pattern-empty {
    color: var(--text-muted);
    font-size: var(--font-size-compact);
  }

  /* Quiet until the chip is under the hand: every chip carries one and almost
     none of them is wanted. */
  .word-chip .chip-x {
    opacity: 0.5;
  }

  .word-chip:hover .chip-x,
  .word-chip .chip-x:focus-visible {
    opacity: 1;
  }

  /*
   * The chip's own height, said in the chip's own terms.
   *
   * This was a bare `<input>`: no class, so it wore the UA's 2px inset border
   * on white with square corners. It then carried a `--local-control-height` of
   * the chip height to match a 24px chip; the chip beside it is the pane's own
   * control height now, so the field is simply what `.text-input` already is.
   */
  .pattern-field {
    font-family: var(--mono);
    font-size: var(--font-size-compact);
    inline-size: 12rem;
    padding-inline: 0.5rem;
  }
</style>
