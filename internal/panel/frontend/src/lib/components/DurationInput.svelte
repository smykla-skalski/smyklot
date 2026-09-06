<script lang="ts">
  import { onDestroy, untrack } from 'svelte';

  import {
    durationParts,
    exactDurationSeconds,
    formatDuration,
    UNIT_SECONDS,
    type DurationEditorValue,
    type DurationUnit,
  } from '../duration';
  import Select from './Select.svelte';

  const {
    value,
    label,
    amountLabel = `${label} amount`,
    id,
    inherited,
    units = ['seconds', 'minutes', 'hours'],
    minimum = 0,
    maximum = Number.MAX_SAFE_INTEGER,
    allowEmpty = false,
    disabled = false,
    editor = null,
    onChange = () => {},
    onEdit,
    onValidityChange = () => {},
  }: {
    value: number | null;
    label: string;
    amountLabel?: string;
    id?: string;
    inherited?: number;
    units?: readonly DurationUnit[];
    minimum?: number;
    maximum?: number;
    allowEmpty?: boolean;
    disabled?: boolean;
    /** A persisted invalid draft, where the caller's document supports one */
    editor?: DurationEditorValue | null;
    onChange?: (seconds: number | null) => void;
    onEdit?: (editor: DurationEditorValue) => void;
    onValidityChange?: (problem: string | null) => void;
  } = $props();

  const uniqueId = $props.id();
  const fieldId = $derived(id ?? uniqueId);
  let draft = $state<DurationEditorValue | null>(null);
  let preferredUnit = $state<DurationUnit | null>(null);
  let lastValue = untrack(() => value);
  let emittedValue = lastValue;
  let lastEditorKey = untrack(() => editorKey(editor));
  let emittedEditorKey = lastEditorKey;
  const unit = $derived(
    draft?.unit ??
      preferredUnit ??
      editor?.unit ??
      durationParts(value ?? inherited ?? 0, units).unit,
  );
  const amount = $derived(
    draft?.amount ?? editor?.amount ?? (value === null ? '' : String(value / UNIT_SECONDS[unit])),
  );
  const placeholder = $derived(
    inherited === undefined ? undefined : String(inherited / UNIT_SECONDS[unit]),
  );
  const options = $derived(units.map((value) => ({ value, label: value })));
  const problem = $derived(validate(amount, unit));

  // Parent saves, discard and restored drafts are external changes. A successful
  // keystroke comes back through value too, and must not replace the text mid-edit.
  // Raw drafts can be discarded without changing seconds, so watch that payload too.
  $effect(() => {
    const next = value;
    const nextEditorKey = editorKey(editor);
    untrack(() => {
      const valueChangedElsewhere = next !== lastValue && next !== emittedValue;
      const editorChangedElsewhere =
        nextEditorKey !== lastEditorKey && nextEditorKey !== emittedEditorKey;
      if (valueChangedElsewhere || editorChangedElsewhere) {
        draft = null;
        preferredUnit = null;
        emittedValue = next;
        emittedEditorKey = nextEditorKey;
      }
      lastValue = next;
      lastEditorKey = nextEditorKey;
    });
  });
  // Validity belongs to the current constraints too. For example, disabling a
  // recurring job allows a zero cadence that was invalid a moment before. If a
  // held edit becomes valid, its displayed duration must reach the owner's wire value.
  $effect(() => {
    const invalid = problem;
    const seconds = amount.trim() === '' ? null : exactDurationSeconds({ amount, unit });
    untrack(() => {
      onValidityChange(invalid);
      if (invalid !== null || draft === null || onEdit !== undefined || seconds === emittedValue)
        return;
      emittedValue = seconds;
      onChange(seconds);
    });
  });
  onDestroy(() => onValidityChange(null));

  function editorKey(value: DurationEditorValue | null): string | null {
    return value === null ? null : JSON.stringify([value.unit, value.amount]);
  }

  function validate(text: string, unit: DurationUnit): string | null {
    if (text.trim() === '' && allowEmpty) return null;
    const seconds = exactDurationSeconds({ amount: text, unit });
    if (seconds === null) return 'Enter a duration in whole seconds';
    if (seconds < minimum || seconds > maximum) {
      return `${label} must be from ${formatDuration(minimum)} to ${formatDuration(maximum)}`;
    }
    return null;
  }

  function typeAmount(text: string): void {
    const next = { amount: text, unit };
    draft = next;
    preferredUnit = unit;
    const invalid = validate(text, unit);
    if (onEdit !== undefined) {
      emittedEditorKey = editorKey(next);
      emittedValue = exactDurationSeconds(next) ?? value;
      onEdit(next);
      return;
    }
    if (invalid !== null) return;
    emittedValue = text.trim() === '' ? null : exactDurationSeconds(next);
    onChange(emittedValue);
  }

  function pickUnit(next: DurationUnit): void {
    const seconds = exactDurationSeconds({ amount, unit });
    if (disabled || (amount.trim() !== '' && seconds === null)) return;
    preferredUnit = next;
    draft = {
      amount:
        amount.trim() === ''
          ? ''
          : String((seconds ?? value ?? inherited ?? 0) / UNIT_SECONDS[next]),
      unit: next,
    };
    // Choosing a presentation never changes the wire value or stages a save.
  }
</script>

<!--
@component
An amount and a shared unit picker for a duration stored in seconds. Unit changes
only change its presentation, so 90 seconds becomes 1.5 minutes without changing
configuration. Inherited values use a placeholder and clearing restores inheritance
only when allowEmpty is explicit. Invalid text stays visible and reports validity to
the owning draft. A caller with persisted raw drafts can pass editor and onEdit.
-->

<span class="duration-field">
  <span class="duration-controls">
    <input
      id={fieldId}
      class="text-input mono duration-amount"
      inputmode="decimal"
      maxlength="32"
      aria-label={amountLabel}
      aria-invalid={problem !== null}
      aria-describedby={problem !== null ? `${fieldId}-problem` : undefined}
      {placeholder}
      value={amount}
      {disabled}
      oninput={(event) => typeAmount(event.currentTarget.value)}
    />
    <Select
      value={unit}
      {options}
      aria-label={`${label} unit`}
      disabled={disabled ||
        (amount.trim() !== '' && exactDurationSeconds({ amount, unit }) === null)}
      onValueChange={(value) => pickUnit(value as DurationUnit)}
    />
  </span>
  {#if problem !== null}
    <span class="duration-problem" id={`${fieldId}-problem`}>{problem}</span>
  {/if}
</span>

<style>
  .duration-field {
    display: grid;
    gap: var(--space-2);
    inline-size: fit-content;
    justify-items: start;
    max-inline-size: 100%;
  }

  .duration-controls {
    align-items: center;
    display: flex;
    gap: var(--space-2);
  }

  .duration-amount {
    text-align: end;
    inline-size: 8ch;
    min-inline-size: 0;
  }

  .duration-amount[aria-invalid='true'] {
    border-color: var(--danger);
  }

  .duration-problem {
    color: var(--danger);
    font-size: var(--font-size-compact);
    max-inline-size: 32ch;
    overflow-wrap: anywhere;
    text-align: start;
  }
</style>
