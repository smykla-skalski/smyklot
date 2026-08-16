<script lang="ts">
  import { chipToneOf, updateFilterSelection } from '../filter-menu';
  import type { FilterOption, FilterSection } from '../filter-menu';
  import Chip from './Chip.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';

  const {
    label,
    summary,
    hint,
    sections,
    selected,
    multiple = false,
    fallbackValue,
    align = 'start',
    wide = false,
    onChange,
  }: {
    label: string;
    summary: string;
    hint: string;
    sections: readonly FilterSection[];
    selected: readonly string[];
    multiple?: boolean;
    fallbackValue?: string;
    align?: 'start' | 'end';
    wide?: boolean;
    onChange: (values: string[]) => void;
  } = $props();

  let triggerButton = $state<HTMLElement | null>(null);
  let open = $state(false);

  const options = $derived(sections.flatMap((section) => section.options));
  // The fallback ("all") is the unfiltered state, not a selection worth counting.
  const selectedCount = $derived(
    selected.filter(
      (value) =>
        value !== fallbackValue &&
        options.find((option) => option.value === value)?.exclusive !== true,
    ).length,
  );
  const canClear = $derived(
    fallbackValue === undefined
      ? selected.length > 0
      : selected.length !== 1 || selected[0] !== fallbackValue,
  );

  function choose(option: FilterOption): void {
    onChange(updateFilterSelection(selected, option, options, multiple, fallbackValue));
    if (!multiple) close();
  }

  function clear(): void {
    onChange(fallbackValue === undefined ? [] : [fallbackValue]);
  }

  /* The layer returns focus to the trigger itself when it closes with focus
     still inside it, so this looks like a second copy of that. It stays: the
     layer can only act on where focus actually is, and Safari does not focus a
     button that was clicked, so closing from Done or an option leaves focus
     loose there and nothing to return. Relaxing the layer's rule to cover that
     would also fire when somebody clicked away to dismiss it, which is the one
     case where the trigger should not take focus back. */
  function close(): void {
    open = false;
    triggerButton?.focus();
  }
</script>

