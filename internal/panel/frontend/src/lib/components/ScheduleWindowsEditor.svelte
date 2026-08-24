<script lang="ts">
  import Button from './Button.svelte';

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

<div class="windows-editor">
  <div class="windows-heading">
    <span>Weekly open windows</span>
    <Button row onclick={() => onChange([...windows, newWindow()])}>Add window</Button>
  </div>
  {#each windows as window, index (window.id)}
    <div class="window-row">
      <label for={`${idPrefix}-day-${index}`}
        >Day<select
          id={`${idPrefix}-day-${index}`}
          value={window.weekday}
          onchange={(event) => update(index, { weekday: Number(event.currentTarget.value) })}
        >
          {#each days as day, dayIndex (day)}
            <option value={dayIndex}>{day}</option>
          {/each}
        </select></label
      >
      <label for={`${idPrefix}-start-${index}`}
        >Opens<input
          id={`${idPrefix}-start-${index}`}
          type="time"
          value={window.start}
          oninput={(event) => update(index, { start: event.currentTarget.value })}
        /></label
      >
      <label for={`${idPrefix}-end-${index}`}
        >Closes<input
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
  .windows-heading > span,
  label {
    font-size: 0.75rem;
    font-weight: 720;
  }
  .window-row {
    grid-template-columns: minmax(8rem, 1.4fr) 1fr 1fr auto;
  }
  label {
    display: grid;
    gap: var(--space-1);
  }
  input,
  select {
    background: var(--input-bg);
    border: 1px solid var(--control-border);
    border-radius: var(--radius-control);
    color: var(--text);
    font: inherit;
    min-height: 2.5rem;
    padding: 0 var(--space-3);
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
