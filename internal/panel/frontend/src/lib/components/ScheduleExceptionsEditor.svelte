<script lang="ts">
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import { exceptionInputs, type EditableException } from '#lib/schedule-exceptions.js';
  import { scheduleExceptionProblems } from '#lib/schedule-validation.js';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import Select from './Select.svelte';
  import FormError from './FormError.svelte';

  const {
    idPrefix,
    entries,
    onChange,
    showProblems = false,
  }: {
    idPrefix: string;
    entries: readonly EditableException[];
    onChange: (entries: EditableException[]) => void;
    showProblems?: boolean;
  } = $props();
  const previousClosingTimes = new SvelteMap<string, string>();
  function endOfDay(entry: EditableException, index: number, checked: boolean): void {
    if (checked) previousClosingTimes.set(entry.id, entry.end);
    change(index, { end: checked ? '24:00' : (previousClosingTimes.get(entry.id) ?? '') });
  }
  const touched = new SvelteSet<string>();
  const problems = $derived(scheduleExceptionProblems(exceptionInputs(entries)));

  function change(index: number, patch: Partial<EditableException>): void {
    onChange(entries.map((entry, at) => (at === index ? { ...entry, ...patch } : entry)));
  }
  function add(): void {
    onChange([
      ...entries,
      { id: crypto.randomUUID(), date: '', closed: true, start: '09:00', end: '17:00' },
    ]);
  }
</script>

<!--
@component
Date-specific replacements for weekly hours, shared by profile and request forms.
The caller owns saving. Unfinished rows stay in its draft, and switching a date
between closed and custom hours preserves the entered opening and closing times.
-->
<div class="form-stack" role="group" aria-label="Date exceptions">
  <div class="exception-heading">
    <span class="form-label">Date exceptions</span>
    <Button tone="add" row onclick={add}
      >{#snippet icon()}<Icon name="plus" size="sm" />{/snippet}Add date</Button
    >
  </div>
  <p class="form-help">Replace the weekly hours for a date in this profile's timezone.</p>
  {#if entries.length === 0}<p class="form-help">
      No exceptions. Weekly hours apply to every date.
    </p>{/if}
  {#each entries as entry, index (entry.id)}
    {@const problem =
      showProblems || touched.has(entry.id)
        ? problems
            .filter((item) => item.index === index)
            .map((item) => item.message)
            .join('. ')
        : ''}
    {@const errorId = `${idPrefix}-${entry.id}-error`}
    <div
      class="exception-row"
      role="group"
      aria-label={`Date exception ${index + 1}`}
      onfocusout={() => touched.add(entry.id)}
    >
      <label class="form-field" for={`${idPrefix}-${entry.id}-date`}>
        <span class="form-label">Date</span>
        <input
          class="text-input"
          id={`${idPrefix}-${entry.id}-date`}
          type="date"
          value={entry.date}
          aria-invalid={problem ? true : undefined}
          aria-describedby={problem ? errorId : undefined}
          oninput={(event) => change(index, { date: event.currentTarget.value })}
        />
      </label>
      <label class="form-field" for={`${idPrefix}-${entry.id}-mode`}>
        <span class="form-label">Hours</span>
        <Select
          id={`${idPrefix}-${entry.id}-mode`}
          value={entry.closed ? 'closed' : 'custom'}
          options={[
            { value: 'closed', label: 'Closed all day' },
            { value: 'custom', label: 'Custom hours' },
          ]}
          onValueChange={(mode) => change(index, { closed: mode === 'closed' })}
        />
      </label>
      <div class="exception-remove">
        <IconButton
          toolbar
          icon="close"
          label={`Remove date exception ${index + 1}`}
          onclick={() => onChange(entries.filter((item) => item.id !== entry.id))}
        />
      </div>
      {#if !entry.closed}
        <label class="form-field" for={`${idPrefix}-${entry.id}-start`}
          ><span class="form-label">Opens</span><input
            class="text-input"
            id={`${idPrefix}-${entry.id}-start`}
            type="time"
            value={entry.start}
            oninput={(event) => change(index, { start: event.currentTarget.value })}
            aria-describedby={problem ? errorId : undefined}
          /></label
        >
        <label class="form-field" for={`${idPrefix}-${entry.id}-end`}
          ><span class="form-label">Closes</span>
          {#if entry.end === '24:00'}<input
              class="text-input"
              id={`${idPrefix}-${entry.id}-end`}
              value="End of day"
              readonly
            />
          {:else}<input
              class="text-input"
              id={`${idPrefix}-${entry.id}-end`}
              type="time"
              value={entry.end}
              oninput={(event) => change(index, { end: event.currentTarget.value })}
              aria-describedby={problem ? errorId : undefined}
            />{/if}
        </label>
        <label class="check-item exception-wide"
          ><input
            type="checkbox"
            checked={entry.end === '24:00'}
            onchange={(event) => endOfDay(entry, index, event.currentTarget.checked)}
          /><span class="check-box"><Icon name="check" size="micro" /></span><span
            >Close at end of day (24:00)</span
          ></label
        >
      {/if}
      {#if problem}<div class="exception-wide" id={errorId}>
          <FormError message={problem} />
        </div>{/if}
    </div>
  {/each}
</div>

<style>
  .exception-heading {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-4);
  }
  .exception-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
    align-items: end;
    gap: var(--space-4);
  }
  .exception-wide {
    grid-column: 1 / -1;
  }
  .exception-remove {
    display: flex;
    justify-content: end;
  }
  .text-input {
    inline-size: 100%;
    min-inline-size: 0;
  }
</style>
