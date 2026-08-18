<script module lang="ts">
  /** The units a duration can be given in; a caller offers whichever suit it. */
  export type DurationUnit = 'seconds' | 'minutes' | 'hours' | 'days';

  const WORDS: Record<DurationUnit, string> = {
    seconds: 'Seconds',
    minutes: 'Minutes',
    hours: 'Hours',
    days: 'Days',
  };
</script>

<script lang="ts">
  import Button from './Button.svelte';
  import Select from './Select.svelte';

  /**
   * A number, the unit it is counted in, and the button that applies them.
   *
   * The Root settings page asks for three of these - a sweep interval, a quiet
   * period and a session length - and had written the same form three times over,
   * differing only in the bound values, the labels, and which units it offered.
   *
   * The labels are visually hidden on purpose: the setting has already been named by
   * the plate this sits in, and repeating it above two controls in a row would say
   * the same words three times. They are still said, because a screen reader arrives
   * at the two fields with no plate heading in between.
   */
  let {
    amount = $bindable(),
    unit = $bindable(),
    units,
    label,
    disabled = false,
    onApply,
  }: {
    amount: number;
    unit: DurationUnit;
    /** In the order they should be offered; the smallest sensible one first. */
    units: readonly DurationUnit[];
    /** What is being timed - "Reaction sweep interval". Announced, not drawn. */
    label: string;
    disabled?: boolean;
    onApply: () => void;
  } = $props();
</script>

<form
  class="duration-form"
  aria-label={`Custom ${label.toLowerCase()}`}
  onsubmit={(event) => {
    event.preventDefault();
    onApply();
  }}
>
  <label>
    <span class="visually-hidden">{label}</span>
    <input
      class="text-input duration-input"
      type="number"
      min="1"
      step="1"
      bind:value={amount}
      {disabled}
    />
  </label>
  <label>
    <span class="visually-hidden">{label} unit</span>
    <Select
      bind:value={unit}
      {disabled}
      options={units.map((value) => ({ value, label: WORDS[value] }))}
    />
  </label>
  <Button tone="signal" type="submit" {disabled}>Apply</Button>
</form>

<style>
  /* Lifted verbatim from `RootSettings`, which declared it three times over for the
     three forms it drew. No `display`: the form is a block and its three controls
     flow inline, which is what it did before and what the narrow layout below
     deliberately overrides. */
  .duration-form {
    color: var(--text-secondary);
    font-size: var(--font-size-meta);
    gap: var(--space-2);
    margin: 0;
  }

  .duration-input {
    width: 6rem;
  }

  /* Two columns and a full-width Apply beneath them, once the row will not fit. */
  @media (max-width: 40rem) {
    .duration-form {
      align-items: stretch;
      display: grid;
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
      width: 100%;
    }

    .duration-form :global(.btn) {
      grid-column: 1 / -1;
    }

    .duration-input {
      width: 100%;
    }
  }
</style>
