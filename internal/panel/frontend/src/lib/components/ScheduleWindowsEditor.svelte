<script lang="ts">
  import Button from './Button.svelte';
  import Select from './Select.svelte';

  export interface EditableWindow {
    id: string;
    weekday: number;
    start: string;
    end: string;
  }

  const {
    idPrefix,
    windows,
    onChange,
  }: {
    idPrefix: string;
    windows: readonly EditableWindow[];
    onChange: (windows: EditableWindow[]) => void;
  } = $props();

  const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

  function update(index: number, patch: Partial<EditableWindow>): void {
    onChange(windows.map((window, at) => (at === index ? { ...window, ...patch } : window)));
  }

  function newWindow(): EditableWindow {
    return {
      id: `window-${Date.now()}-${Math.random().toString(36).slice(2)}`,
      weekday: 1,
      start: '09:00',
      end: '17:00',
    };
  }
</script>

<!--
@component
The weekly windows during which work may run, edited as a list rather than a calendar.
A window is a day and a span, and the list is the profile.

Overlapping windows are not an error and are not merged: two that overlap mean the same
thing as one that spans both, and rewriting what somebody typed into what it is
equivalent to is a change they did not make.
-->

<div class="windows-editor">
  <div class="windows-heading">
    <span>Open hours, week by week</span>
    <Button row onclick={() => onChange([...windows, newWindow()])}>Add a day</Button>
  </div>
  {#each windows as window, index (window.id)}
    <div class="window-row">
      <label class="form-field" for={`${idPrefix}-day-${index}`}
        ><span class="form-label">Day</span><Select
          id={`${idPrefix}-day-${index}`}
          value={window.weekday}
          onValueChange={(value) => update(index, { weekday: value })}
          options={days.map((day, weekday) => ({ value: weekday, label: day }))}
        /></label
      >
      <label class="form-field" for={`${idPrefix}-start-${index}`}
        ><span class="form-label">Opens</span><input
          class="text-input"
          id={`${idPrefix}-start-${index}`}
          type="time"
          value={window.start}
          oninput={(event) => update(index, { start: event.currentTarget.value })}
        /></label
      >
      <label class="form-field" for={`${idPrefix}-end-${index}`}
        ><span class="form-label">Closes</span><input
          class="text-input"
          id={`${idPrefix}-end-${index}`}
          type="time"
          value={window.end}
          oninput={(event) => update(index, { end: event.currentTarget.value })}
        /></label
      >
      <Button
        row
        tone="stop-quiet"
        disabled={windows.length === 1}
        onclick={() => onChange(windows.filter((_, at) => at !== index))}>Remove</Button
      >
    </div>
  {/each}
</div>

<style>
  .windows-editor {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-control);
    display: grid;
    gap: var(--space-3);
    padding: var(--space-3);
  }
  .windows-heading,
  .window-row {
    align-items: end;
    display: grid;
    gap: var(--space-3);
  }
  .windows-heading {
    align-items: center;
    grid-template-columns: 1fr auto;
  }
  .windows-heading > span {
    font-size: 0.75rem;
    font-weight: 720;
  }
  .window-row {
    grid-template-columns: minmax(8rem, 1.4fr) 1fr 1fr auto;
  }
  @media (max-width: 34rem) {
    .window-row {
      align-items: stretch;
      grid-template-columns: 1fr 1fr;
    }
    .window-row label:first-child {
      grid-column: 1 / -1;
    }
  }
</style>