<Popover bind:open {align} itemSelector=".filter-option">
  {#snippet trigger(attributes)}
    <!-- Open state is read from the trigger's own `aria-expanded`, which the
         layer writes: mirroring it into a class here made two conventions out of
         one fact, and the other two menus already read the attribute.

         A funnel in a column heading is the only shape this has: every caller
         asked for it, so the component no longer offers another. It carried a
         `placement` prop, an `iconOnly` prop and a `showIcon` prop whose other
         values nothing had ever passed - a labelled toolbar control, with a
         chevron and a visible summary, that could not be reached from anywhere
         and so could not be seen to have rotted. -->
    <button
      class="filter-trigger"
      class:filtered={canClear}
      type="button"
      bind:this={triggerButton}
      aria-haspopup="listbox"
      aria-label={`${label}: ${summary}`}
      {...attributes}
    >
      <Icon name="filter" size={14} />
      {#if selectedCount > 0}
        <span class="selection-count" aria-hidden="true">{selectedCount}</span>
      {/if}
    </button>
  {/snippet}

  <div class="filter-body" class:wide>
    <header>
      <strong>{label}</strong>
      <span>{hint}</span>
    </header>

    <div
      class="filter-options"
      role="listbox"
      aria-label={label}
      aria-multiselectable={multiple ? 'true' : undefined}
    >
      {#each sections as section, sectionIndex (section.label ?? sectionIndex)}
        <div
          class="filter-section"
          role="group"
          aria-label={section.label === undefined ? undefined : section.label}
        >
          {#if section.label !== undefined}
            <p class="section-label">{section.label}</p>
          {/if}
          {#each section.options as option (option.value)}
            {@const isSelected = selected.includes(option.value)}
            <button
              type="button"
              class="filter-option"
              class:selected={isSelected}
              role="option"
              aria-selected={isSelected}
              onclick={() => choose(option)}
            >
              <span class:multiple class="selection-mark" aria-hidden="true">
                {#if isSelected}<span></span>{/if}
              </span>
              <span class="option-copy">
                {#if option.tone !== undefined && option.tone !== 'default'}
                  <!-- The value drawn the way its column draws it, at the size
                       its column draws it: the menu shows exactly what the table
                       shows. Shrunk to `small` it stops being the same object and
                       becomes a smaller thing that resembles it. -->
                  <span class="option-chip">
                    <Chip tone={chipToneOf(option.tone)} icon={option.icon}>{option.label}</Chip>
                  </span>
                {:else}
                  <strong>{option.label}</strong>
                {/if}
                {#if option.description !== undefined}
                  <span class="option-description">{option.description}</span>
                {/if}
              </span>
            </button>
          {/each}
        </div>
      {/each}
    </div>

    {#if multiple}
      <footer>
        <button type="button" class="clear-button" disabled={!canClear} onclick={clear}
          >Clear</button
        >
        <button type="button" class="done-button" onclick={close}>Done</button>
      </footer>
    {/if}
  </div>
</Popover>

<style>
  /* No `position: relative` on an ancestor and no z-index: the layer is in the
     top layer, which nothing in the page can be stacked over or clipped by. The
     trigger is positioned only so the count can ride its corner. */
  .filter-trigger {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--radius-control);
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    flex: none;
    height: 1.75rem;
    justify-content: center;
    line-height: 1;
    padding: 0;
    position: relative;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out),
      transform var(--duration-press) var(--ease-standard);
    user-select: none;
    width: 1.75rem;
  }

  .filter-trigger:hover,
  .filter-trigger[aria-expanded='true'] {
    background: color-mix(in srgb, var(--text-primary) 8%, transparent);
    color: var(--text-primary);
  }

  .filter-trigger:active {
    background: color-mix(in srgb, var(--text-primary) 14%, transparent);
    color: var(--text-primary);
    transform: scale(var(--press-scale-disc));
  }

  .filter-trigger.filtered {
    background: var(--brand-action);
    color: var(--on-brand-action);
  }

  .filter-trigger.filtered:hover,
  .filter-trigger.filtered[aria-expanded='true'] {
    background: var(--brand-action-hover);
    color: var(--on-brand-action);
  }

  .filter-trigger.filtered:active {
    background: var(--brand-action-pressed);
    color: var(--on-brand-action);
  }

  /* The count rides the funnel's corner so an active filter says how many
     values it holds without reclaiming header width. */
  .selection-count {
    align-items: center;
    background: var(--surface-base);
    border-radius: var(--radius-chip);
    box-shadow: 0 0 0 1px var(--border-subtle);
    color: var(--brand-action-text);
    display: inline-grid;
    font: 700 0.5625rem / 1 var(--sans);
    height: 0.875rem;
    padding: 2px 4px;
    place-items: center;
    position: absolute;
    right: -4px;
    top: -4px;
  }

  /* A column that can shrink, so the header and the footer keep their size and
     the options between them take whatever the layer has left. Which edge it
     lines up with is the layer's business now, not a left/right pair here. */
  /* The menu hangs off a column heading, and a heading is uppercase with tracking
     - both of which inherit. Without this the menu's own title and its hint were
     shouted in capitals and spaced like a label, because they were sitting inside
     a `<th>`. The menu is its own surface and states its own typography. */
  .filter-body {
    display: flex;
    flex-direction: column;
    letter-spacing: normal;
    min-height: 0;
    overflow: hidden;
    text-transform: none;
    width: min(17rem, calc(100vw - 2rem));
  }

  .filter-body.wide {
    width: min(21rem, calc(100vw - 2rem));
  }

  /* Every measure in this menu used to be smaller than the one the rest of the
     product uses - a 12px title, a 10px hint, 0.35rem of padding - so it read as
     cramped beside every other surface. It is on the type ramp and the space
     scale now, like everything else. */
  .filter-body > header {
    border-bottom: 1px solid var(--rule);
    display: flex;
    flex-direction: column;
    padding: var(--space-4);
  }

  .filter-body > header strong {
    font-size: var(--font-size-meta);
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  .filter-body > header span {
    color: var(--dim);
    font-size: var(--font-size-compact);
    line-height: 1.4;
    /* A real step between the title and the line under it: at 0.1rem the two ran
       together and the hint read as a second line of the title. */
    margin-top: var(--space-2);
    text-box: trim-both cap alphabetic;
  }

  /* `min-height: 0`, which is what lets a flex item scroll rather than push its
     siblings out of the layer. The height it may take is whatever the layer
     measured for itself, so there is no 24rem guess here any more. */
  .filter-options {
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-2);
  }

  .filter-section + .filter-section {
    border-top: 1px solid var(--rule);
    margin-top: var(--space-2);
    padding-top: var(--space-2);
  }

  .section-label {
    color: var(--dim);
    font: 700 var(--font-size-micro) / 1 var(--sans);
    letter-spacing: 0.1em;
    margin: 0;
    padding: var(--space-2) var(--space-2) var(--space-1);
    text-transform: uppercase;
  }

  /* A grid, so the mark, the label and anything after them are one ROW and share
     a centre. Under `align-items: center` on a flex row the mark would centre on
     the label-and-description block instead - 6.47px below the line it marks. */
  .filter-option {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: var(--r-ctl);
    color: var(--text);
    column-gap: var(--space-3);
    display: grid;
    font: 400 var(--font-size-meta) / 1 var(--sans);
    grid-template-columns: auto minmax(0, 1fr) auto;
    padding: var(--space-3) var(--space-2);
    row-gap: var(--space-2);
    text-align: left;
    width: 100%;
  }

  /* A row with nothing on its second line does not need the room a second line
     would have taken. The approved design gives every value a description and so
     draws every row at 66.73px; where a menu has none, the same padding leaves a
     chip floating in a 49.8px row with the space a sentence would have used. */
  .filter-option:not(:has(.option-description)) {
    padding-block: var(--space-2);
  }

  .filter-option:hover,
  .filter-option:focus-visible {
    background: var(--interactive-hover);
  }

  .filter-option:focus-visible {
    outline-offset: -1px;
  }

  .filter-option:active {
    box-shadow: inset 0 0 0 100vmax var(--press);
  }

  /* One size for both marks and one weight for their borders, so a menu of
     checkboxes and a menu of radios read as the same control in two moods. The
     two selected states stay conventionally distinct - a checkbox fills and
     carries a tick, a radio keeps its ring and takes a dot - which is what tells
     a reader whether picking one thing will drop another. */
  .selection-mark {
    block-size: 1rem;
    border: 1.5px solid var(--border-strong);
    border-radius: 50%;
    display: grid;
    flex: none;
    inline-size: 1rem;
    place-items: center;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out);
  }

  .selection-mark.multiple {
    border-radius: 5px;
  }

  .filter-option:hover .selection-mark {
    border-color: var(--text-soft);
  }

  /* The checkbox fills; the radio keeps its ring, which is the whole of the
     distinction. */
  .filter-option.selected .selection-mark.multiple {
    background: var(--accent);
    border-color: var(--accent);
  }

  .filter-option.selected .selection-mark:not(.multiple) {
    border-color: var(--accent);
  }

  /* The radio's dot is a grid item like the checkbox's tick, not an `::after`
     positioned against the box: an absolutely-sized pseudo-element inside a
     bordered box centres on the PADDING box, which is a border-width off the
     mark's own middle. */
  .selection-mark span {
    background: var(--accent);
    block-size: 0.5rem;
    border-radius: 50%;
    inline-size: 0.5rem;
  }

  .selection-mark.multiple span {
    background: transparent;
    border-bottom: 1.5px solid var(--on-admin);
    border-right: 1.5px solid var(--on-admin);
    border-radius: 0;
    block-size: 0.45rem;
    inline-size: 0.25rem;
    transform: rotate(45deg) translate(-0.05rem, -0.05rem);
  }

  .option-chip {
    grid-column: 2;
    justify-self: start;
    min-width: 0;
  }

  /* The label sits on the option's own row; a description drops to a second row
     spanning from the label across, so it lines up under the words rather than
     under the mark. */
  .option-copy {
    display: contents;
  }

  .option-copy strong {
    font: 600 var(--font-size-meta) / 1 var(--sans);
    grid-column: 2;
    justify-self: start;
    min-width: 0;
    text-box: trim-both cap alphabetic;
  }

  .option-description {
    color: var(--dim);
    font-size: var(--font-size-compact);
    grid-column: 2 / -1;
    grid-row: 2;
    line-height: 1;
    text-box: trim-both cap alphabetic;
  }

  /* The quiet way out sits on the left and the committing one on the right, and
     the buttons keep their own room under the rule above them. */
  .filter-body > footer {
    align-items: center;
    border-top: 1px solid var(--rule);
    display: flex;
    gap: var(--space-2);
    justify-content: space-between;
    padding: var(--space-3);
  }

  .filter-body > footer button {
    border: 0;
    border-radius: 6px;
    font-size: 0.6875rem;
    font-weight: 650;
    height: 1.875rem;
    padding: 0 0.65rem;
  }

  .clear-button {
    background: transparent;
    color: var(--dim);
  }

  .clear-button:hover:not(:disabled) {
    background: var(--strip-lift);
    color: var(--text);
  }

  .clear-button:disabled {
    cursor: default;
    opacity: 0.4;
  }

  .done-button {
    background: var(--admin);
    color: var(--on-admin);
  }

  .done-button:hover {
    background: color-mix(in srgb, var(--admin) 88%, var(--strip));
  }

  @media (prefers-reduced-motion: reduce) {
    .filter-trigger {
      transition: none;
    }
  }
</style>
