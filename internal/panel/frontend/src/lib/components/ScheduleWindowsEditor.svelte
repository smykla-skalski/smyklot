<script lang="ts">
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
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

Each caller owns validation and saving. Keep the entered windows intact so invalid
or overlapping intervals remain visible for correction.
-->

<div class="form-stack windows-editor">
  <div class="windows-heading">
    <span class="form-label">Weekly hours</span>
    <Button row onclick={() => onChange([...windows, newWindow()])}>Add hours</Button>
  </div>
  {#each windows as window, index (window.id)}
    <div class="window-row" role="group" aria-label={`Hours for ${days[window.weekday]}`}>
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
      <div class="window-remove">
        <IconButton
          toolbar
          icon="close"
          label={`Remove ${days[window.weekday]} hours, ${window.start} to ${window.end}`}
          disabled={windows.length === 1}
          onclick={() => onChange(windows.filter((_, at) => at !== index))}
        />
      </div>
    </div>
  {/each}
</div>

<style>
  .windows-editor {
    container: hours-editor / inline-size;
  }
  .windows-heading {
    align-items: center;
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
  }
  .window-row {
    align-items: end;
    display: grid;
    gap: var(--space-4);
    grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr) minmax(0, 1fr) auto;
  }
  .window-remove {
    display: flex;
    justify-content: end;
  }
  @container hours-editor (max-width: 26rem) {
    .window-row {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .window-remove {
      grid-column: 2;
      grid-row: 1;
    }
    .window-row label:nth-child(2),
    .window-row label:nth-child(3) {
      grid-row: 2;
    }
  }
</style>
