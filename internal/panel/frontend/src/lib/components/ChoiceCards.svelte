<script lang="ts">
  /**
   * A saved choice whose options each need a sentence.
   *
   * Used where a wrong pick is expensive and the words on a segment cannot
   * carry the difference: a ruleset's enforcement mode, a file's merge
   * strategy, how two lists combine. Each card is a native radio wearing the
   * title and the sentence that tells it from its neighbours.
   *
   * Controlled, like every value control here: the caller owns the value and
   * hears the wish through `onSelect`.
   */
  interface Choice {
    value: string;
    title: string;
    /** The sentence that separates this option from the one beside it. */
    why: string;
  }

  const {
    options,
    value,
    name,
    label,
    onSelect,
    disabled = false,
  }: {
    options: readonly Choice[];
    value: string;
    /** Groups the radios; unique per rendered control. */
    name: string;
    /** Names the group for assistive tech - "Enforcement". */
    label: string;
    onSelect: (next: string) => void;
    disabled?: boolean;
  } = $props();
</script>

<div class="choice-cards" role="radiogroup" aria-label={label}>
  {#each options as option (option.value)}
    <label class="choice-card" class:is-chosen={option.value === value}>
      <input
        type="radio"
        {name}
        value={option.value}
        checked={option.value === value}
        {disabled}
        onchange={() => onSelect(option.value)}
      />
      <span class="choice-dot" aria-hidden="true"></span>
      <span class="choice-title cap-trim">{option.title}</span>
      <span class="choice-why">{option.why}</span>
    </label>
  {/each}
</div>

<style>
  .choice-cards {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
  }

  .choice-card {
    background: var(--surface-base);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-surface);
    cursor: pointer;
    display: grid;
    gap: var(--space-2);
    grid-template-columns: auto 1fr;
    padding: var(--space-3) var(--space-4);
    transition:
      border-color var(--duration-fast) var(--ease-standard),
      background var(--duration-fast) var(--ease-standard);
  }

  .choice-card:hover {
    background: var(--surface-raised);
  }

  input {
    block-size: 1px;
    inline-size: 1px;
    margin: 0;
    opacity: 0;
    position: absolute;
  }

  .choice-dot {
    border: 1.5px solid var(--border-strong);
    border-radius: 50%;
    block-size: 15px;
    inline-size: 15px;
    margin-block-start: 0.1rem;
    position: relative;
  }

  .choice-card.is-chosen {
    background: var(--brand-action-tint);
    border-color: var(--brand-action);
  }

  .choice-card.is-chosen .choice-dot {
    border-color: var(--brand-action);
  }

  .choice-card.is-chosen .choice-dot::after {
    background: var(--brand-action);
    border-radius: 50%;
    content: '';
    inset: 2.5px;
    position: absolute;
  }

  input:focus-visible ~ .choice-dot {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }

  input:disabled ~ .choice-title,
  input:disabled ~ .choice-why {
    opacity: 0.5;
  }

  .choice-title {
    align-self: center;
    font-size: var(--font-size-meta);
    font-weight: 600;
  }

  .choice-why {
    color: var(--text-secondary);
    font-size: var(--font-size-compact);
    grid-column: 2;
  }
</style>
